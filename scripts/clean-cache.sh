#!/bin/bash
# Clean local build caches and generated runtime artifacts.

set -euo pipefail

PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

CLEAN_FRONTEND=false
CLEAN_BACKEND=false
CLEAN_WORKSPACE=false
DEEP=false
DRY_RUN=false

usage() {
  cat <<'EOF'
Usage: ./scripts/clean-cache.sh [--all|--frontend|--backend|--workspace] [--deep] [--dry-run]

Options:
  --all       Clean frontend and backend generated files. Default when no target is set.
  --frontend  Clean frontend build and tool caches.
  --backend   Clean backend build outputs and local generated binaries.
  --workspace Clean stray root-level generated files.
  --deep      Also clean deeper dependency/tool caches and stray root node_modules.
  --dry-run   Print what would be removed without deleting files.
EOF
}

for arg in "$@"; do
  case "$arg" in
    --all)
      CLEAN_FRONTEND=true
      CLEAN_BACKEND=true
      CLEAN_WORKSPACE=true
      ;;
    --frontend)
      CLEAN_FRONTEND=true
      ;;
    --backend)
      CLEAN_BACKEND=true
      ;;
    --workspace)
      CLEAN_WORKSPACE=true
      ;;
    --deep)
      DEEP=true
      ;;
    --dry-run)
      DRY_RUN=true
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      echo "Unknown option: $arg" >&2
      usage >&2
      exit 1
      ;;
  esac
done

if [ "$CLEAN_FRONTEND" = false ] && [ "$CLEAN_BACKEND" = false ] && [ "$CLEAN_WORKSPACE" = false ]; then
  CLEAN_FRONTEND=true
  CLEAN_BACKEND=true
  CLEAN_WORKSPACE=true
fi

remove_path() {
  local target="$1"
  if [ -e "$target" ]; then
    if [ "$DRY_RUN" = true ]; then
      echo "would remove ${target#$PROJECT_DIR/}"
    else
      rm -rf "$target"
      echo "removed ${target#$PROJECT_DIR/}"
    fi
  fi
}

clean_frontend() {
  remove_path "$PROJECT_DIR/frontend/.next"
  remove_path "$PROJECT_DIR/frontend/out"
  remove_path "$PROJECT_DIR/frontend/.eslintcache"
  if [ "$DRY_RUN" = true ]; then
    find "$PROJECT_DIR/frontend" -maxdepth 1 -name "*.tsbuildinfo" -type f -print |
      sed "s#^$PROJECT_DIR/#would remove #"
  else
    find "$PROJECT_DIR/frontend" -maxdepth 1 -name "*.tsbuildinfo" -type f -delete
  fi

  if [ "$DEEP" = true ]; then
    remove_path "$PROJECT_DIR/frontend/node_modules/.cache"
  fi
}

clean_workspace() {
  find "$PROJECT_DIR" \
    \( -path "$PROJECT_DIR/.git" \
    -o -path "$PROJECT_DIR/node_modules" \
    -o -path "$PROJECT_DIR/frontend/node_modules" \
    -o -path "$PROJECT_DIR/frontend/.next" \
    -o -path "$PROJECT_DIR/backend/data" \) -prune \
    -o -name ".DS_Store" -type f -print |
    while IFS= read -r target; do
      remove_path "$target"
    done

  remove_path "$PROJECT_DIR/api_server.log"
  remove_path "$PROJECT_DIR/api_server.pid"

  if [ "$DEEP" = true ] && [ -d "$PROJECT_DIR/node_modules" ] && [ ! -f "$PROJECT_DIR/package.json" ]; then
    remove_path "$PROJECT_DIR/node_modules"
  fi
}

clean_backend() {
  remove_path "$PROJECT_DIR/backend/tmp"
  remove_path "$PROJECT_DIR/backend/bin"
  remove_path "$PROJECT_DIR/backend/api"
  remove_path "$PROJECT_DIR/backend/api_server"
  remove_path "$PROJECT_DIR/backend/api_server.pid"
  remove_path "$PROJECT_DIR/backend/api_server.log"
}

if [ "$CLEAN_FRONTEND" = true ]; then
  clean_frontend
fi

if [ "$CLEAN_BACKEND" = true ]; then
  clean_backend
fi

if [ "$CLEAN_WORKSPACE" = true ]; then
  clean_workspace
fi

if [ "$DRY_RUN" = true ]; then
  echo "dry run complete"
else
  echo "clean complete"
fi
