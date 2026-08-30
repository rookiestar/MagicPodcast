#!/bin/bash
# Run the explicit, versioned MagicPodcast SQLite migration flow.

set -euo pipefail

PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BACKEND_DIR="$PROJECT_DIR/backend"
CONFIG_PATH="${CONFIG_PATH:-$BACKEND_DIR/configs/config.yaml}"
DB_PATH="${DB_PATH:-$BACKEND_DIR/data/magicpodcast.db}"
RELEASE_ROOT="${MAGICPODCAST_RELEASE_ROOT:-$PROJECT_DIR/.magicpodcast-releases}"
TARGET_RELEASE_STAGE="${MAGICPODCAST_MIGRATION_RELEASE_STAGE:-}"
MIGRATION_CONFIRMATION="I_UNDERSTAND_THIS_WRITES_DATA"
RELEASE_CONFIRMATION="I_UNDERSTAND_THIS_SWITCHES_RELEASE"
source "$PROJECT_DIR/scripts/production-maintenance.sh"
STOP_SCRIPT="${MAGICPODCAST_MIGRATION_STOP_SCRIPT:-$PROJECT_DIR/scripts/stop.sh}"
RELEASE_SCRIPT="${MAGICPODCAST_MIGRATION_RELEASE_SCRIPT:-$PROJECT_DIR/scripts/release.sh}"
CURL_BIN="${MAGICPODCAST_CURL_BIN:-curl}"
SQLITE_BIN="${MAGICPODCAST_SQLITE_BIN:-sqlite3}"
GO_BIN="${MAGICPODCAST_GO_BIN:-go}"
LSOF_BIN="${MAGICPODCAST_LSOF_BIN:-$(command -v lsof || true)}"

usage() {
  cat <<'EOF'
用法：
  MAGICPODCAST_MIGRATION_BACKUP=/path/to/verified-backup.db.gz \
  ./scripts/migrate-db.sh --preflight
  MAGICPODCAST_MIGRATION_CONFIRM=I_UNDERSTAND_THIS_WRITES_DATA \
  ./scripts/migrate-db.sh --apply

--preflight 从已验证备份创建隔离副本，真实执行影子迁移并生成 Migration Report。
--dry-run   --preflight 的兼容别名，行为完全相同。
--apply   要求已确认的备份、通过报告、目标 release stage，以及数据库写和 release 切换两项确认。

环境变量：
  CONFIG_PATH                  后端配置文件，默认 backend/configs/config.yaml
  DB_PATH                      目标数据库，默认 backend/data/magicpodcast.db
  MAGICPODCAST_MIGRATION_BACKUP
                              已验证的近期备份文件（--preflight / --apply 必填）
  MAGICPODCAST_MIGRATION_REPORT
                              Migration Report 路径；preflight 默认写入 backend/data/migration-reports/latest.json
  MAGICPODCAST_TARGET_COMMIT  目标代码完整 SHA；默认读取当前 HEAD
  MAGICPODCAST_MIGRATION_RELEASE_STAGE
                              release.sh --prepare 生成的目标 release 绝对目录（--apply 必填）
  MAGICPODCAST_MIGRATION_CONFIRM
                              必须精确为 I_UNDERSTAND_THIS_WRITES_DATA
  MAGICPODCAST_MIGRATION_RELEASE_CONFIRM
                              必须精确为 I_UNDERSTAND_THIS_SWITCHES_RELEASE
EOF
}

migration_report_value() {
  local key="$1"
  awk -F'"' -v key="$key" '$2 == key { print $4; exit }' "$report"
}

manifest_value() {
  local key="$1"
  local file="$2"
  [ -f "$file" ] || return 1
  awk -F= -v key="$key" '$1 == key { sub(/^[^=]*=/, ""); print; exit }' "$file"
}

hash_file() {
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  elif command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    return 1
  fi
}

load_verified_migration_release() {
  [ -n "$TARGET_RELEASE_STAGE" ] && [ -d "$TARGET_RELEASE_STAGE" ] || {
    echo "拒绝执行真实迁移：必须提供已验证的目标 release stage" >&2
    return 1
  }
  local manifest backend_artifact frontend_build_id_file release_root
  local manifest_release manifest_frontend manifest_backend_sha manifest_commit manifest_clean
  local actual_backend_sha actual_frontend
  TARGET_RELEASE_STAGE="$(cd "$(dirname "$TARGET_RELEASE_STAGE")" && pwd)/$(basename "$TARGET_RELEASE_STAGE")"
  mkdir -p "$RELEASE_ROOT"
  release_root="$(cd "$RELEASE_ROOT" && pwd)"
  case "$TARGET_RELEASE_STAGE" in
    "$release_root"/*) ;;
    *) echo "拒绝执行真实迁移：目标 release stage 必须位于 release root" >&2; return 1 ;;
  esac
  manifest="$TARGET_RELEASE_STAGE/manifest.env"
  backend_artifact="$TARGET_RELEASE_STAGE/backend.api"
  frontend_build_id_file="$TARGET_RELEASE_STAGE/frontend.next/BUILD_ID"
  [ -f "$manifest" ] || {
    echo "拒绝执行真实迁移：目标 release manifest 不存在" >&2
    return 1
  }
  manifest_release="$(manifest_value release_id "$manifest")"
  manifest_frontend="$(manifest_value frontend_build_id "$manifest")"
  manifest_backend_sha="$(manifest_value backend_sha256 "$manifest")"
  manifest_commit="$(manifest_value commit "$manifest")"
  manifest_clean="$(manifest_value worktree_clean "$manifest")"
  if [ -z "$manifest_release" ] || [ -z "$manifest_frontend" ] || [ -z "$manifest_backend_sha" ] ||
    [ "$manifest_release" != "$(basename "$TARGET_RELEASE_STAGE")" ] || [ "$manifest_commit" != "$target_commit" ] ||
    [ "$manifest_clean" != true ]; then
    echo "拒绝执行真实迁移：目标 release 与目标 commit 未配对" >&2
    return 1
  fi
  [[ "$manifest_release" =~ ^[A-Za-z0-9._-]+$ ]] && [[ "$manifest_frontend" =~ ^[A-Za-z0-9._-]+$ ]] || {
    echo "拒绝执行真实迁移：release 标识无效" >&2
    return 1
  }
  [ -x "$backend_artifact" ] && [ -f "$frontend_build_id_file" ] || {
    echo "拒绝执行真实迁移：目标 release 产物不完整" >&2
    return 1
  }
  actual_backend_sha="$(hash_file "$backend_artifact")" || {
    echo "拒绝执行真实迁移：无法校验后端产物" >&2
    return 1
  }
  actual_frontend="$(tr -d '\r\n' < "$frontend_build_id_file")"
  if [ "$actual_backend_sha" != "$manifest_backend_sha" ] || [ "$actual_frontend" != "$manifest_frontend" ]; then
    echo "拒绝执行真实迁移：目标 stage 产物与 manifest 不一致" >&2
    return 1
  fi
  expected_release_id="$manifest_release"
  expected_frontend_build_id="$manifest_frontend"
}

verify_started_migration_service() {
  local expected_schema="$1"
  local attempts="${MAGICPODCAST_MIGRATION_HEALTH_ATTEMPTS:-30}"
  local interval="${MAGICPODCAST_MIGRATION_HEALTH_INTERVAL:-1}"
  local health=""
  local attempt=1
  [[ "$attempts" =~ ^[1-9][0-9]*$ ]] || return 1
  [[ "$interval" =~ ^[0-9]+$ ]] || return 1
  while [ "$attempt" -le "$attempts" ]; do
    health="$($CURL_BIN --fail --silent --show-error --max-time 5 http://127.0.0.1:8080/ready 2>/dev/null || true)"
    if printf '%s\n' "$health" | grep -Fq '"status":"ok"' &&
      printf '%s\n' "$health" | grep -Fq "\"schema_version\":$expected_schema" &&
      printf '%s\n' "$health" | grep -Fq "\"release_id\":\"$expected_release_id\"" &&
      printf '%s\n' "$health" | grep -Fq "\"frontend_build_id\":\"$expected_frontend_build_id\"" &&
      printf '%s\n' "$health" | grep -Fq '"build_mode":"release"' &&
      printf '%s\n' "$health" | grep -Fq '"data_profile":"production"' &&
      [ -n "$expected_release_id" ]; then
      return 0
    fi
    sleep "$interval"
    attempt=$((attempt + 1))
  done
  return 1
}

mode=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    --preflight)
      [ -z "$mode" ] || { echo "只能指定一个迁移模式。" >&2; exit 2; }
      mode="preflight"
      shift
      ;;
    --dry-run)
      [ -z "$mode" ] || { echo "只能指定一个迁移模式。" >&2; exit 2; }
      mode="preflight"
      shift
      ;;
    --apply)
      [ -z "$mode" ] || { echo "只能指定一个迁移模式。" >&2; exit 2; }
      mode="apply"
      shift
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      echo "未知参数：$1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

[ -n "$mode" ] || { usage >&2; exit 2; }
[ -f "$CONFIG_PATH" ] || { echo "迁移配置文件不存在" >&2; exit 1; }
[ -f "$DB_PATH" ] || { echo "迁移数据库不存在" >&2; exit 1; }

cd "$BACKEND_DIR"

backup="${MAGICPODCAST_MIGRATION_BACKUP:-}"
if [ -z "$backup" ] || [ ! -f "$backup" ] || [ ! -f "$backup.sha256" ]; then
  echo "拒绝执行迁移：MAGICPODCAST_MIGRATION_BACKUP 必须指向带 SHA-256 sidecar 的已验证近期备份" >&2
  exit 1
fi
backup="$(cd "$(dirname "$backup")" && pwd)/$(basename "$backup")"
target_commit="${MAGICPODCAST_TARGET_COMMIT:-$(git -C "$PROJECT_DIR" rev-parse HEAD)}"
target_commit="$(printf '%s' "$target_commit" | tr '[:upper:]' '[:lower:]')"
[[ "$target_commit" =~ ^[0-9a-f]{40}$ ]] || {
  echo "拒绝执行迁移：目标 commit 必须是完整 SHA" >&2
  exit 1
}

if [ "$mode" = "preflight" ]; then
  command -v "$GO_BIN" >/dev/null 2>&1 || {
    echo "拒绝执行迁移：缺少 Go 命令" >&2
    exit 1
  }
  report="${MAGICPODCAST_MIGRATION_REPORT:-$BACKEND_DIR/data/migration-reports/latest.json}"
  mkdir -p "$(dirname "$report")"
  CONFIG_PATH="$CONFIG_PATH" \
    MAGICPODCAST_DATABASE_PATH="$DB_PATH" \
    MAGICPODCAST_MIGRATION_BACKUP="$backup" \
    MAGICPODCAST_MIGRATION_REPORT="$report" \
    MAGICPODCAST_TARGET_COMMIT="$target_commit" \
    "$GO_BIN" run ./cmd/migrate --preflight
  exit $?
fi

if [ "${MAGICPODCAST_MIGRATION_CONFIRM:-}" != "$MIGRATION_CONFIRMATION" ]; then
  echo "拒绝执行真实迁移：请设置 MAGICPODCAST_MIGRATION_CONFIRM=$MIGRATION_CONFIRMATION" >&2
  exit 1
fi
if [ "${MAGICPODCAST_MIGRATION_RELEASE_CONFIRM:-}" != "$RELEASE_CONFIRMATION" ]; then
  echo "拒绝执行真实迁移：请单独确认目标 release 切换" >&2
  exit 1
fi

export CONFIG_PATH DB_PATH
export MAGICPODCAST_DATABASE_PATH="$DB_PATH"
export MAGICPODCAST_MIGRATION_BACKUP="$backup"
export MAGICPODCAST_MIGRATION_CONFIRM="$MIGRATION_CONFIRMATION"
export MAGICPODCAST_TARGET_COMMIT="$target_commit"
report="${MAGICPODCAST_MIGRATION_REPORT:-}"
[ -n "$report" ] && [ -f "$report" ] || {
  echo "拒绝执行真实迁移：MAGICPODCAST_MIGRATION_REPORT 必须指向通过的 preflight 报告" >&2
  exit 1
}
[ -f "$backup.meta" ] || {
  echo "拒绝执行真实迁移：备份元信息不存在" >&2
  exit 1
}
if [ -z "$LSOF_BIN" ] || ! command -v "$LSOF_BIN" >/dev/null 2>&1; then
  echo "拒绝执行真实迁移：缺少 lsof，无法确认服务端口已释放" >&2
  exit 1
fi
for required_command in "$GO_BIN" "$SQLITE_BIN" "$CURL_BIN"; do
  command -v "$required_command" >/dev/null 2>&1 || {
    echo "拒绝执行真实迁移：缺少必要的迁移命令" >&2
    exit 1
  }
done
[ -x "$STOP_SCRIPT" ] && [ -x "$RELEASE_SCRIPT" ] || {
  echo "拒绝执行真实迁移：停服或 release 脚本不可执行" >&2
  exit 1
}
expected_release_id=""
expected_frontend_build_id=""
load_verified_migration_release

maintenance_entered=false
hold_for_recovery=false
service_stopped=false
service_start_attempted=false
migration_cleanup() {
  local status="$?"
  local service_state=unknown
  trap - EXIT
  if [ "$maintenance_entered" = true ]; then
    if [ "$hold_for_recovery" = true ]; then
      if [ "$service_start_attempted" = true ]; then
        if "$STOP_SCRIPT"; then
          service_stopped=true
        else
          service_stopped=false
        fi
      fi
      if [ "$service_stopped" = true ]; then
        service_state=stopped
        for port in 3000 8080; do
          if "$LSOF_BIN" -tiTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1; then
            service_state=unknown
            break
          fi
        done
      fi
      if production_maintenance_hold_for_recovery; then
        echo "migration_recovery_required=true" >&2
        echo "migration_service_state=$service_state" >&2
        echo "migration_plan_id=$(migration_report_value plan_id)" >&2
        echo "migration_backup_sha256=$(migration_report_value backup_sha256)" >&2
      else
        echo "migration_recovery_lock_failed=true" >&2
      fi
    elif ! production_maintenance_finish; then
      status=1
    fi
  fi
  exit "$status"
}
trap migration_cleanup EXIT

production_maintenance_enter migration
maintenance_entered=true
hold_for_recovery=true

"$STOP_SCRIPT"
service_stopped=true

for port in 3000 8080; do
  if "$LSOF_BIN" -tiTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1; then
    service_stopped=false
    echo "拒绝执行真实迁移：端口 $port 仍有服务监听" >&2
    exit 1
  fi
done

# Re-bind the prepared artifacts after services stop and immediately before
# the first database write; a stage changed since the outer precheck is stale.
load_verified_migration_release
production_maintenance_mark_critical
"$GO_BIN" run ./cmd/migrate --apply

expected_schema="$($SQLITE_BIN -readonly "$DB_PATH" "SELECT COALESCE(MAX(version), 0) FROM schema_migrations;")"
[[ "$expected_schema" =~ ^[0-9]+$ ]] || {
  echo "迁移后 schema 无法读取" >&2
  exit 1
}

# The stage is activated only after a second digest/commit check. If it moved
# during apply, keep the committed database stopped for explicit recovery.
load_verified_migration_release
service_start_attempted=true
MAGICPODCAST_RELEASE_MAINTENANCE_OPERATION=migration \
  MAGICPODCAST_RELEASE_SCHEMA_VERSION_OVERRIDE="$expected_schema" \
  "$RELEASE_SCRIPT" --activate-prepared "$TARGET_RELEASE_STAGE"
service_stopped=false
verify_started_migration_service "$expected_schema" || {
  echo "迁移后服务验收失败" >&2
  exit 1
}

queue_projection="$($SQLITE_BIN -readonly "$DB_PATH" "SELECT queue_state || '=' || COUNT(*) FROM episode_triage_decisions WHERE queue_state IN ('inbox','focus','someday','done') GROUP BY queue_state ORDER BY queue_state;")"
processing_projection="$($SQLITE_BIN -readonly "$DB_PATH" "SELECT status || '=' || COUNT(*) FROM episode_processing_runs GROUP BY status ORDER BY status;")"
printf 'migration_post_start_schema=%s\n' "$expected_schema"
printf 'migration_post_start_release=%s\n' "$expected_release_id"
printf 'migration_post_start_frontend_build=%s\n' "$expected_frontend_build_id"
printf 'migration_post_start_queue_counts=%s\n' "$(printf '%s' "$queue_projection" | tr '\n' ',')"
printf 'migration_post_start_processing_counts=%s\n' "$(printf '%s' "$processing_projection" | tr '\n' ',')"
printf 'migration_service_state=running\n'

hold_for_recovery=false
