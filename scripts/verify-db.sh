#!/bin/bash
# Verify a MagicPodcast SQLite database file.

set -euo pipefail

PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
INPUT_PATH="${1:-$PROJECT_DIR/backend/data/magicpodcast.db}"
DB_PATH="$INPUT_PATH"
TEMP_DB=""

if ! command -v sqlite3 >/dev/null 2>&1; then
  echo "sqlite3 is required but was not found." >&2
  exit 1
fi

if [ ! -f "$INPUT_PATH" ]; then
  echo "Database not found: $INPUT_PATH" >&2
  exit 1
fi

cleanup() {
  if [ -n "$TEMP_DB" ]; then
    rm -f "$TEMP_DB" "$TEMP_DB-wal" "$TEMP_DB-shm"
  fi
}
trap cleanup EXIT

if [[ "$INPUT_PATH" == *.gz ]]; then
  if ! command -v gzip >/dev/null 2>&1; then
    echo "gzip is required but was not found." >&2
    exit 1
  fi
  TEMP_DB="$(mktemp "${TMPDIR:-/tmp}/magicpodcast-verify.XXXXXX.db")"
  gzip -cd "$INPUT_PATH" > "$TEMP_DB"
  DB_PATH="$TEMP_DB"
fi

sqlite() {
  sqlite3 -readonly "$DB_PATH" "$1"
}

integrity="$(sqlite "PRAGMA integrity_check;")"
if [ "$integrity" != "ok" ]; then
  echo "Integrity check failed:"
  echo "$integrity"
  exit 1
fi

required_tables=(
  podcasts
  episodes
  tags
  podcasts_tags
  workflows
  jobs
  job_executions
  reports
  sync_configs
)

for table in "${required_tables[@]}"; do
  exists="$(sqlite "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='$table';")"
  if [ "$exists" != "1" ]; then
    echo "Missing required table: $table" >&2
    exit 1
  fi
done

foreign_key_issues="$(sqlite "PRAGMA foreign_key_check;")"
if [ -n "$foreign_key_issues" ]; then
  echo "Foreign key issues found:"
  echo "$foreign_key_issues"
  exit 1
fi

podcasts="$(sqlite "SELECT COUNT(*) FROM podcasts;")"
episodes="$(sqlite "SELECT COUNT(*) FROM episodes;")"
tags="$(sqlite "SELECT COUNT(*) FROM tags;")"
workflows="$(sqlite "SELECT COUNT(*) FROM workflows;")"

echo "Database OK: $INPUT_PATH"
echo "  podcasts:  $podcasts"
echo "  episodes:  $episodes"
echo "  tags:      $tags"
echo "  workflows: $workflows"
