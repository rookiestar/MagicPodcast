#!/bin/bash
# MagicPodcast health check.

set -euo pipefail

PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DB_FILE="$PROJECT_DIR/backend/data/magicpodcast.db"

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

http_status() {
  local url="$1"
  curl -s -o /dev/null -w "%{http_code}" "$url" 2>/dev/null || true
}

echo -e "${BLUE}========================================"
echo "  MagicPodcast 健康检查"
echo "  $(date '+%Y-%m-%d %H:%M:%S')"
echo -e "========================================${NC}"
echo ""

issues=0

echo -e "${YELLOW}[1] 服务端口${NC}"
echo "-------------------------------------------"
backend_pid="$(listener_pid 8080)"
frontend_pid="$(listener_pid 3000)"

if [ -n "$backend_pid" ]; then
  backend_cmd="$(ps -p "$backend_pid" -o command= 2>/dev/null || echo unknown)"
  ok "后端端口 8080 正在监听 [PID: $backend_pid, $backend_cmd]"
else
  warn "后端端口 8080 未监听"
fi

if [ -n "$frontend_pid" ]; then
  frontend_cmd="$(ps -p "$frontend_pid" -o command= 2>/dev/null || echo unknown)"
  ok "前端端口 3000 正在监听 [PID: $frontend_pid, $frontend_cmd]"

  if ps -axo command= | grep -F "$PROJECT_DIR/frontend" | grep -q "next dev"; then
    fail "前端正在开发模式运行，公网访问会加载开发资源。请运行: $PROJECT_DIR/scripts/restart.sh"
    issues=$((issues + 1))
  else
    ok "前端未发现开发模式进程"
  fi
else
  warn "前端端口 3000 未监听"
fi
echo ""

echo -e "${YELLOW}[2] HTTP 健康检查${NC}"
echo "-------------------------------------------"
backend_health="$(curl -s http://localhost:8080/health 2>/dev/null || true)"
if echo "$backend_health" | grep -q '"status":"ok"'; then
  ok "后端 /health 正常: $backend_health"
else
  fail "后端 /health 无法确认正常"
  [ -n "$backend_health" ] && echo "    响应: $backend_health"
  issues=$((issues + 1))
fi

frontend_status="$(http_status http://localhost:3000)"
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

  if command -v sqlite3 >/dev/null 2>&1; then
    integrity="$(sqlite3 -readonly "$DB_FILE" "PRAGMA integrity_check;" 2>/dev/null || true)"
    if [ "$integrity" = "ok" ]; then
      ok "SQLite integrity_check 通过"
    else
      fail "SQLite integrity_check 失败: ${integrity:-N/A}"
      issues=$((issues + 1))
    fi

    podcasts="$(sqlite3 -readonly "$DB_FILE" "SELECT COUNT(*) FROM podcasts;" 2>/dev/null || echo "N/A")"
    episodes="$(sqlite3 -readonly "$DB_FILE" "SELECT COUNT(*) FROM episodes;" 2>/dev/null || echo "N/A")"
    tags="$(sqlite3 -readonly "$DB_FILE" "SELECT COUNT(*) FROM tags;" 2>/dev/null || echo "N/A")"
    workflows="$(sqlite3 -readonly "$DB_FILE" "SELECT COUNT(*) FROM workflows;" 2>/dev/null || echo "N/A")"
    echo "    播客数:   $podcasts"
    echo "    单集数:   $episodes"
    echo "    标签数:   $tags"
    echo "    工作流数: $workflows"

    fk_issues="$(sqlite3 -readonly "$DB_FILE" "PRAGMA foreign_key_check;" 2>/dev/null || true)"
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
backup_dir="$PROJECT_DIR/backend/data/backups"
latest_backup="$(find "$backup_dir" -maxdepth 1 -type f \( -name 'magicpodcast_*.db' -o -name 'magicpodcast_*.db.gz' \) 2>/dev/null | sort -r | head -1 || true)"
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

echo -e "${YELLOW}[5] 构建缓存与脚本入口${NC}"
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

for script in start.sh stop.sh restart.sh health.sh; do
  if [ -L "$PROJECT_DIR/$script" ] || [ -x "$PROJECT_DIR/$script" ]; then
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
