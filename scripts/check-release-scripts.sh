#!/bin/bash
# Run only isolated release tests; never start the application's services.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
for tool in node sqlite3 git lsof python3; do
  command -v "$tool" >/dev/null || { echo "Missing test dependency: $tool" >&2; exit 1; }
done
printf 'platform=%s node=%s sqlite=%s\n' "$(uname -s)" "$(node --version)" "$(sqlite3 --version)"
for script in scripts/{production-*,release,start,stop,restart,service-supervisor,migrate-db,restore-db,backup-db,verify-db}.sh; do
  bash -n "$script"
done
bash -n scripts/sqlite-readonly.sh
node --test scripts/__tests__/production-maintenance.test.mjs scripts/__tests__/release-reliability.test.mjs
