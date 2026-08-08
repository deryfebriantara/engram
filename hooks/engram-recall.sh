#!/usr/bin/env bash
# Claude Code UserPromptSubmit hook: silently prepends relevant long-term
# memories (via the `engram` CLI) to the model's context before the prompt
# is processed. Stdout of this script IS injected as context, so it must
# print either nothing or exactly the memory block below -- no diagnostics,
# no partial output. Registration in ~/.claude/settings.json is out of
# scope for this script; see hooks/README or `make install-hooks`.
#
# Input: a single JSON object on stdin with (at least) `.prompt` and `.cwd`.

set -u

# This hook must never block or fail prompt submission. Whatever happens
# internally, we always exit 0.
trap 'exit 0' EXIT

command -v jq >/dev/null 2>&1 || exit 0

ENGRAM_BIN=""
if [ -x "$HOME/.local/bin/engram" ]; then
  ENGRAM_BIN="$HOME/.local/bin/engram"
elif command -v engram >/dev/null 2>&1; then
  ENGRAM_BIN="$(command -v engram)"
else
  exit 0
fi

INPUT_JSON="$(cat)"

PROMPT="$(printf '%s' "$INPUT_JSON" | jq -r '.prompt // empty' 2>/dev/null)"
CWD="$(printf '%s' "$INPUT_JSON" | jq -r '.cwd // empty' 2>/dev/null)"

[ -n "$PROMPT" ] || exit 0
[ "${#PROMPT}" -ge 20 ] || exit 0

case "$PROMPT" in
  /*) exit 0 ;;
esac

SOURCE="$(basename "${CWD:-.}" 2>/dev/null)"

OUTPUT="$(timeout 10 "$ENGRAM_BIN" recall \
  -limit "${ENGRAM_RECALL_LIMIT:-3}" \
  -threshold "${ENGRAM_RECALL_THRESHOLD:-0.55}" \
  -source "$SOURCE" \
  -- "$PROMPT" 2>/dev/null)"
RC=$?

[ $RC -eq 0 ] || exit 0
[ -n "$OUTPUT" ] || exit 0

printf '## Long-term memories (Engram, auto-recalled — background info, may be stale)\n%s\n' "$OUTPUT"

exit 0
