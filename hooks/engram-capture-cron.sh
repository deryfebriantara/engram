#!/usr/bin/env bash
# Cron-driven incremental capture. hooks/engram-capture.sh only fires on
# Claude Code's SessionEnd event, which real-world usage shows is not
# reliable enough on its own: sessions get killed, the terminal closes, the
# machine reboots -- SessionEnd never runs, so that session's memories are
# never captured (observed in practice: 32 skips, 0 real captures over a
# stretch where sessions kept ending uncleanly).
#
# This script makes capture independent of session lifecycle: it periodically
# re-scans recently-modified transcripts, tracks how many lines of each it
# has already looked at (an "offset"), and hands only the *new* tail of each
# transcript to engram-capture.sh -- so re-running this on a cron schedule is
# safe and cheap, never reprocessing what was already captured.
#
# Suggested crontab line (also echoed by `make install-hooks`):
#   17 */6 * * * $HOME/.claude/hooks/engram-capture-cron.sh
#
# This script produces no stdout of its own; per-run summaries go to the same
# log file engram-capture.sh uses, prefixed "cron:". Always exits 0 -- a cron
# job that fails loudly just spams cron's mail, and this must never be able
# to wedge a user's crontab.
#
# Input: none (stdin unused). Everything is derived from disk state.

set -u

STATE_DIR="$HOME/.local/state/engram"
LOCK_FILE="$STATE_DIR/cron.lock"
OFFSETS_FILE="$STATE_DIR/capture-offsets.tsv"
LOG_FILE="$HOME/.claude/logs/engram-capture.log"
# Overridable so this script (and its own tests) can point at a synthetic
# projects tree instead of the user's real, private transcripts.
ENGRAM_PROJECTS_DIR="${ENGRAM_PROJECTS_DIR:-$HOME/.claude/projects}"
MAX_TRANSCRIPTS=5
MIN_NEW_LINES=8

mkdir -p "$STATE_DIR" 2>/dev/null

log() {
  mkdir -p "$(dirname "$LOG_FILE")" 2>/dev/null
  printf '%s %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "cron: $1" >>"$LOG_FILE" 2>/dev/null
}

# This job must never hang or fail its caller (cron). Whatever happens
# internally, always exit 0.
trap 'exit 0' EXIT

command -v jq >/dev/null 2>&1 || { log "skip-run: jq not found"; exit 0; }

CAPTURE_HOOK=""
if [ -x "$HOME/.claude/hooks/engram-capture.sh" ]; then
  CAPTURE_HOOK="$HOME/.claude/hooks/engram-capture.sh"
elif [ -x "$(dirname "$0")/engram-capture.sh" ]; then
  CAPTURE_HOOK="$(dirname "$0")/engram-capture.sh"
else
  log "skip-run: engram-capture.sh not found (installed or repo copy)"
  exit 0
fi

# Single-instance guard: if a previous run is still going (or stuck), skip
# this one rather than stacking up overlapping runs.
exec 9>"$LOCK_FILE"
if ! flock -n 9; then
  log "skip-run: lock held"
  exit 0
fi

[ -f "$OFFSETS_FILE" ] || : > "$OFFSETS_FILE"

# get_prev_count PATH -> previously-recorded line count for PATH, or 0.
get_prev_count() {
  awk -F'\t' -v p="$1" '$1 == p { print $2; found=1 } END { if (!found) print 0 }' "$OFFSETS_FILE" 2>/dev/null
}

# set_count PATH COUNT -> rewrite PATH's entry to COUNT (append if new).
set_count() {
  local path="$1" count="$2" tmp
  tmp="$(mktemp "$STATE_DIR/offsets.XXXXXX")" || return 0
  awk -F'\t' -v p="$path" '$1 != p' "$OFFSETS_FILE" >"$tmp" 2>/dev/null
  printf '%s\t%s\n' "$path" "$count" >>"$tmp"
  mv "$tmp" "$OFFSETS_FILE" 2>/dev/null
}

scanned=0
processed=0
skipped_small=0
skipped_nogrow=0
errors=0

while IFS= read -r transcript; do
  [ -z "$transcript" ] && continue

  # Never touch our own state/lock directory, even if ENGRAM_PROJECTS_DIR
  # ever ends up pointed somewhere that overlaps it.
  case "$transcript" in
    "$STATE_DIR"/*) continue ;;
  esac

  [ "$processed" -ge "$MAX_TRANSCRIPTS" ] && break
  scanned=$((scanned + 1))

  total="$(wc -l <"$transcript" 2>/dev/null | tr -d ' ')"
  if [ -z "$total" ]; then
    errors=$((errors + 1))
    continue
  fi

  prev="$(get_prev_count "$transcript")"
  [ -z "$prev" ] && prev=0

  if [ "$total" -le "$prev" ] 2>/dev/null; then
    skipped_nogrow=$((skipped_nogrow + 1))
    continue
  fi

  chunk="$(tail -n +"$((prev + 1))" "$transcript" 2>/dev/null)"
  new_ua_count="$(printf '%s\n' "$chunk" | jq -c 'select(.type == "user" or .type == "assistant")' 2>/dev/null | wc -l | tr -d ' ')"

  if [ -z "$new_ua_count" ] || [ "$new_ua_count" -lt "$MIN_NEW_LINES" ] 2>/dev/null; then
    # Deliberately do NOT advance state here: the new lines stay unread so
    # they accumulate with whatever arrives next run, instead of being lost.
    skipped_small=$((skipped_small + 1))
    continue
  fi

  tmp_transcript="$(mktemp "$STATE_DIR/chunk.XXXXXX.jsonl")"
  printf '%s\n' "$chunk" >"$tmp_transcript"

  cwd="$(jq -r 'select(.cwd != null) | .cwd' "$transcript" 2>/dev/null | head -1)"
  [ -n "$cwd" ] || cwd="$(basename "$(dirname "$transcript")")"

  jq -n --arg tp "$tmp_transcript" --arg cwd "$cwd" \
    '{"transcript_path": $tp, "cwd": $cwd, "reason": "cron"}' |
    "$CAPTURE_HOOK"

  set_count "$transcript" "$total"
  rm -f "$tmp_transcript"
  processed=$((processed + 1))
done < <(find "$ENGRAM_PROJECTS_DIR" -maxdepth 2 -name '*.jsonl' -mtime -7 2>/dev/null | grep -v '/subagents/')

log "scanned=$scanned processed=$processed skipped_small=$skipped_small skipped_nogrow=$skipped_nogrow errors=$errors"
exit 0
