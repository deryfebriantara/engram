#!/usr/bin/env bash
# Backs up the ChromaDB data volume (the entire memory store) to a timestamped
# tarball, keeping the newest 8. Meant to be run periodically from cron; see
# hooks/README or `make install-hooks` for the suggested schedule.
#
# Caveat: this is a *live-file* copy -- `docker run ... tar czf` reads
# ChromaDB's on-disk files (including chroma.sqlite3) while the ChromaDB
# container may still be writing to them. That's a small torn-copy /
# inconsistent-snapshot risk (no transaction lock or write freeze around the
# tar), but engram is a low-write store (memories are stored/updated one at a
# time, not in bulk), so the odds of tarring mid-write are low and, if it
# ever did happen, the previous 7 backups are still there. A cleaner backup
# would stop the container or use ChromaDB's own snapshot/export first; not
# worth the added complexity for this use case.
#
# This script produces no stdout; one summary line goes to
# ~/.claude/logs/engram-backup.log. Always exits 0 -- a cron job that fails
# loudly just spams cron's mail.

set -u

BACKUP_DIR="$HOME/backups/engram"
LOG_FILE="$HOME/.claude/logs/engram-backup.log"
VOLUME_NAME="engram_chroma_data"
KEEP=8

log() {
  mkdir -p "$(dirname "$LOG_FILE")" 2>/dev/null
  printf '%s %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$1" >>"$LOG_FILE" 2>/dev/null
}

# This job must never hang or fail its caller (cron). Whatever happens
# internally, always exit 0.
trap 'exit 0' EXIT

command -v docker >/dev/null 2>&1 || { log "skip: docker not found"; exit 0; }

if ! docker volume inspect "$VOLUME_NAME" >/dev/null 2>&1; then
  log "skip: volume $VOLUME_NAME not found"
  exit 0
fi

mkdir -p "$BACKUP_DIR" 2>/dev/null

TARBALL="chroma-$(date -u +%Y%m%d-%H%M).tar.gz"

if ! docker run --rm \
  -v "$VOLUME_NAME:/data:ro" \
  -v "$BACKUP_DIR:/backup" \
  alpine tar czf "/backup/$TARBALL" -C / data >/dev/null 2>&1; then
  log "error: docker run/tar failed (volume=$VOLUME_NAME)"
  exit 0
fi

if [ ! -s "$BACKUP_DIR/$TARBALL" ]; then
  log "error: $TARBALL missing or empty after backup attempt"
  exit 0
fi

# Prune to the newest $KEEP tarballs.
pruned=0
mapfile -t all_backups < <(ls -1t "$BACKUP_DIR"/chroma-*.tar.gz 2>/dev/null)
if [ "${#all_backups[@]}" -gt "$KEEP" ]; then
  for old in "${all_backups[@]:$KEEP}"; do
    rm -f "$old" 2>/dev/null && pruned=$((pruned + 1))
  done
fi

size="$(du -h "$BACKUP_DIR/$TARBALL" 2>/dev/null | cut -f1)"
log "done: wrote $TARBALL (${size:-?}), pruned=$pruned, kept<=$KEEP"
exit 0
