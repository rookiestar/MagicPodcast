#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../../.." && pwd)"

cd "$REPO_ROOT"

echo "MagicPodcast change verification"
echo
git status --short
git diff --check

changed_files="$({
  git diff --name-only
  git diff --cached --name-only
  git ls-files --others --exclude-standard
} | sort -u)"

if [ -z "$changed_files" ]; then
  echo "No changed files detected."
  exit 0
fi

echo
echo "Changed files:"
printf '%s\n' "$changed_files"

if printf '%s\n' "$changed_files" | grep -q '^backend/'; then
  echo
  echo "Running backend tests..."
  (cd backend && go test ./...)
  echo
  echo "Running backend static checks..."
  (cd backend && go vet ./...)
fi

if printf '%s\n' "$changed_files" | grep -q '^frontend/'; then
  if [ ! -d frontend/node_modules ]; then
    echo "frontend/node_modules is missing; refusing to install dependencies automatically." >&2
    exit 2
  fi

  echo
  echo "Running frontend type check..."
  (cd frontend && npm run type-check)
  echo
  echo "Running frontend tests..."
  (cd frontend && npm run test:run)
fi

if printf '%s\n' "$changed_files" | grep -qE '(^|/)scripts/.*\.sh$'; then
  echo
  echo "Checking changed shell scripts..."
  while IFS= read -r script; do
    [ -n "$script" ] && [ -f "$script" ] && bash -n "$script"
  done <<EOF
$(printf '%s\n' "$changed_files" | grep -E '(^|/)scripts/.*\.sh$' || true)
EOF
fi

echo
echo "Relevant verification passed."
