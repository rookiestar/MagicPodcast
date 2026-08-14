#!/usr/bin/env bash
# Run an approved production deploy or rollback against the persistent
# production checkout, never against the GitHub Actions workspace.

set -euo pipefail

usage() {
  cat <<'EOF'
用法:
  ./scripts/production-deploy.sh deploy <40位 main SHA> [--dry-run]
  ./scripts/production-deploy.sh rollback [--dry-run]

环境变量:
  MAGICPODCAST_PRODUCTION_DIR  固定生产项目目录（必填）
  MAGICPODCAST_RELEASE_ROOT     发布产物目录，默认 <项目目录>/.magicpodcast-releases
EOF
}

fail() {
  printf 'production deploy failed: %s\n' "$1" >&2
  exit 1
}

ACTION="${1:-}"
shift || true
DRY_RUN=false
TARGET_SHA=""

case "$ACTION" in
  deploy)
    TARGET_SHA="${1:-}"
    shift || true
    ;;
  rollback)
    ;;
  --help|-h|"")
    usage
    exit 0
    ;;
  *)
    usage >&2
    exit 2
    ;;
esac

while [ "$#" -gt 0 ]; do
  case "$1" in
    --dry-run)
      DRY_RUN=true
      ;;
    *)
      usage >&2
      exit 2
      ;;
  esac
  shift
done

PROJECT_DIR="${MAGICPODCAST_PRODUCTION_DIR:-}"
[ -n "$PROJECT_DIR" ] || fail "MAGICPODCAST_PRODUCTION_DIR is required"
[[ "$PROJECT_DIR" = /* ]] || fail "production directory must be an absolute path"

GIT_BIN="${MAGICPODCAST_GIT_BIN:-git}"
CURL_BIN="${MAGICPODCAST_CURL_BIN:-curl}"
RELEASE_ROOT="${MAGICPODCAST_RELEASE_ROOT:-$PROJECT_DIR/.magicpodcast-releases}"
CURRENT_FILE="$RELEASE_ROOT/current.env"
SOURCE_STATE_FILE="$RELEASE_ROOT/source-state.env"
LOCK_DIR="${MAGICPODCAST_DEPLOY_LOCK_DIR:-/tmp/magicpodcast-production-deploy.lock}"

git_at() {
  "$GIT_BIN" -C "$PROJECT_DIR" "$@"
}

require_project() {
  [ -d "$PROJECT_DIR" ] || fail "production directory does not exist: $PROJECT_DIR"
  git_at rev-parse --show-toplevel >/dev/null 2>&1 || fail "production directory is not a Git worktree"
  [ -x "$PROJECT_DIR/scripts/release.sh" ] || fail "release.sh is missing or not executable"
  [ -x "$PROJECT_DIR/scripts/restart.sh" ] || fail "restart.sh is missing or not executable"
}

ensure_clean_worktree() {
  local status
  status="$(git_at status --porcelain=v1 --untracked-files=all)"
  [ -z "$status" ] || {
    printf '%s\n' "$status" >&2
    fail "production checkout has tracked or non-ignored changes"
  }
}

normalize_sha() {
  printf '%s' "$1" | tr '[:upper:]' '[:lower:]'
}

ensure_full_sha() {
  local sha="$1"
  [[ "$sha" =~ ^[0-9a-fA-F]{40}$ ]] || fail "deploy target must be a full 40-character commit SHA"
  sha="$(normalize_sha "$sha")"
  git_at cat-file -e "$sha^{commit}" 2>/dev/null || fail "commit is not available in the production checkout: $sha"
  printf '%s\n' "$sha"
}

validate_main_sha() {
  local sha="$1"
  [[ "$sha" =~ ^[0-9a-fA-F]{40}$ ]] || fail "deploy target must be a full 40-character commit SHA"
  sha="$(normalize_sha "$sha")"
  git_at fetch --no-tags origin main >/dev/null
  git_at cat-file -e "origin/main^{commit}" 2>/dev/null || fail "origin/main is unavailable after fetch"
  git_at cat-file -e "$sha^{commit}" 2>/dev/null || fail "commit is not available after fetching origin/main: $sha"
  git_at merge-base --is-ancestor "$sha" origin/main ||
    fail "deploy target is not an ancestor of origin/main: $sha"
  printf '%s\n' "$sha"
}

manifest_value() {
  local key="$1"
  local file="$2"
  [ -f "$file" ] || return 0
  awk -F= -v key="$key" '$1 == key { sub(/^[^=]*=/, ""); print; exit }' "$file"
}

write_source_state() {
  local previous_sha="$1"
  local current_sha="$2"
  local tmp
  mkdir -p "$RELEASE_ROOT"
  umask 077
  tmp="$(mktemp "$SOURCE_STATE_FILE.tmp.XXXXXX")"
  cat > "$tmp" <<EOF
previous_source_sha=$previous_sha
current_source_sha=$current_sha
updated_at=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
EOF
  mv "$tmp" "$SOURCE_STATE_FILE"
}

read_source_state() {
  local key="$1"
  manifest_value "$key" "$SOURCE_STATE_FILE"
}

restore_source() {
  local sha="$1"
  git_at cat-file -e "$sha^{commit}" 2>/dev/null || fail "source recovery commit is unavailable: $sha"
  git_at checkout --detach "$sha" >/dev/null
}

acquire_lock() {
  if ! mkdir "$LOCK_DIR" 2>/dev/null; then
    fail "another production deploy is already running: $LOCK_DIR"
  fi
  printf '%s\n' "$$" > "$LOCK_DIR/pid"
}

release_lock() {
  if [ -d "$LOCK_DIR" ]; then
    rm -f "$LOCK_DIR/pid"
    rmdir "$LOCK_DIR" 2>/dev/null || true
  fi
}

export_production_environment() {
  export MAGICPODCAST_PROJECT_DIR="$PROJECT_DIR"
  export MAGICPODCAST_DATA_PROFILE=production
  export MAGICPODCAST_PRODUCTION_PROFILE_CONFIRM=I_UNDERSTAND_THIS_USES_PRODUCTION_DATA
  export MAGICPODCAST_SERVER_MODE=release
  export MAGICPODCAST_DATABASE_DEBUG=false
}

run_release_dry_run() {
  export_production_environment
  (cd "$PROJECT_DIR" && "$PROJECT_DIR/scripts/release.sh" --dry-run)
}

run_release() {
  export_production_environment
  (cd "$PROJECT_DIR" && "$PROJECT_DIR/scripts/restart.sh" --prod)
}

run_rollback() {
  export_production_environment
  (cd "$PROJECT_DIR" && "$PROJECT_DIR/scripts/release.sh" --rollback)
}

verify_production_health() {
  local release_id frontend_build_id health
  release_id="$(manifest_value release_id "$CURRENT_FILE")"
  frontend_build_id="$(manifest_value frontend_build_id "$CURRENT_FILE")"
  [ -n "$release_id" ] || fail "current release metadata has no release_id"
  [ -n "$frontend_build_id" ] || fail "current release metadata has no frontend_build_id"
  health="$("$CURL_BIN" --fail --silent --show-error --max-time 10 http://127.0.0.1:8080/health)" ||
    fail "production /health is unavailable"
  printf '%s\n' "$health" | grep -Fq '"status":"ok"' || fail "production /health is not ok"
  printf '%s\n' "$health" | grep -Fq "\"release_id\":\"$release_id\"" ||
    fail "production /health release_id does not match current.env"
  printf '%s\n' "$health" | grep -Fq "\"frontend_build_id\":\"$frontend_build_id\"" ||
    fail "production /health frontend_build_id does not match current.env"
  printf '%s\n' "$health" | grep -Fq '"build_mode":"release"' ||
    fail "production /health is not release mode"
  printf '%s\n' "$health" | grep -Fq '"data_profile":"production"' ||
    fail "production /health is not using production data profile"
  "$CURL_BIN" --fail --silent --show-error --max-time 10 http://127.0.0.1:3000 >/dev/null ||
    fail "production frontend is unavailable"
  printf '%s\n' "$health"
}

ORIGINAL_SOURCE_SHA=""
SOURCE_SWITCHED=false
RELEASE_SUCCEEDED=false
OPERATION_SUCCEEDED=false

finish() {
  local status=$?
  if [ "$ACTION" = deploy ] && [ "$SOURCE_SWITCHED" = true ] && [ "$OPERATION_SUCCEEDED" = false ]; then
    if [ "$RELEASE_SUCCEEDED" = true ]; then
      printf 'post-release verification failed; attempting paired artifact rollback\n' >&2
      if ! run_rollback; then
        printf 'paired artifact rollback failed; inspect production before retrying\n' >&2
        status=1
      fi
    fi
    printf 'deployment failed; restoring production checkout to %s\n' "$ORIGINAL_SOURCE_SHA" >&2
    if restore_source "$ORIGINAL_SOURCE_SHA"; then
      printf 'production checkout restored\n' >&2
    else
      printf 'production checkout restoration failed; inspect it before the next deploy\n' >&2
      status=1
    fi
  fi
  release_lock
  exit "$status"
}

require_project
ensure_clean_worktree
acquire_lock
trap finish EXIT

ORIGINAL_SOURCE_SHA="$(git_at rev-parse HEAD)"

if [ "$ACTION" = deploy ]; then
  TARGET_SHA="$(validate_main_sha "$TARGET_SHA")"
  printf 'deploy target: %s\n' "$TARGET_SHA"
  printf 'production checkout: %s\n' "$PROJECT_DIR"

  restore_source "$TARGET_SHA"
  SOURCE_SWITCHED=true
  run_release_dry_run
  if [ "$DRY_RUN" = true ]; then
    restore_source "$ORIGINAL_SOURCE_SHA"
    SOURCE_SWITCHED=false
    OPERATION_SUCCEEDED=true
    printf 'dry-run complete; production services were not changed\n'
    exit 0
  fi

  run_release
  RELEASE_SUCCEEDED=true
  verify_production_health
  write_source_state "$ORIGINAL_SOURCE_SHA" "$TARGET_SHA"
  SOURCE_SWITCHED=false
  OPERATION_SUCCEEDED=true
  printf 'production deploy complete\n'
  exit 0
fi

CURRENT_SOURCE_SHA="$(read_source_state current_source_sha)"
PREVIOUS_SOURCE_SHA="$(read_source_state previous_source_sha)"
[ -n "$CURRENT_SOURCE_SHA" ] && [ -n "$PREVIOUS_SOURCE_SHA" ] ||
  fail "source-state.env is missing; run one managed deploy before using the rollback workflow"
[ "$CURRENT_SOURCE_SHA" = "$ORIGINAL_SOURCE_SHA" ] ||
  fail "production checkout HEAD does not match source-state.env"
git_at fetch --no-tags origin main >/dev/null
ensure_full_sha "$PREVIOUS_SOURCE_SHA" >/dev/null
printf 'rollback source: %s -> %s\n' "$CURRENT_SOURCE_SHA" "$PREVIOUS_SOURCE_SHA"
run_release_dry_run
if [ "$DRY_RUN" = true ]; then
  printf 'dry-run complete; production services and checkout were not changed\n'
  exit 0
fi

run_rollback
verify_production_health
restore_source "$PREVIOUS_SOURCE_SHA"
write_source_state "$CURRENT_SOURCE_SHA" "$PREVIOUS_SOURCE_SHA"
OPERATION_SUCCEEDED=true
printf 'production rollback complete\n'
