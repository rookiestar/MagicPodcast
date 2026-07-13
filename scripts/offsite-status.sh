#!/bin/bash
# Read-only status check for the configured encrypted off-device backup mirror.

set -euo pipefail

PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BACKUP_DIR="${MAGICPODCAST_BACKUP_DIR:-$PROJECT_DIR/backend/data/backups}"
OFFSITE_DIR="${MAGICPODCAST_OFFSITE_DIR:-}"
MAX_AGE_HOURS="${MAGICPODCAST_OFFSITE_MAX_AGE_HOURS:-26}"
STATUS_FILE="${MAGICPODCAST_OFFSITE_STATUS_FILE:-$PROJECT_DIR/logs/offsite-backup.status}"

mtime() {
  stat -f '%m' "$1" 2>/dev/null || stat -c '%Y' "$1"
}

if [ -z "$OFFSITE_DIR" ]; then
  echo "status=unconfigured"
  exit 2
fi
if ! [[ "$MAX_AGE_HOURS" =~ ^[0-9]+$ ]] || [ "$MAX_AGE_HOURS" -lt 1 ]; then
  echo "status=invalid_config"
  exit 1
fi
if [ ! -d "$OFFSITE_DIR" ]; then
  echo "status=destination_missing"
  exit 1
fi

latest_local="$(find "$BACKUP_DIR" -maxdepth 1 -type f -name 'magicpodcast_*.db.gz' | sort -r | head -1 || true)"
if [ -z "$latest_local" ]; then
  echo "status=local_backup_missing"
  exit 1
fi

local_name="$(basename "$latest_local")"
encrypted="$OFFSITE_DIR/$local_name.age"
if [ ! -f "$encrypted" ]; then
  echo "status=encrypted_copy_missing backup=$local_name"
  exit 1
fi

if [ ! -f "$encrypted.sha256" ] || ! (cd "$OFFSITE_DIR" && shasum -a 256 -c "$(basename "$encrypted").sha256") >/dev/null 2>&1; then
  echo "status=encrypted_copy_checksum_failed backup=$local_name"
  exit 1
fi

local_mtime="$(mtime "$latest_local")"
encrypted_mtime="$(mtime "$encrypted")"
now_epoch="$(date '+%s')"
max_age_seconds=$((MAX_AGE_HOURS * 3600))
if [ "$encrypted_mtime" -lt "$local_mtime" ] || [ $((now_epoch - encrypted_mtime)) -gt "$max_age_seconds" ]; then
  echo "status=encrypted_copy_stale backup=$local_name"
  exit 1
fi

if [ -f "$STATUS_FILE" ] && grep -q '^status=error$' "$STATUS_FILE"; then
  echo "status=last_sync_failed backup=$local_name"
  exit 1
fi

echo "status=ok backup=$local_name encrypted=$(basename "$encrypted")"
