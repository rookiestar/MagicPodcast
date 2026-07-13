#!/bin/bash
# Encrypt one verified local backup to an explicitly configured off-device directory.

set -euo pipefail

PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BACKUP_DIR="${MAGICPODCAST_BACKUP_DIR:-$PROJECT_DIR/backend/data/backups}"
OFFSITE_DIR="${MAGICPODCAST_OFFSITE_DIR:-}"
RECIPIENT_FILE="${MAGICPODCAST_AGE_RECIPIENT_FILE:-}"
AGE_BIN="${MAGICPODCAST_AGE_BIN:-age}"
OFFSITE_KEEP="${MAGICPODCAST_OFFSITE_KEEP:-14}"
STATUS_FILE="${MAGICPODCAST_OFFSITE_STATUS_FILE:-$PROJECT_DIR/logs/offsite-backup.status}"

fail() {
  echo "offsite backup failed: $1" >&2
  exit 1
}

hash_file() {
  shasum -a 256 "$1" | awk '{print $1}'
}

write_status() {
  local state="$1"
  local backup_name="$2"
  local encrypted_name="$3"
  local encrypted_sha="$4"
  mkdir -p "$(dirname "$STATUS_FILE")"
  umask 077
  cat > "$STATUS_FILE" <<EOF
status=$state
backup=$backup_name
encrypted=$encrypted_name
encrypted_sha256=$encrypted_sha
updated_at=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
EOF
}

[ -n "$OFFSITE_DIR" ] || fail "MAGICPODCAST_OFFSITE_DIR is not configured"
[ -n "$RECIPIENT_FILE" ] || fail "MAGICPODCAST_AGE_RECIPIENT_FILE is not configured"
command -v "$AGE_BIN" >/dev/null 2>&1 || fail "age executable was not found"
command -v shasum >/dev/null 2>&1 || fail "shasum is required"
command -v gzip >/dev/null 2>&1 || fail "gzip is required"
[ -f "$RECIPIENT_FILE" ] || fail "recipient file not found"

if ! [[ "$OFFSITE_KEEP" =~ ^[0-9]+$ ]] || [ "$OFFSITE_KEEP" -lt 1 ]; then
  fail "MAGICPODCAST_OFFSITE_KEEP must be a positive integer"
fi

recipient="$(tr -d '[:space:]' < "$RECIPIENT_FILE")"
case "$recipient" in
  age1*) ;;
  *) fail "recipient file does not contain an age recipient" ;;
esac

backup_file="${1:-}"
if [ -z "$backup_file" ]; then
  backup_file="$(find "$BACKUP_DIR" -maxdepth 1 -type f -name 'magicpodcast_*.db.gz' | sort -r | head -1 || true)"
fi
[ -n "$backup_file" ] && [ -f "$backup_file" ] || fail "local compressed backup not found"
case "$backup_file" in
  "$BACKUP_DIR"/*) ;;
  *) fail "backup must be inside the configured local backup directory" ;;
esac

gzip -t "$backup_file" || fail "local backup gzip check failed"
if [ -f "$backup_file.sha256" ]; then
  (cd "$(dirname "$backup_file")" && shasum -a 256 -c "$(basename "$backup_file").sha256") >/dev/null ||
    fail "local backup checksum failed"
fi

mkdir -p "$OFFSITE_DIR"
offsite_real="$(cd "$OFFSITE_DIR" && pwd -P)"
backup_real="$(cd "$(dirname "$backup_file")" && pwd -P)"
[ "$offsite_real" != "$backup_real" ] || fail "offsite directory must differ from local backup directory"

backup_name="$(basename "$backup_file")"
encrypted_name="$backup_name.age"
encrypted_file="$OFFSITE_DIR/$encrypted_name"
tmp_dir="$(mktemp -d "${TMPDIR:-/tmp}/magicpodcast-offsite.XXXXXX")"
tmp_file="$tmp_dir/$encrypted_name"
cloud_tmp_file="$encrypted_file.tmp.$$"
cloud_checksum_tmp="$encrypted_file.sha256.tmp.$$"
cloud_meta_tmp="$encrypted_file.meta.tmp.$$"

cleanup() {
  rm -rf "$tmp_dir"
  rm -f "$cloud_tmp_file" "$cloud_checksum_tmp" "$cloud_meta_tmp" 2>/dev/null || true
}
trap cleanup EXIT

"$AGE_BIN" -r "$recipient" -o "$tmp_file" "$backup_file" || fail "age encryption failed"
[ -s "$tmp_file" ] || fail "age produced an empty file"
chmod 600 "$tmp_file"
cp "$tmp_file" "$cloud_tmp_file" || fail "could not copy encrypted backup to offsite directory"
mv "$cloud_tmp_file" "$encrypted_file" || fail "could not finalize encrypted offsite backup"

encrypted_sha="$(hash_file "$encrypted_file")"
printf '%s  %s\n' "$encrypted_sha" "$encrypted_name" > "$tmp_dir/$encrypted_name.sha256"
chmod 600 "$tmp_dir/$encrypted_name.sha256"
cp "$tmp_dir/$encrypted_name.sha256" "$cloud_checksum_tmp" || fail "could not copy encrypted checksum to offsite directory"
mv "$cloud_checksum_tmp" "$encrypted_file.sha256" || fail "could not finalize encrypted checksum"

encrypted_size="$(wc -c < "$encrypted_file" | tr -d ' ')"
backup_sha="$(hash_file "$backup_file")"
umask 077
cat > "$tmp_dir/$encrypted_name.meta" <<EOF
created_at=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
backup=$backup_name
backup_sha256=$backup_sha
encrypted_size_bytes=$encrypted_size
encrypted_sha256=$encrypted_sha
EOF
chmod 600 "$tmp_dir/$encrypted_name.meta"
cp "$tmp_dir/$encrypted_name.meta" "$cloud_meta_tmp" || fail "could not copy encrypted metadata to offsite directory"
mv "$cloud_meta_tmp" "$encrypted_file.meta" || fail "could not finalize encrypted metadata"

# Some macOS cloud file providers reject the metadata calls that `find` makes
# while still allowing exact-path reads and shell glob expansion. Enumerate
# only the script-owned backup names so retention does not turn a successful
# upload into a failed launchd run.
shopt -s nullglob
offsite_backups=("$OFFSITE_DIR"/magicpodcast_*.db.gz.age)
if [ "${#offsite_backups[@]}" -gt "$OFFSITE_KEEP" ]; then
  IFS=$'\n' sorted_backups=($(printf '%s\n' "${offsite_backups[@]}" | sort -r))
  for old_file in "${sorted_backups[@]:OFFSITE_KEEP}"; do
    rm -f "$old_file" "$old_file.sha256" "$old_file.meta" ||
      echo "warning: could not remove expired offsite backup: $old_file" >&2
  done
fi

write_status ok "$backup_name" "$encrypted_name" "$encrypted_sha"
echo "encrypted offsite backup complete: $encrypted_name"
