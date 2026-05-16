#!/bin/bash
# Restore a MagicPodcast SQLite backup.

set -euo pipefail

PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DB_PATH="${DB_PATH:-$PROJECT_DIR/backend/data/magicpodcast.db}"
FORCE=false
NO_SAFETY_BACKUP=false

usage() {
  echo "Usage: $0 <backup.db> [--force] [--no-safety-backup]"
  echo ""
  echo "  --force             allow restore while ports 8080/3000 are in use"
  echo "  --no-safety-backup  do not back up the current database before restoring"
}

if [ $# -lt 1 ]; then
  usage
  exit 1
fi

BACKUP_FILE="$1"
shift

while [ $# -gt 0 ]; do
  case "$1" in
    --force)
      FORCE=true
      ;;
    --no-safety-backup)
      NO_SAFETY_BACKUP=true
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      usage
      exit 1
      ;;
  esac
  shift
done

if [ ! -f "$BACKUP_FILE" ]; then
  echo "Backup file not found: $BACKUP_FILE" >&2
  exit 1
fi

"$PROJECT_DIR/scripts/verify-db.sh" "$BACKUP_FILE" >/dev/null

if [ "$FORCE" != "true" ]; then
  if lsof -i :8080 -P -n 2>/dev/null | grep -q "LISTEN" || lsof -i :3000 -P -n 2>/dev/null | grep -q "LISTEN"; then
    echo "Refusing to restore while MagicPodcast ports are in use."
    echo "Stop services first with: $PROJECT_DIR/scripts/stop.sh"
    echo "Or pass --force if you know what you are doing."
    exit 1
  fi
fi

mkdir -p "$(dirname "$DB_PATH")"

if [ -f "$DB_PATH" ] && [ "$NO_SAFETY_BACKUP" != "true" ]; then
  echo "Creating safety backup before restore..."
  BACKUP_DIR="$PROJECT_DIR/backend/data/backups" RETENTION_DAYS="${RETENTION_DAYS:-14}" "$PROJECT_DIR/scripts/backup-db.sh"
fi

echo "Restoring database..."
echo "  from: $BACKUP_FILE"
echo "  to:   $DB_PATH"

restore_tmp="$DB_PATH.restore.tmp"
rm -f "$restore_tmp"

if [[ "$BACKUP_FILE" == *.gz ]]; then
  if ! command -v gzip >/dev/null 2>&1; then
    echo "gzip is required but was not found." >&2
    exit 1
  fi
  gzip -cd "$BACKUP_FILE" > "$restore_tmp"
else
  cp "$BACKUP_FILE" "$restore_tmp"
fi

mv "$restore_tmp" "$DB_PATH"
rm -f "$DB_PATH-wal" "$DB_PATH-shm"

"$PROJECT_DIR/scripts/verify-db.sh" "$DB_PATH"
echo "Restore complete."
