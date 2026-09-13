#!/bin/bash
# MagicPodcast health check.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="${MAGICPODCAST_PROJECT_DIR:-$(cd "$SCRIPT_DIR/.." && pwd)}"
DB_FILE="${MAGICPODCAST_DB_FILE:-$PROJECT_DIR/backend/data/magicpodcast.db}"
BACKUP_DIR="${MAGICPODCAST_BACKUP_DIR:-$PROJECT_DIR/backend/data/backups}"
BACKEND_HEALTH_URL="${MAGICPODCAST_BACKEND_HEALTH_URL:-http://localhost:8080/health}"
FRONTEND_URL="${MAGICPODCAST_FRONTEND_URL:-http://localhost:3000}"
CURL_BIN="${MAGICPODCAST_CURL_BIN:-curl}"
HTTP_TIMEOUT_SECONDS="${MAGICPODCAST_HEALTH_HTTP_TIMEOUT:-5}"
MAX_REDIRECTS="${MAGICPODCAST_HEALTH_MAX_REDIRECTS:-3}"

case "$HTTP_TIMEOUT_SECONDS" in
  ""|*[!0-9]*) echo "MAGICPODCAST_HEALTH_HTTP_TIMEOUT must be a positive integer." >&2; exit 2 ;;
esac
if [ "$HTTP_TIMEOUT_SECONDS" -le 0 ]; then
  echo "MAGICPODCAST_HEALTH_HTTP_TIMEOUT must be a positive integer." >&2
  exit 2
fi
case "$MAX_REDIRECTS" in
  ""|*[!0-9]*) echo "MAGICPODCAST_HEALTH_MAX_REDIRECTS must be a non-negative integer." >&2; exit 2 ;;
esac

# shellcheck disable=SC1091
source "$SCRIPT_DIR/sqlite-readonly.sh"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

ok() { echo -e "  ${GREEN}✓${NC} $1"; }
warn() { echo -e "  ${YELLOW}⚠${NC} $1"; }
fail() { echo -e "  ${RED}✗${NC} $1"; }

listener_pid() {
  local port="$1"
  lsof -ti :"$port" -sTCP:LISTEN 2>/dev/null | head -1 || true
}

url_port() {
  local url="$1"
  local authority="${url#*://}"
  authority="${authority%%/*}"
  authority="${authority##*@}"
  case "$authority" in
    \[*\]:*) printf '%s\n' "${authority##*]:}" ;;
    *:*) printf '%s\n' "${authority##*:}" ;;
    *)
      case "$url" in
        https://*) printf '443\n' ;;
        *) printf '80\n' ;;
      esac
      ;;
  esac
}

url_origin() {
  case "$1" in
    http://*|https://*) printf '%s\n' "$1" | sed -E 's#^(https?://[^/]+).*#\1#' ;;
    *) return 1 ;;
  esac
}

is_auth_redirect() {
  case "$1" in
    */cdn-cgi/access/login|*/cdn-cgi/access/login\?*|*/cdn-cgi/access/login\#*) return 0 ;;
    */oauth/authorize|*/oauth/authorize\?*|*/oauth/authorize\#*) return 0 ;;
    */oauth2/authorize|*/oauth2/authorize\?*|*/oauth2/authorize\#*) return 0 ;;
    *) return 1 ;;
  esac
}

http_status() {
  local current="$1"
  local origin
  origin="$(url_origin "$current" 2>/dev/null || true)"

  for _ in $(seq 0 "$MAX_REDIRECTS"); do
    local probe code redirect redirect_origin
    probe="$("$CURL_BIN" --silent --show-error \
      --connect-timeout "$HTTP_TIMEOUT_SECONDS" \
      --max-time "$HTTP_TIMEOUT_SECONDS" \
      --max-redirs 0 \
      --output /dev/null \
      --write-out '%{http_code}\t%{redirect_url}' \
      "$current" 2>/dev/null || true)"
    IFS=$'\t' read -r code redirect <<< "$probe"
    code="${code:-000}"
    if [[ "$code" != 3[0-9][0-9] ]] || [ -z "$redirect" ]; then
      printf '%s\n' "$code"
      return
    fi

    if is_auth_redirect "$redirect"; then
      # Authentication pages are not application health, even when the
      # identity provider uses the same origin.
      printf '%s\n' "$code"
      return
    fi
    redirect_origin="$(url_origin "$redirect" 2>/dev/null || true)"
    if [ -z "$origin" ] || [ "$redirect_origin" != "$origin" ]; then
      # Do not follow external or authentication redirects. Returning the
      # redirect status keeps a 200 response at the other origin from being
      # mistaken for a healthy local frontend.
      printf '%s\n' "$code"
      return
    fi
    current="$redirect"
  done

  printf '%s\n' "${code:-310}"
}

echo -e "${BLUE}========================================"
echo "  MagicPodcast 健康检查"
echo "  $(date '+%Y-%m-%d %H:%M:%S')"
echo -e "========================================${NC}"
echo ""

issues=0

echo -e "${YELLOW}[1] 服务端口${NC}"
echo "-------------------------------------------"
backend_port="$(url_port "$BACKEND_HEALTH_URL")"
frontend_port="$(url_port "$FRONTEND_URL")"
backend_pid="$(listener_pid "$backend_port")"
frontend_pid="$(listener_pid "$frontend_port")"

if [ -n "$backend_pid" ]; then
  backend_cmd="$(ps -p "$backend_pid" -o command= 2>/dev/null || echo unknown)"
  ok "后端端口 $backend_port 正在监听 [PID: $backend_pid, $backend_cmd]"
else
  warn "后端端口 $backend_port 未监听"
fi

if [ -n "$frontend_pid" ]; then
  frontend_cmd="$(ps -p "$frontend_pid" -o command= 2>/dev/null || echo unknown)"
  ok "前端端口 $frontend_port 正在监听 [PID: $frontend_pid, $frontend_cmd]"

  # shellcheck disable=SC2009
  if ps -axo command= | grep -F "$PROJECT_DIR/frontend" | grep -q "next dev"; then
    fail "前端正在开发模式运行，公网访问会加载开发资源。请运行: $PROJECT_DIR/scripts/restart.sh"
    issues=$((issues + 1))
  else
    ok "前端未发现开发模式进程"
  fi
else
  warn "前端端口 $frontend_port 未监听"
fi
echo ""

echo -e "${YELLOW}[2] HTTP 健康检查${NC}"
echo "-------------------------------------------"
backend_health="$("$CURL_BIN" --silent --show-error --connect-timeout "$HTTP_TIMEOUT_SECONDS" --max-time "$HTTP_TIMEOUT_SECONDS" "$BACKEND_HEALTH_URL" 2>/dev/null || true)"
if echo "$backend_health" | grep -q '"status":"ok"'; then
  ok "后端 /health 正常: $backend_health"
else
  fail "后端 /health 无法确认正常"
  [ -n "$backend_health" ] && echo "    响应: $backend_health"
  issues=$((issues + 1))
fi

frontend_status="$(http_status "$FRONTEND_URL")"
if [ "$frontend_status" = "200" ]; then
  ok "前端首页正常: HTTP $frontend_status"
elif [ -n "$frontend_pid" ]; then
  warn "前端端口存在，但首页返回 HTTP ${frontend_status:-N/A}"
  issues=$((issues + 1))
else
  warn "前端未运行，跳过页面检查"
fi
echo ""

echo -e "${YELLOW}[3] 数据库${NC}"
echo "-------------------------------------------"
if [ -f "$DB_FILE" ]; then
  size="$(du -sh "$DB_FILE" | cut -f1)"
  ok "数据库文件存在: $DB_FILE ($size)"

  if command -v "${MAGICPODCAST_SQLITE_BIN:-sqlite3}" >/dev/null 2>&1; then
    integrity="$(sqlite_readonly "$DB_FILE" "PRAGMA integrity_check;" 2>/dev/null || true)"
    if [ "$integrity" = "ok" ]; then
      ok "SQLite integrity_check 通过"
    else
      fail "SQLite integrity_check 失败: ${integrity:-N/A}"
      issues=$((issues + 1))
    fi

    podcasts="$(sqlite_readonly "$DB_FILE" "SELECT COUNT(*) FROM podcasts;" 2>/dev/null || echo "N/A")"
    episodes="$(sqlite_readonly "$DB_FILE" "SELECT COUNT(*) FROM episodes;" 2>/dev/null || echo "N/A")"
    tags="$(sqlite_readonly "$DB_FILE" "SELECT COUNT(*) FROM tags;" 2>/dev/null || echo "N/A")"
    workflows="$(sqlite_readonly "$DB_FILE" "SELECT COUNT(*) FROM workflows;" 2>/dev/null || echo "N/A")"
    echo "    播客数:   $podcasts"
    echo "    单集数:   $episodes"
    echo "    标签数:   $tags"
    echo "    工作流数: $workflows"

    fk_issues="$(sqlite_readonly "$DB_FILE" "PRAGMA foreign_key_check;" 2>/dev/null || true)"
    if [ -z "$fk_issues" ]; then
      ok "外键一致性检查通过"
    else
      fail "外键一致性检查发现问题"
      echo "$fk_issues"
      issues=$((issues + 1))
    fi
  else
    warn "未安装 sqlite3，跳过数据库内容检查"
  fi
else
  fail "数据库文件不存在: $DB_FILE"
  issues=$((issues + 1))
fi
echo ""

echo -e "${YELLOW}[4] 备份${NC}"
echo "-------------------------------------------"
latest_backup="$(find "$BACKUP_DIR" -maxdepth 1 -type f \( -name 'magicpodcast_*.db' -o -name 'magicpodcast_*.db.gz' \) 2>/dev/null | sort -r | head -1 || true)"
if [ -n "$latest_backup" ]; then
  backup_size="$(du -sh "$latest_backup" | cut -f1)"
  ok "最新备份: $latest_backup ($backup_size)"
else
  warn "未找到数据库备份。建议运行: $PROJECT_DIR/scripts/backup-db.sh"
fi

backup_label="com.magicpodcast.backup"
if [ "$(uname)" = "Darwin" ] && command -v launchctl >/dev/null 2>&1; then
  if launchctl print "gui/$(id -u)/$backup_label" >/dev/null 2>&1; then
    ok "每日备份定时任务已加载: $backup_label"
  else
    warn "每日备份定时任务未加载。建议运行: $PROJECT_DIR/scripts/install-backup-agent.sh"
  fi
fi
echo ""

echo -e "${YELLOW}[5] 加密异机备份${NC}"
echo "-------------------------------------------"
# When the daily launchd job is configured, reuse its non-secret destination
# settings so an interactive health check does not report a false
# "unconfigured" state merely because the shell has no launchd environment.
backup_plist="$HOME/Library/LaunchAgents/com.magicpodcast.backup.plist"
if [ -z "${MAGICPODCAST_OFFSITE_DIR:-}" ] && [ "$(uname)" = "Darwin" ] && command -v plutil >/dev/null 2>&1 && [ -f "$backup_plist" ]; then
  launchd_offsite_dir="$(plutil -extract EnvironmentVariables.MAGICPODCAST_OFFSITE_DIR raw -o - "$backup_plist" 2>/dev/null || true)"
  launchd_recipient_file="$(plutil -extract EnvironmentVariables.MAGICPODCAST_AGE_RECIPIENT_FILE raw -o - "$backup_plist" 2>/dev/null || true)"
  if [ -n "$launchd_offsite_dir" ] && [ -n "$launchd_recipient_file" ]; then
    export MAGICPODCAST_OFFSITE_DIR="$launchd_offsite_dir"
    export MAGICPODCAST_AGE_RECIPIENT_FILE="$launchd_recipient_file"
    launchd_offsite_max_age="$(plutil -extract EnvironmentVariables.MAGICPODCAST_OFFSITE_MAX_AGE_HOURS raw -o - "$backup_plist" 2>/dev/null || echo 26)"
    export MAGICPODCAST_OFFSITE_MAX_AGE_HOURS="$launchd_offsite_max_age"
  fi
fi
if [ -n "${MAGICPODCAST_OFFSITE_DIR:-}" ] || [ -n "${MAGICPODCAST_AGE_RECIPIENT_FILE:-}" ]; then
  offsite_status="$("$PROJECT_DIR/scripts/offsite-status.sh" 2>/dev/null || true)"
  if echo "$offsite_status" | grep -q '^status=ok'; then
    ok "异机加密备份正常: $offsite_status"
  else
    fail "异机加密备份异常: ${offsite_status:-unknown}"
    issues=$((issues + 1))
  fi
else
  fail "异机加密备份未配置；当前仅有本机备份"
  issues=$((issues + 1))
fi
echo ""

echo -e "${YELLOW}[6] 构建缓存与脚本入口${NC}"
echo "-------------------------------------------"
if [ -d "$PROJECT_DIR/frontend/.next" ]; then
  cache_size="$(du -sh "$PROJECT_DIR/frontend/.next" 2>/dev/null | cut -f1)"
  if [ -f "$PROJECT_DIR/frontend/.next/BUILD_ID" ]; then
    ok "Next.js 生产构建存在 ($cache_size)"
  else
    warn "Next.js 临时构建目录存在 ($cache_size)，热更新异常时可运行: $PROJECT_DIR/scripts/restart.sh --clean"
  fi
else
  ok "Next.js 临时构建目录不存在"
fi

for script in start.sh stop.sh restart.sh health.sh release.sh offsite-backup.sh offsite-status.sh restore-drill.sh; do
  script_path="$PROJECT_DIR/$script"
  if [ ! -e "$script_path" ] && [ -e "$PROJECT_DIR/scripts/$script" ]; then
    script_path="$PROJECT_DIR/scripts/$script"
  fi
  if [ -L "$script_path" ] || [ -x "$script_path" ]; then
    ok "$script 可用"
  else
    warn "$script 不可执行或不存在"
  fi
done
echo ""

echo -e "${BLUE}========================================"
echo "  诊断总结"
echo -e "========================================${NC}"
if [ "$issues" -eq 0 ]; then
  ok "核心检查通过"
else
  fail "发现 $issues 个需要处理的问题"
fi
echo ""

exit "$issues"
