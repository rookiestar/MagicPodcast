#!/bin/bash
# Read a real existing database, including committed WAL pages; never create one.
sqlite_readonly() {
  local database="$1"
  shift
  [ -f "$database" ] || { echo 'Readonly database missing' >&2; return 1; }
  database="$(cd "$(dirname "$database")" && pwd)/$(basename "$database")" || return 1
  database="${database//%/%25}"
  database="${database//\?/%3F}"
  database="${database//#/%23}"
  "${MAGICPODCAST_SQLITE_BIN:-sqlite3}" -bail "file:$database?mode=ro" "$@"
}
if [ "${BASH_SOURCE[0]}" = "$0" ]; then
  set -euo pipefail
  [ "$#" -ge 2 ] || { echo 'Usage: sqlite-readonly.sh database SQL' >&2; exit 2; }
  sqlite_readonly "$@"
fi
