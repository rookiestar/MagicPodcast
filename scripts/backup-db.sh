#!/bin/bash
# Create a consistent SQLite backup for MagicPodcast.

set -euo pipefail

PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DB_PATH="${DB_PATH:-$PROJECT_DIR/backend/data/magicpodcast.db}"
BACKUP_DIR="${BACKUP_DIR:-$PROJECT_DIR/backend/data/backups}"
RETENTION_DAYS="${RETENTION_DAYS:-14}"
COMPRESS="${COMPRESS:-true}"
KEEP="${KEEP:-}"

if ! command -v sqlite3 >/dev/null 2>&1; then
  echo "sqlite3 is required but was not found." >&2
  exit 1
fi

if ! command -v shasum >/dev/null 2>&1; then
  echo "shasum is required but was not found." >&2
  exit 1
fi

if [ "$COMPRESS" = "true" ] && ! command -v gzip >/dev/null 2>&1; then
  echo "gzip is required but was not found." >&2
  exit 1
fi

if [ ! -f "$DB_PATH" ]; then
  echo "Database not found: $DB_PATH" >&2
  exit 1
fi

if ! [[ "$RETENTION_DAYS" =~ ^[0-9]+$ ]] || [ "$RETENTION_DAYS" -lt 1 ]; then
  echo "RETENTION_DAYS must be a positive integer." >&2
  exit 1
fi

mkdir -p "$BACKUP_DIR"

timestamp="$(date '+%Y%m%d_%H%M%S')"
backup_file="$BACKUP_DIR/magicpodcast_${timestamp}.db"
tmp_file="$backup_file.tmp"
final_file="$backup_file"

cleanup_tmp() {
  rm -f "$tmp_file" "$backup_file-wal" "$backup_file-shm"
}
trap cleanup_tmp EXIT

echo "Creating backup..."
echo "  source: $DB_PATH"
echo "  target: $backup_file"

sqlite3 "$DB_PATH" ".timeout 5000" ".backup '$tmp_file'"
mv "$tmp_file" "$backup_file"

sqlite3 "$backup_file" "PRAGMA journal_mode=DELETE;" >/dev/null
"$PROJECT_DIR/scripts/verify-db.sh" "$backup_file" >/dev/null
rm -f "$backup_file-wal" "$backup_file-shm"

uncompressed_size="$(wc -c < "$backup_file" | tr -d ' ')"

if [ "$COMPRESS" = "true" ]; then
  echo "Compressing backup..."
  gzip -9 "$backup_file"
  final_file="$backup_file.gz"
  gzip -t "$final_file"
fi

compressed_size="$(wc -c < "$final_file" | tr -d ' ')"
shasum -a 256 "$final_file" > "$final_file.sha256"

cat > "$final_file.meta" <<EOF
created_at=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
source=$DB_PATH
compressed=$COMPRESS
retention_days=$RETENTION_DAYS
uncompressed_size_bytes=$uncompressed_size
size_bytes=$compressed_size
sha256=$(awk '{print $1}' "$final_file.sha256")
EOF

remove_backup_family() {
  local backup="$1"
  rm -f "$backup" "$backup.sha256" "$backup.meta" "$backup-wal" "$backup-shm"
  if [[ "$backup" == *.gz ]]; then
    local raw_backup="${backup%.gz}"
    rm -f "$raw_backup" "$raw_backup.sha256" "$raw_backup.meta" "$raw_backup-wal" "$raw_backup-shm"
  fi
}

retention_mtime=$((RETENTION_DAYS - 1))
find "$BACKUP_DIR" -maxdepth 1 -type f \( -name 'magicpodcast_*.db' -o -name 'magicpodcast_*.db.gz' \) -mtime +"$retention_mtime" | while IFS= read -r old_backup; do
  [ -n "$old_backup" ] && remove_backup_family "$old_backup"
done

if [ -n "$KEEP" ]; then
  if ! [[ "$KEEP" =~ ^[0-9]+$ ]] || [ "$KEEP" -lt 1 ]; then
    echo "KEEP must be a positive integer when set." >&2
    exit 1
  fi
  find "$BACKUP_DIR" -maxdepth 1 -type f \( -name 'magicpodcast_*.db' -o -name 'magicpodcast_*.db.gz' \) | sort -r | tail -n +$((KEEP + 1)) | while IFS= read -r old_backup; do
    [ -n "$old_backup" ] && remove_backup_family "$old_backup"
  done
fi

echo "Backup complete: $final_file"
