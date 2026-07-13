#!/bin/bash
# Decrypt an off-device backup into a temporary database and verify it.

set -euo pipefail

PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"
PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
AGE_BIN="${MAGICPODCAST_AGE_BIN:-age}"
IDENTITY_FILE="${MAGICPODCAST_AGE_IDENTITY_FILE:-}"
STATUS_FILE="${MAGICPODCAST_RESTORE_DRILL_STATUS_FILE:-$PROJECT_DIR/logs/restore-drill.status}"

fail() {
  echo "restore drill failed: $1" >&2
  exit 1
}

[ "$#" -eq 1 ] || fail "usage: restore-drill.sh backup.db.gz.age"
encrypted_file="$1"
[ -f "$encrypted_file" ] || fail "encrypted backup not found"
[ -n "$IDENTITY_FILE" ] && [ -f "$IDENTITY_FILE" ] || fail "MAGICPODCAST_AGE_IDENTITY_FILE is not configured"
command -v "$AGE_BIN" >/dev/null 2>&1 || fail "age executable was not found"
command -v gzip >/dev/null 2>&1 || fail "gzip is required"
command -v shasum >/dev/null 2>&1 || fail "shasum is required"

if [ -f "$encrypted_file.sha256" ]; then
  (cd "$(dirname "$encrypted_file")" && shasum -a 256 -c "$(basename "$encrypted_file").sha256") >/dev/null ||
    fail "encrypted backup checksum failed"
fi

drill_dir="$(mktemp -d "${TMPDIR:-/tmp}/magicpodcast-restore-drill.XXXXXX")"
cleanup() { rm -rf "$drill_dir"; }
trap cleanup EXIT
decrypted="$drill_dir/backup.db.gz"

"$AGE_BIN" -d -i "$IDENTITY_FILE" -o "$decrypted" "$encrypted_file" || fail "age decryption failed"
gzip -t "$decrypted" || fail "decrypted backup gzip check failed"
"$PROJECT_DIR/scripts/verify-db.sh" "$decrypted" > "$drill_dir/verify.out" || fail "temporary database verification failed"

mkdir -p "$(dirname "$STATUS_FILE")"
umask 077
cat > "$STATUS_FILE" <<EOF
status=ok
source=$(basename "$encrypted_file")
completed_at=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
EOF
echo "restore drill passed: $(basename "$encrypted_file")"
