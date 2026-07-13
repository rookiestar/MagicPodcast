#!/bin/bash
# Run the daily MagicPodcast database backup.

set -euo pipefail

PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"

PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LOG_DIR="$PROJECT_DIR/logs"
LOCK_DIR="${TMPDIR:-/tmp}/magicpodcast-backup.lock"
BACKUP_DIR="${BACKUP_DIR:-$PROJECT_DIR/backend/data/backups}"

mkdir -p "$LOG_DIR"

if ! mkdir "$LOCK_DIR" 2>/dev/null; then
  echo "[$(date '+%Y-%m-%d %H:%M:%S')] Backup already running; skipping."
  exit 0
fi

cleanup() {
  rmdir "$LOCK_DIR" 2>/dev/null || true
}
trap cleanup EXIT

echo "[$(date '+%Y-%m-%d %H:%M:%S')] Starting daily backup."
RETENTION_DAYS="${RETENTION_DAYS:-14}" COMPRESS="${COMPRESS:-true}" "$PROJECT_DIR/scripts/backup-db.sh"
if [ -n "${MAGICPODCAST_OFFSITE_DIR:-}" ] || [ -n "${MAGICPODCAST_AGE_RECIPIENT_FILE:-}" ]; then
  latest_backup="$(find "$BACKUP_DIR" -maxdepth 1 -type f -name 'magicpodcast_*.db.gz' | sort -r | head -1 || true)"
  [ -n "$latest_backup" ] || {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] No local backup found for offsite encryption." >&2
    exit 1
  }
  "$PROJECT_DIR/scripts/offsite-backup.sh" "$latest_backup"
else
  echo "[$(date '+%Y-%m-%d %H:%M:%S')] Offsite encryption is not configured; local backup only."
fi
echo "[$(date '+%Y-%m-%d %H:%M:%S')] Daily backup finished."
