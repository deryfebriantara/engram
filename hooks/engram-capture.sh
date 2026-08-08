#!/usr/bin/env bash
# Claude Code SessionEnd hook: asks the self-hosted Qwen gateway to pull 0-5
# durable memories out of the just-finished transcript, then stores them via
# the `engram` CLI (server-side dedup makes re-storing the same fact safe).
# This script produces NO stdout. All status/skip reasons go to a log file.
# Registration in ~/.claude/settings.json is out of scope for this script;
# see hooks/README or `make install-hooks`.
#
# Input: a single JSON object on stdin with (at least) `.transcript_path`,
# `.cwd`, `.reason`.

set -u

LOG_FILE="$HOME/.claude/logs/engram-capture.log"

log() {
  mkdir -p "$(dirname "$LOG_FILE")" 2>/dev/null
  printf '%s %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$1" >>"$LOG_FILE" 2>/dev/null
}

# This hook must never fail or hang the SessionEnd flow. Whatever happens
# internally, we always exit 0.
trap 'exit 0' EXIT

command -v jq >/dev/null 2>&1 || { log "skip: jq not found"; exit 0; }
command -v curl >/dev/null 2>&1 || { log "skip: curl not found"; exit 0; }

# --- outer stage: read stdin, then re-exec ourselves under a hard timeout ---
# so a stuck curl call or a huge transcript can never hang SessionEnd. The
# inner stage below (curl --max-time 90 plus up to 5x `timeout 5 engram
# store`) bounds itself to ~115s, comfortably inside this 118s outer budget.
if [ "${ENGRAM_CAPTURE_STAGE:-outer}" = "outer" ]; then
  INPUT_JSON="$(cat)"

  TRANSCRIPT_PATH="$(printf '%s' "$INPUT_JSON" | jq -r '.transcript_path // empty' 2>/dev/null)"
  CWD="$(printf '%s' "$INPUT_JSON" | jq -r '.cwd // empty' 2>/dev/null)"
  REASON="$(printf '%s' "$INPUT_JSON" | jq -r '.reason // empty' 2>/dev/null)"

  ENGRAM_CAPTURE_STAGE=inner timeout 118 "$0" "$TRANSCRIPT_PATH" "$CWD" "$REASON" </dev/null >/dev/null 2>&1
  exit 0
fi

# --- inner stage: does the real work ---
TRANSCRIPT_PATH="${1:-}"
CWD="${2:-}"
REASON="${3:-}"

ENGRAM_BIN=""
if [ -x "$HOME/.local/bin/engram" ]; then
  ENGRAM_BIN="$HOME/.local/bin/engram"
elif command -v engram >/dev/null 2>&1; then
  ENGRAM_BIN="$(command -v engram)"
else
  log "skip: engram binary not found"
  exit 0
fi

if [ -z "$TRANSCRIPT_PATH" ] || [ ! -r "$TRANSCRIPT_PATH" ]; then
  log "skip: transcript missing or unreadable ($TRANSCRIPT_PATH)"
  exit 0
fi

LINE_COUNT="$(jq -c 'select(.type == "user" or .type == "assistant")' "$TRANSCRIPT_PATH" 2>/dev/null | wc -l | tr -d ' ')"
if [ -z "$LINE_COUNT" ] || [ "$LINE_COUNT" -lt 8 ] 2>/dev/null; then
  log "skip: only ${LINE_COUNT:-0} user/assistant lines (<8) in transcript"
  exit 0
fi

# --- gateway config: env override, else parse the fallback literal out of
# the shared claude-qwen launcher script (never hardcode the token here) ---
GATEWAY_URL="${CLAUDE_QWEN_GATEWAY_URL:-}"
GATEWAY_TOKEN="${CLAUDE_QWEN_GATEWAY_TOKEN:-}"

QWEN_SCRIPT="$HOME/.claude/scripts/claude-qwen.sh"
if [ -z "$GATEWAY_URL" ] && [ -f "$QWEN_SCRIPT" ]; then
  GATEWAY_URL="$(sed -n 's/^GATEWAY_URL="\${CLAUDE_QWEN_GATEWAY_URL:-\(.*\)}"$/\1/p' "$QWEN_SCRIPT" | head -1)"
fi
if [ -z "$GATEWAY_TOKEN" ] && [ -f "$QWEN_SCRIPT" ]; then
  GATEWAY_TOKEN="$(sed -n 's/^GATEWAY_TOKEN="\${CLAUDE_QWEN_GATEWAY_TOKEN:-\(.*\)}"$/\1/p' "$QWEN_SCRIPT" | head -1)"
fi

if [ -z "$GATEWAY_URL" ] || [ -z "$GATEWAY_TOKEN" ]; then
  log "skip: gateway URL/token unavailable"
  exit 0
fi

# --- extract a "ROLE: text" conversation stream, text blocks only ---
CONVO="$(jq -r '
  select(.type == "user" or .type == "assistant") |
  .type as $role |
  (
    if (.message.content | type) == "string" then .message.content
    elif (.message.content | type) == "array" then
      [.message.content[]? | select(.type == "text") | .text] | join("\n")
    else empty end
  ) as $text |
  if ($text // "") == "" then empty else ($role | ascii_upcase) + ": " + $text end
' "$TRANSCRIPT_PATH" 2>/dev/null)"

CONVO="$(printf '%s' "$CONVO" | tail -c 40000)"

if [ -z "$CONVO" ]; then
  log "skip: no extractable text in transcript"
  exit 0
fi

SYSTEM_PROMPT='You are a memory-extraction function for a coding assistant. Read the transcript and extract at most 5 LASTING memories: durable user preferences, project decisions, recurring patterns, or hard-won facts. Do NOT extract task minutiae, anything session-specific, or secrets/credentials.

Respond with ONLY a raw JSON array -- no markdown fences, no explanation, no thinking, nothing before or after it. Each element MUST use exactly these three keys:
{"content": "<one self-contained sentence>", "category": "<one of: preference, project, pattern, decision, fact>", "tags": ["<short-tag>", "..."]}

Example of a fully correct response:
[{"content": "The user always wants conventional commit prefixes (feat:, fix:, chore:) on every commit.", "category": "preference", "tags": ["git", "commits"]}]

If nothing qualifies, respond with exactly: []'

REQUEST_BODY="$(jq -n \
  --arg system "$SYSTEM_PROMPT" \
  --arg convo "$CONVO" \
  '{
    model: "Qwen/Qwen3.6-27B-FP8",
    max_tokens: 2048,
    system: $system,
    messages: [{role: "user", content: $convo}]
  }')"

RESPONSE="$(curl -sS --max-time 90 \
  -H "x-api-key: $GATEWAY_TOKEN" \
  -H "anthropic-version: 2023-06-01" \
  -H "content-type: application/json" \
  -d "$REQUEST_BODY" \
  "$GATEWAY_URL/v1/messages" 2>/dev/null)"
CURL_RC=$?

if [ $CURL_RC -ne 0 ] || [ -z "$RESPONSE" ]; then
  log "skip: gateway unreachable (curl rc=$CURL_RC)"
  exit 0
fi

RAW_TEXT="$(printf '%s' "$RESPONSE" | jq -r '[.content[]? | select(.type == "text") | .text] | join("")' 2>/dev/null)"
if [ -z "$RAW_TEXT" ]; then
  log "skip: gateway response had no text content"
  exit 0
fi

# Strip markdown code fences (```json ... ``` or ``` ... ```) if present.
CLEANED="$(printf '%s' "$RAW_TEXT" | sed -E '/^```/d')"

MEMORIES_JSON="$(printf '%s' "$CLEANED" | jq -c 'if type == "array" then . else empty end' 2>/dev/null)"
if [ -z "$MEMORIES_JSON" ]; then
  log "skip: model did not return a JSON array"
  exit 0
fi

# Validate: non-empty content, content <= 500 chars, category in the
# allowed set (else default to "fact"), capped at 5 items.
VALID_JSON="$(printf '%s' "$MEMORIES_JSON" | jq -c '
  map(select((.content // "") != "" and ((.content | length) <= 500)))
  | map(.category as $c | .category = (if (["preference","project","pattern","decision","fact"] | index($c)) then $c else "fact" end))
  | .[0:5]
' 2>/dev/null)"

if [ -z "$VALID_JSON" ] || [ "$VALID_JSON" = "[]" ]; then
  log "done: gateway returned no qualifying memories"
  exit 0
fi

SOURCE="$(basename "${CWD:-.}" 2>/dev/null)"
STORED=0

while IFS= read -r ITEM; do
  [ -z "$ITEM" ] && continue

  CONTENT="$(printf '%s' "$ITEM" | jq -r '.content')"
  CATEGORY="$(printf '%s' "$ITEM" | jq -r '.category')"
  TAGS="$(printf '%s' "$ITEM" | jq -r '[.tags[]? | select(type == "string")] | join(",")')"

  STORE_OUT="$(timeout 5 "$ENGRAM_BIN" store -category "$CATEGORY" -tags "$TAGS" -source "$SOURCE" -- "$CONTENT" 2>&1)"
  STORE_RC=$?
  log "store rc=$STORE_RC category=$CATEGORY tags=$TAGS -> $STORE_OUT"

  STORED=$((STORED + 1))
done < <(printf '%s' "$VALID_JSON" | jq -c '.[]' 2>/dev/null)

log "done: stored=$STORED reason=${REASON:-unknown}"
exit 0
