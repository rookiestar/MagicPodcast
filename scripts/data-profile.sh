#!/usr/bin/env bash
# Manage MagicPodcast's local fixture/snapshot data profiles.

set -euo pipefail

PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PROFILE_CLI_DIR="${MAGICPODCAST_DATA_PROFILE_CLI_DIR:-${TMPDIR:-/tmp}/magicpodcast-data-profile-cli}"
PROFILE_CLI="$PROFILE_CLI_DIR/data-profile"
PROFILE_CLI_TMP="$PROFILE_CLI.tmp.$$"
trap 'rm -f "$PROFILE_CLI_TMP"' EXIT

if [ "${1:-}" = "use" ] && [ "${2:-}" = "production" ]; then
  echo "data-profile: production profile cannot be selected by the local command" >&2
  exit 1
fi

mkdir -p "$PROFILE_CLI_DIR"
chmod 700 "$PROFILE_CLI_DIR"

cd "$PROJECT_DIR/backend"
go build -o "$PROFILE_CLI_TMP" ./cmd/data-profile
chmod 700 "$PROFILE_CLI_TMP"
mv -f "$PROFILE_CLI_TMP" "$PROFILE_CLI"
exec "$PROFILE_CLI" --project-dir "$PROJECT_DIR" "$@"
