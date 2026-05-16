#!/bin/bash
# Run the daily MagicPodcast database backup.

set -euo pipefail

PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"

PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
LOG_DIR="$PROJECT_DIR/logs"
LOCK_DIR="${TMPDIR:-/tmp}/magicpodcast-backup.lock"

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
echo "[$(date '+%Y-%m-%d %H:%M:%S')] Daily backup finished."
