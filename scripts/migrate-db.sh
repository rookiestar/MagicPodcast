#!/bin/bash
# Run the explicit, versioned MagicPodcast SQLite migration flow.

set -euo pipefail

PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BACKEND_DIR="$PROJECT_DIR/backend"
CONFIG_PATH="${CONFIG_PATH:-$BACKEND_DIR/configs/config.yaml}"
DB_PATH="${DB_PATH:-$BACKEND_DIR/data/magicpodcast.db}"
MIGRATION_CONFIRMATION="I_UNDERSTAND_THIS_WRITES_DATA"

usage() {
  cat <<'EOF'
用法：
  ./scripts/migrate-db.sh --dry-run
  MAGICPODCAST_MIGRATION_CONFIRM=I_UNDERSTAND_THIS_WRITES_DATA \
  ./scripts/migrate-db.sh --apply

--dry-run 只读取并展示当前数据库的迁移状态。
--apply   要求已确认的备份路径和确认字符串，然后应用版本化迁移。

环境变量：
  CONFIG_PATH                  后端配置文件，默认 backend/configs/config.yaml
  DB_PATH                      目标数据库，默认 backend/data/magicpodcast.db
  MAGICPODCAST_MIGRATION_BACKUP
                              已验证的近期备份文件（--apply 必填）
  MAGICPODCAST_MIGRATION_CONFIRM
                              必须精确为 I_UNDERSTAND_THIS_WRITES_DATA
EOF
}

mode=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    --dry-run)
      [ -z "$mode" ] || { echo "只能指定一个迁移模式。" >&2; exit 2; }
      mode="dry-run"
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
[ -f "$CONFIG_PATH" ] || { echo "配置文件不存在：$CONFIG_PATH" >&2; exit 1; }
[ -f "$DB_PATH" ] || { echo "数据库不存在：$DB_PATH" >&2; exit 1; }

cd "$BACKEND_DIR"

if [ "$mode" = "dry-run" ]; then
  CONFIG_PATH="$CONFIG_PATH" DB_PATH="$DB_PATH" MAGICPODCAST_DATABASE_PATH="$DB_PATH" go run ./cmd/migrate --dry-run
  exit $?
fi

if [ "${MAGICPODCAST_MIGRATION_CONFIRM:-}" != "$MIGRATION_CONFIRMATION" ]; then
  echo "拒绝执行真实迁移：请设置 MAGICPODCAST_MIGRATION_CONFIRM=$MIGRATION_CONFIRMATION" >&2
  exit 1
fi

backup="${MAGICPODCAST_MIGRATION_BACKUP:-}"
if [ -z "$backup" ] || [ ! -f "$backup" ]; then
  echo "拒绝执行真实迁移：MAGICPODCAST_MIGRATION_BACKUP 必须指向已验证的近期备份文件" >&2
  exit 1
fi

backup="$(cd "$(dirname "$backup")" && pwd)/$(basename "$backup")"
export CONFIG_PATH DB_PATH
export MAGICPODCAST_DATABASE_PATH="$DB_PATH"
export MAGICPODCAST_MIGRATION_BACKUP="$backup"
export MAGICPODCAST_MIGRATION_CONFIRM="$MIGRATION_CONFIRMATION"

if command -v lsof >/dev/null 2>&1; then
  for port in 3000 8080; do
    if lsof -tiTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1; then
      echo "拒绝执行真实迁移：端口 $port 仍有服务监听，请先停止前后端并再次确认" >&2
      exit 1
    fi
  done
fi

go run ./cmd/migrate --apply
