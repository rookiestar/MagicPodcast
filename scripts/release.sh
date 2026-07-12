#!/bin/bash
# Build, verify, switch and roll back one paired MagicPodcast production release.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="${MAGICPODCAST_PROJECT_DIR:-$(cd "$SCRIPT_DIR/.." && pwd)}"
FRONTEND_DIR="$PROJECT_DIR/frontend"
BACKEND_DIR="$PROJECT_DIR/backend"
RELEASE_ROOT="${MAGICPODCAST_RELEASE_ROOT:-$PROJECT_DIR/.magicpodcast-releases}"
RELEASE_LOG="${MAGICPODCAST_RELEASE_LOG:-$PROJECT_DIR/logs/release.log}"
CURRENT_FILE="$RELEASE_ROOT/current.env"
PREVIOUS_FILE="$RELEASE_ROOT/previous.env"
BACKEND_PID_FILE="${MAGICPODCAST_BACKEND_PID_FILE:-/tmp/magicpodcast-backend.pid}"
FRONTEND_PID_FILE="${MAGICPODCAST_FRONTEND_PID_FILE:-/tmp/magicpodcast-frontend.pid}"
START_SCRIPT="${MAGICPODCAST_START_SCRIPT:-$PROJECT_DIR/scripts/start.sh}"
STOP_SCRIPT="${MAGICPODCAST_STOP_SCRIPT:-$PROJECT_DIR/scripts/stop.sh}"
GO_BIN="${MAGICPODCAST_GO_BIN:-go}"
NPM_BIN="${MAGICPODCAST_NPM_BIN:-npm}"
TEST_MODE="${MAGICPODCAST_RELEASE_TEST_MODE:-false}"

for bin_dir in /opt/homebrew/bin /usr/local/bin; do
  if [ -d "$bin_dir" ]; then
    PATH="$bin_dir:$PATH"
  fi
done
export PATH

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

usage() {
  cat <<'EOF'
用法:
  ./scripts/release.sh --prod       构建、验证并切换生产版本
  ./scripts/release.sh --prepare    只构建并验证，不停止当前服务
  ./scripts/release.sh --rollback   用单一步骤恢复上一版本
  ./scripts/release.sh --dry-run    只显示目标和当前状态，不写入或停止服务
EOF
}

now() { date -u '+%Y-%m-%dT%H:%M:%SZ'; }

log() {
  local level="$1"
  shift
  mkdir -p "$(dirname "$RELEASE_LOG")"
  printf '%s [%s] %s\n' "$(now)" "$level" "$*" >> "$RELEASE_LOG"
}

info() { printf '%b%s%b\n' "$GREEN" "$*" "$NC"; }
warn() { printf '%b%s%b\n' "$YELLOW" "$*" "$NC" >&2; }
error() { printf '%b%s%b\n' "$RED" "$*" "$NC" >&2; }

hash_file() {
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  else
    sha256sum "$1" | awk '{print $1}'
  fi
}

manifest_value() {
  local key="$1"
  local file="$2"
  [ -f "$file" ] || return 0
  awk -F= -v key="$key" '$1 == key { sub(/^[^=]*=/, ""); print; exit }' "$file"
}

write_manifest() {
  local file="$1"
  local release_id="$2"
  local frontend_build_id="$3"
  local backend_sha="$4"
  local commit="$5"
  umask 077
  cat > "$file" <<EOF
release_id=$release_id
frontend_build_id=$frontend_build_id
backend_sha256=$backend_sha
commit=$commit
created_at=$(now)
EOF
}

write_pointer() {
  local file="$1"
  local release_id="$2"
  local frontend_build_id="$3"
  local backend_sha="$4"
  local artifact_dir="$5"
  umask 077
  cat > "$file" <<EOF
release_id=$release_id
frontend_build_id=$frontend_build_id
backend_sha256=$backend_sha
artifact_dir=$artifact_dir
updated_at=$(now)
EOF
}

pid_cwd() {
  local pid="$1"
  lsof -a -p "$pid" -d cwd -Fn 2>/dev/null |
    awk '/^n/ { sub(/^n/, ""); print; exit }'
}

pid_command() {
  local pid="$1"
  ps -p "$pid" -o command= 2>/dev/null || true
}

listener_pid() {
  local port="$1"
  lsof -tiTCP:"$port" -sTCP:LISTEN 2>/dev/null | head -1 || true
}

process_is_expected() {
  local role="$1"
  local pid="$2"
  local expected_cwd="$3"
  local port="$4"
  local command

  [ -n "$pid" ] || return 1
  [ "$(pid_cwd "$pid")" = "$expected_cwd" ] || return 1
  command="$(pid_command "$pid")"
  case "$role" in
    backend) [[ "$command" == *"api"* ]] || return 1 ;;
    frontend) [[ "$command" == *"next-server"* ]] || return 1 ;;
    *) return 1 ;;
  esac
  lsof -nP -a -p "$pid" -iTCP:"$port" -sTCP:LISTEN 2>/dev/null |
    grep -Eq "TCP (127\.0\.0\.1|\[::1\]):$port \(LISTEN\)"
}

verify_current_processes() {
  if [ "$TEST_MODE" = true ]; then
    log INFO "process identity check skipped in explicit test mode"
    return 0
  fi

  local backend_listener frontend_listener
  backend_listener="$(listener_pid 8080)"
  frontend_listener="$(listener_pid 3000)"
  if ! process_is_expected backend "$backend_listener" "$BACKEND_DIR" 8080; then
    error "拒绝切换：8080 当前进程不是可确认的 MagicPodcast 后端"
    log ERROR "process identity check failed for backend"
    return 1
  fi
  if ! process_is_expected frontend "$frontend_listener" "$FRONTEND_DIR" 3000; then
    error "拒绝切换：3000 当前进程不是可确认的 MagicPodcast 前端"
    log ERROR "process identity check failed for frontend"
    return 1
  fi

  if [ -f "$BACKEND_PID_FILE" ] && [ "$(cat "$BACKEND_PID_FILE")" != "$backend_listener" ]; then
    error "拒绝切换：后端 PID 文件与监听者不一致"
    log ERROR "backend pid file does not match listener"
    return 1
  fi
  if [ -f "$FRONTEND_PID_FILE" ] && [ "$(cat "$FRONTEND_PID_FILE")" != "$frontend_listener" ]; then
    error "拒绝切换：前端 PID 文件与监听者不一致"
    log ERROR "frontend pid file does not match listener"
    return 1
  fi
}

wait_for_ports_gone() {
  [ "$TEST_MODE" = true ] && return 0
  for _ in 1 2 3 4 5 6 7 8 9 10; do
    if [ -z "$(listener_pid 8080)" ] && [ -z "$(listener_pid 3000)" ]; then
      return 0
    fi
    sleep 1
  done
  return 1
}

verify_health() {
  local release_id="$1"
  local frontend_build_id="$2"
  local health

  if [ "$TEST_MODE" = true ]; then
    if [ -n "${MAGICPODCAST_RELEASE_TEST_HEALTH_FILE:-}" ]; then
      health="$(cat "$MAGICPODCAST_RELEASE_TEST_HEALTH_FILE")"
      printf '%s\n' "$health" | grep -Fq "release_id=$release_id" || return 1
      printf '%s\n' "$health" | grep -Fq "frontend_build_id=$frontend_build_id" || return 1
    fi
    return 0
  fi

  health="$(curl --fail --silent --show-error http://127.0.0.1:8080/health 2>/dev/null)" || return 1
  printf '%s\n' "$health" | grep -Fq "\"status\":\"ok\"" || return 1
  printf '%s\n' "$health" | grep -Fq "\"release_id\":\"$release_id\"" || return 1
  printf '%s\n' "$health" | grep -Fq "\"frontend_build_id\":\"$frontend_build_id\"" || return 1
}

build_release() {
  local release_id="$1"
  local stage="$RELEASE_ROOT/$release_id"
  local frontend_dist_name=".next-release-$release_id"
  local frontend_dist="$FRONTEND_DIR/$frontend_dist_name"
  local frontend_build_id backend_sha commit

  mkdir -p "$stage"
  log INFO "build started release=$release_id"

  if ! (cd "$BACKEND_DIR" && "$GO_BIN" build -o "$stage/backend.api" ./cmd/api > "$stage/backend-build.log" 2>&1); then
    log ERROR "backend build failed release=$release_id"
    error "后端构建失败；当前运行版本未停止"
    return 1
  fi
  if [ ! -x "$stage/backend.api" ]; then
    log ERROR "backend artifact missing release=$release_id"
    error "后端构建未生成可执行产物；当前运行版本未停止"
    return 1
  fi

  rm -rf "$frontend_dist"
  if ! (cd "$FRONTEND_DIR" && MAGICPODCAST_NEXT_DIST_DIR="$frontend_dist_name" "$NPM_BIN" run build > "$stage/frontend-build.log" 2>&1); then
    log ERROR "frontend build failed release=$release_id"
    rm -rf "$frontend_dist"
    error "前端构建失败；当前运行版本未停止"
    return 1
  fi
  if [ ! -f "$frontend_dist/BUILD_ID" ]; then
    log ERROR "frontend BUILD_ID missing release=$release_id"
    rm -rf "$frontend_dist"
    error "前端构建未生成 BUILD_ID；当前运行版本未停止"
    return 1
  fi

  frontend_build_id="$(tr -d '\r\n' < "$frontend_dist/BUILD_ID")"
  backend_sha="$(hash_file "$stage/backend.api")"
  commit="$(git -C "$PROJECT_DIR" rev-parse --short HEAD 2>/dev/null || printf 'nogit')"
  mv "$frontend_dist" "$stage/frontend.next"
  write_manifest "$stage/manifest.env" "$release_id" "$frontend_build_id" "$backend_sha" "$commit"
  log INFO "build verified release=$release_id frontend_build_id=$frontend_build_id"
  printf '%s\n' "$stage"
}

verify_stage() {
  local stage="$1"
  local release_id frontend_build_id backend_sha
  release_id="$(manifest_value release_id "$stage/manifest.env")"
  frontend_build_id="$(manifest_value frontend_build_id "$stage/manifest.env")"
  backend_sha="$(manifest_value backend_sha256 "$stage/manifest.env")"
  [ -x "$stage/backend.api" ] || return 1
  [ -d "$stage/frontend.next" ] || return 1
  [ -f "$stage/frontend.next/BUILD_ID" ] || return 1
  [ "$release_id" = "$(basename "$stage")" ] || return 1
  [ "$frontend_build_id" = "$(tr -d '\r\n' < "$stage/frontend.next/BUILD_ID")" ] || return 1
  [ "$backend_sha" = "$(hash_file "$stage/backend.api")" ] || return 1
}

stop_services() {
  log INFO "switch stop requested"
  if ! "$STOP_SCRIPT" >> "$RELEASE_LOG" 2>&1; then
    log ERROR "stop script failed"
    return 1
  fi
  wait_for_ports_gone || {
    log ERROR "ports remained after stop"
    return 1
  }
}

start_services() {
  local release_id="$1"
  local frontend_build_id="$2"
  log INFO "start requested release=$release_id"
  if ! MAGICPODCAST_RELEASE_ID="$release_id" \
    MAGICPODCAST_FRONTEND_BUILD_ID="$frontend_build_id" \
    "$START_SCRIPT" --prod --no-build >> "$RELEASE_LOG" 2>&1; then
    log ERROR "start failed release=$release_id"
    return 1
  fi
}

PREVIOUS_DIR=""
PREVIOUS_ID=""
PREVIOUS_FRONTEND_ID=""
PREVIOUS_BACKEND_SHA=""
OLD_BACKEND_MOVED=false
OLD_FRONTEND_MOVED=false
NEW_BACKEND_INSTALLED=false
NEW_FRONTEND_INSTALLED=false
ACTIVE_RELEASE_ID=""
ACTIVE_FRONTEND_ID=""

capture_previous_artifacts() {
  local current_id="$1"
  local frontend_id="$2"
  local backend_sha="$3"
  PREVIOUS_ID="$current_id"
  PREVIOUS_FRONTEND_ID="$frontend_id"
  PREVIOUS_BACKEND_SHA="$backend_sha"
  PREVIOUS_DIR="$RELEASE_ROOT/${current_id}-previous-$(date -u '+%Y%m%dT%H%M%SZ')-$$"
  mkdir -p "$PREVIOUS_DIR"

  if ! mv "$BACKEND_DIR/api" "$PREVIOUS_DIR/backend.api"; then
    return 1
  fi
  OLD_BACKEND_MOVED=true
  if ! mv "$FRONTEND_DIR/.next" "$PREVIOUS_DIR/frontend.next"; then
    mv "$PREVIOUS_DIR/backend.api" "$BACKEND_DIR/api" || true
    OLD_BACKEND_MOVED=false
    return 1
  fi
  OLD_FRONTEND_MOVED=true
  write_manifest "$PREVIOUS_DIR/manifest.env" "$current_id" "$frontend_id" "$backend_sha" "previous"
  write_pointer "$PREVIOUS_FILE" "$current_id" "$frontend_id" "$backend_sha" "$PREVIOUS_DIR"
}

install_stage() {
  local stage="$1"
  if ! mv "$stage/backend.api" "$BACKEND_DIR/api"; then
    return 1
  fi
  NEW_BACKEND_INSTALLED=true
  if ! mv "$stage/frontend.next" "$FRONTEND_DIR/.next"; then
    return 1
  fi
  NEW_FRONTEND_INSTALLED=true
}

restore_previous() {
  local failed_dir="$RELEASE_ROOT/${ACTIVE_RELEASE_ID}-failed-$(date -u '+%Y%m%dT%H%M%SZ')-$$"
  mkdir -p "$failed_dir"

  if [ "$NEW_FRONTEND_INSTALLED" = true ] && [ -d "$FRONTEND_DIR/.next" ]; then
    mv "$FRONTEND_DIR/.next" "$failed_dir/frontend.next" || true
  fi
  if [ "$NEW_BACKEND_INSTALLED" = true ] && [ -f "$BACKEND_DIR/api" ]; then
    mv "$BACKEND_DIR/api" "$failed_dir/backend.api" || true
  fi

  if [ "$OLD_BACKEND_MOVED" = true ] && [ ! -f "$BACKEND_DIR/api" ]; then
    mv "$PREVIOUS_DIR/backend.api" "$BACKEND_DIR/api" || return 1
  fi
  if [ "$OLD_FRONTEND_MOVED" = true ] && [ ! -d "$FRONTEND_DIR/.next" ]; then
    mv "$PREVIOUS_DIR/frontend.next" "$FRONTEND_DIR/.next" || return 1
  fi

  write_pointer "$CURRENT_FILE" "$PREVIOUS_ID" "$PREVIOUS_FRONTEND_ID" "$PREVIOUS_BACKEND_SHA" "$PREVIOUS_DIR"
  log WARN "rollback artifacts restored release=$PREVIOUS_ID failed_dir=$failed_dir"
  if ! start_services "$PREVIOUS_ID" "$PREVIOUS_FRONTEND_ID"; then
    log ERROR "rollback start failed release=$PREVIOUS_ID"
    return 1
  fi
  if ! verify_health "$PREVIOUS_ID" "$PREVIOUS_FRONTEND_ID"; then
    log ERROR "rollback health verification failed release=$PREVIOUS_ID"
    return 1
  fi
  log INFO "rollback completed release=$PREVIOUS_ID"
}

deploy_release() {
  local stage="$1"
  local release_id frontend_id backend_sha current_id current_frontend_id current_backend_sha

  release_id="$(manifest_value release_id "$stage/manifest.env")"
  frontend_id="$(manifest_value frontend_build_id "$stage/manifest.env")"
  backend_sha="$(manifest_value backend_sha256 "$stage/manifest.env")"
  ACTIVE_RELEASE_ID="$release_id"
  ACTIVE_FRONTEND_ID="$frontend_id"

  verify_current_processes || return 1
  current_id="$(manifest_value release_id "$CURRENT_FILE")"
  current_frontend_id="$(manifest_value frontend_build_id "$CURRENT_FILE")"
  current_backend_sha="$(manifest_value backend_sha256 "$CURRENT_FILE")"
  if [ -z "$current_id" ]; then
    current_id="legacy-$(hash_file "$BACKEND_DIR/api" | cut -c1-16)"
  fi
  if [ -z "$current_frontend_id" ] && [ -f "$FRONTEND_DIR/.next/BUILD_ID" ]; then
    current_frontend_id="$(tr -d '\r\n' < "$FRONTEND_DIR/.next/BUILD_ID")"
  fi
  if [ -z "$current_backend_sha" ]; then
    current_backend_sha="$(hash_file "$BACKEND_DIR/api")"
  fi

  if ! stop_services; then
    log WARN "stop incomplete; attempting to keep previous release available"
    start_services "$current_id" "$current_frontend_id" || true
    return 1
  fi
  if ! capture_previous_artifacts "$current_id" "$current_frontend_id" "$current_backend_sha"; then
    error "无法保存上一版本，未切换新版本"
    log ERROR "failed to retain previous artifacts"
    if [ "$OLD_BACKEND_MOVED" = true ] && [ ! -f "$BACKEND_DIR/api" ]; then
      mv "$PREVIOUS_DIR/backend.api" "$BACKEND_DIR/api" || true
    fi
    if [ "$OLD_FRONTEND_MOVED" = true ] && [ ! -d "$FRONTEND_DIR/.next" ]; then
      mv "$PREVIOUS_DIR/frontend.next" "$FRONTEND_DIR/.next" || true
    fi
    start_services "$current_id" "$current_frontend_id" || true
    return 1
  fi
  if ! install_stage "$stage"; then
    error "新版本切换失败，开始自动回退"
    log ERROR "install failed release=$release_id"
    restore_previous || return 1
    return 1
  fi

  write_pointer "$CURRENT_FILE" "$release_id" "$frontend_id" "$backend_sha" "$RELEASE_ROOT/$release_id"
  log INFO "switch installed release=$release_id"
  if ! start_services "$release_id" "$frontend_id" || ! verify_health "$release_id" "$frontend_id"; then
    error "新版本启动或健康验证失败，开始自动回退"
    log ERROR "post-switch verification failed release=$release_id"
    restore_previous || return 1
    return 1
  fi
  log INFO "switch completed release=$release_id"
  info "发布完成: release=$release_id frontend_build_id=$frontend_id"
}

rollback_command() {
  local previous_id previous_frontend_id previous_backend_sha previous_dir
  previous_id="$(manifest_value release_id "$PREVIOUS_FILE")"
  previous_frontend_id="$(manifest_value frontend_build_id "$PREVIOUS_FILE")"
  previous_backend_sha="$(manifest_value backend_sha256 "$PREVIOUS_FILE")"
  previous_dir="$(manifest_value artifact_dir "$PREVIOUS_FILE")"
  [ -n "$previous_id" ] && [ -n "$previous_dir" ] || {
    error "没有可回退的上一版本"
    return 1
  }
  [ -f "$previous_dir/backend.api" ] && [ -d "$previous_dir/frontend.next" ] || {
    error "上一版本产物不完整，拒绝回退"
    return 1
  }

  ACTIVE_RELEASE_ID="$(manifest_value release_id "$CURRENT_FILE")"
  ACTIVE_FRONTEND_ID="$(manifest_value frontend_build_id "$CURRENT_FILE")"
  PREVIOUS_ID="$previous_id"
  PREVIOUS_FRONTEND_ID="$previous_frontend_id"
  PREVIOUS_BACKEND_SHA="$previous_backend_sha"
  PREVIOUS_DIR="$previous_dir"
  OLD_BACKEND_MOVED=true
  OLD_FRONTEND_MOVED=true
  NEW_BACKEND_INSTALLED=true
  NEW_FRONTEND_INSTALLED=true

  verify_current_processes || return 1
  if ! stop_services; then
    return 1
  fi
  if [ -f "$BACKEND_DIR/api" ]; then
    mv "$BACKEND_DIR/api" "$RELEASE_ROOT/${ACTIVE_RELEASE_ID}-rollback-failed-$(date -u '+%Y%m%dT%H%M%SZ')-$$.api" || return 1
  fi
  if [ -d "$FRONTEND_DIR/.next" ]; then
    mv "$FRONTEND_DIR/.next" "$RELEASE_ROOT/${ACTIVE_RELEASE_ID}-rollback-failed-$(date -u '+%Y%m%dT%H%M%SZ')-$$.next" || return 1
  fi
  mv "$previous_dir/backend.api" "$BACKEND_DIR/api" || return 1
  mv "$previous_dir/frontend.next" "$FRONTEND_DIR/.next" || return 1
  write_pointer "$CURRENT_FILE" "$previous_id" "$previous_frontend_id" "$previous_backend_sha" "$previous_dir"
  if ! start_services "$previous_id" "$previous_frontend_id" || ! verify_health "$previous_id" "$previous_frontend_id"; then
    error "回退后的健康检查失败"
    log ERROR "manual rollback verification failed release=$previous_id"
    return 1
  fi
  log INFO "manual rollback completed release=$previous_id"
  info "回退完成: release=$previous_id frontend_build_id=$previous_frontend_id"
}

MODE="deploy"
DRY_RUN=false
while [ "$#" -gt 0 ]; do
  case "$1" in
    --prod|--production) MODE="deploy" ;;
    --prepare) MODE="prepare" ;;
    --rollback) MODE="rollback" ;;
    --dry-run) DRY_RUN=true ;;
    --help|-h) usage; exit 0 ;;
    *) usage >&2; exit 2 ;;
  esac
  shift
done

if [ "$DRY_RUN" = true ]; then
  printf 'release_root=%s\n' "$RELEASE_ROOT"
  printf 'project_root=%s\n' "$PROJECT_DIR"
  printf 'current_release=%s\n' "$(manifest_value release_id "$CURRENT_FILE")"
  printf 'previous_release=%s\n' "$(manifest_value release_id "$PREVIOUS_FILE")"
  printf 'no mutation performed\n'
  exit 0
fi

mkdir -p "$RELEASE_ROOT"
if [ "$MODE" = rollback ]; then
  rollback_command
  exit $?
fi

RELEASE_ID="$(date -u '+%Y%m%dT%H%M%SZ')-$(git -C "$PROJECT_DIR" rev-parse --short HEAD 2>/dev/null || printf 'nogit')-$$"
STAGE="$(build_release "$RELEASE_ID")" || exit $?
if ! verify_stage "$STAGE"; then
  log ERROR "staged release verification failed release=$RELEASE_ID"
  error "新版本最低验证失败；当前运行版本未停止"
  exit 1
fi

if [ "$MODE" = prepare ]; then
  info "构建验证完成，未切换: release=$RELEASE_ID"
  exit 0
fi

deploy_release "$STAGE"
