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
  echo "Database not found." >&2
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
echo "  source: magicpodcast_sqlite"
echo "  target: $(basename "$backup_file")"

sqlite3 "$DB_PATH" ".timeout 5000" ".backup '$tmp_file'"
mv "$tmp_file" "$backup_file"

sqlite3 "$backup_file" "PRAGMA journal_mode=DELETE;" >/dev/null
"$PROJECT_DIR/scripts/verify-db.sh" "$backup_file" >/dev/null
rm -f "$backup_file-wal" "$backup_file-shm"

uncompressed_size="$(wc -c < "$backup_file" | tr -d ' ')"
schema_version="$(sqlite3 -readonly "$backup_file" "SELECT COALESCE(MAX(version), 0) FROM schema_migrations;")"
code_commit="$(git -C "$PROJECT_DIR" rev-parse HEAD 2>/dev/null || printf 'unknown')"

table_count() {
  local table="$1"
  local exists
  exists="$(sqlite3 -readonly "$backup_file" "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='$table';")"
  if [ "$exists" = "1" ]; then
    sqlite3 -readonly "$backup_file" "SELECT COUNT(*) FROM $table;"
  else
    printf '0\n'
  fi
}

podcasts_count="$(table_count podcasts)"
episodes_count="$(table_count episodes)"
tags_count="$(table_count tags)"
triage_count="$(table_count episode_triage_decisions)"
completion_count="$(table_count episode_completions)"
processing_run_count="$(table_count episode_processing_runs)"
artifact_set_count="$(table_count episode_artifact_sets)"
delivery_count="$(table_count knowledge_deliveries)"
audio_asset_count="$(table_count episode_audio_assets)"

queue_count() {
  local state="$1"
  if [ "$triage_count" = "0" ]; then
    printf '0\n'
    return
  fi
  sqlite3 -readonly "$backup_file" "SELECT COUNT(*) FROM episode_triage_decisions WHERE queue_state='$state';"
}

queue_inbox_count="$(queue_count inbox)"
queue_focus_count="$(queue_count focus)"
queue_someday_count="$(queue_count someday)"
queue_done_count="$(queue_count "done")"

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
source_kind=magicpodcast_sqlite
compressed=$COMPRESS
retention_days=$RETENTION_DAYS
uncompressed_size_bytes=$uncompressed_size
size_bytes=$compressed_size
sha256=$(awk '{print $1}' "$final_file.sha256")
schema_version=$schema_version
target_commit=$code_commit
podcasts_count=$podcasts_count
episodes_count=$episodes_count
tags_count=$tags_count
episode_triage_decisions_count=$triage_count
episode_completions_count=$completion_count
episode_processing_runs_count=$processing_run_count
episode_artifact_sets_count=$artifact_set_count
knowledge_deliveries_count=$delivery_count
episode_audio_assets_count=$audio_asset_count
queue_inbox_count=$queue_inbox_count
queue_focus_count=$queue_focus_count
queue_someday_count=$queue_someday_count
queue_done_count=$queue_done_count
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

echo "Backup complete: $(basename "$final_file")"
