#!/bin/bash
# Restore a MagicPodcast SQLite backup.

set -euo pipefail

PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source "$PROJECT_DIR/scripts/production-maintenance.sh"
DB_PATH="${DB_PATH:-$PROJECT_DIR/backend/data/magicpodcast.db}"
NO_SAFETY_BACKUP=false
LSOF_BIN="${MAGICPODCAST_LSOF_BIN:-$(command -v lsof || true)}"
maintenance_entered=false
hold_for_recovery=false
restore_tmp=""

restore_cleanup() {
  local status="$?"
  trap - EXIT
  [ -z "$restore_tmp" ] || rm -f "$restore_tmp"
  if [ "$maintenance_entered" = true ]; then
    if [ "$hold_for_recovery" = true ]; then
      production_maintenance_hold_for_recovery || status=1
    else
      production_maintenance_finish || status=1
    fi
  fi
  exit "$status"
}
trap restore_cleanup EXIT

usage() {
  echo "Usage: $0 <backup.db> [--no-safety-backup]"
  echo ""
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

prior_maintenance_state="$(production_maintenance_value state 2>/dev/null || true)"
production_maintenance_enter recovery
maintenance_entered=true
if [ "$prior_maintenance_state" = critical ] || [ "$prior_maintenance_state" = recovery_required ]; then
  hold_for_recovery=true
fi

"$PROJECT_DIR/scripts/verify-db.sh" "$BACKUP_FILE" >/dev/null

if [ -z "$LSOF_BIN" ] || ! command -v "$LSOF_BIN" >/dev/null 2>&1; then
  echo "Refusing to restore without lsof port verification." >&2
  exit 1
fi
if "$LSOF_BIN" -i :8080 -P -n 2>/dev/null | grep -q "LISTEN" ||
  "$LSOF_BIN" -i :3000 -P -n 2>/dev/null | grep -q "LISTEN"; then
  echo "Refusing to restore while MagicPodcast ports are in use."
  echo "Stop services first with: $PROJECT_DIR/scripts/stop.sh"
  exit 1
fi

mkdir -p "$(dirname "$DB_PATH")"

if [ -f "$DB_PATH" ] && [ "$NO_SAFETY_BACKUP" != "true" ]; then
  echo "Creating safety backup before restore..."
  BACKUP_DIR="$PROJECT_DIR/backend/data/backups" RETENTION_DAYS="${RETENTION_DAYS:-14}" "$PROJECT_DIR/scripts/backup-db.sh"
fi

echo "Restoring database..."
echo "  from: $BACKUP_FILE"
echo "  to:   $DB_PATH"

production_maintenance_mark_critical
hold_for_recovery=true
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
echo "Recovery lock retained; activate and verify a schema-paired prepared release before resuming service."
