#!/bin/bash
# Stop MagicPodcast local services.

set -euo pipefail

PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
FRONTEND_DIR="$PROJECT_DIR/frontend"
FRONTEND_PID_FILE="${MAGICPODCAST_FRONTEND_PID_FILE:-/tmp/magicpodcast-frontend.pid}"
BACKEND_PID_FILE="${MAGICPODCAST_BACKEND_PID_FILE:-/tmp/magicpodcast-backend.pid}"
FRONTEND_SCREEN_SESSION="magicpodcast-frontend"
BACKEND_SCREEN_SESSION="magicpodcast-backend"

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

ok() { echo -e "  ${GREEN}✓${NC} $1"; }
warn() { echo -e "  ${YELLOW}⚠${NC} $1"; }

listener_pids() {
  local port="$1"
  lsof -ti :"$port" -sTCP:LISTEN 2>/dev/null || true
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

pid_belongs_to_magicpodcast() {
  local pid="$1"
  local expected_cwd="$2"
  local role="$3"
  local command

  [ -n "$pid" ] || return 1
  [ "$(pid_cwd "$pid")" = "$expected_cwd" ] || return 1
  command="$(pid_command "$pid")"
  case "$role" in
    backend) [[ "$command" == *"api"* ]] ;;
    frontend) [[ "$command" == *"next-server"* ]] ;;
    *) return 1 ;;
  esac
}

stop_pid() {
  local name="$1"
  local pid="$2"

  if [ -z "$pid" ] || ! ps -p "$pid" >/dev/null 2>&1; then
    warn "$name 未运行"
    return
  fi

  kill "$pid" 2>/dev/null || true
  for _ in 1 2 3 4 5; do
    if ! ps -p "$pid" >/dev/null 2>&1; then
      ok "$name 已停止 (PID: $pid)"
      return
    fi
    sleep 1
  done

  kill -9 "$pid" 2>/dev/null || true
  ok "$name 已强制停止 (PID: $pid)"
}

stop_pid_tree() {
  local name="$1"
  local pid="$2"

  if [ -z "$pid" ] || ! ps -p "$pid" >/dev/null 2>&1; then
    return
  fi

  while IFS= read -r child_pid; do
    [ -z "$child_pid" ] && continue
    stop_pid_tree "$name" "$child_pid"
  done < <(pgrep -P "$pid" 2>/dev/null || true)

  if ps -p "$pid" >/dev/null 2>&1; then
    stop_pid "$name" "$pid"
  fi
}

stop_pid_file() {
  local name="$1"
  local pid_file="$2"
  local expected_cwd="$3"
  local role="$4"
  local pid

  if [ -f "$pid_file" ]; then
    pid="$(cat "$pid_file" 2>/dev/null || true)"
    if pid_belongs_to_magicpodcast "$pid" "$expected_cwd" "$role"; then
      stop_pid "$name" "$pid"
    else
      warn "$name PID 文件不属于当前 MagicPodcast，未停止该进程"
    fi
    rm -f "$pid_file"
  else
    warn "未找到 $name PID 文件"
  fi
}

stop_screen_session() {
  local name="$1"
  local session="$2"
  local expected_cwd="$3"
  local session_pid
  local command

  if command -v screen >/dev/null 2>&1 && screen -ls 2>/dev/null | grep -q "[.]$session[[:space:]]"; then
    session_pid="$(screen -ls 2>/dev/null | awk -v suffix=".${session}" '$1 ~ suffix { split($1, parts, "."); print parts[1]; exit }')"
    command="$(pid_command "$session_pid")"
    if [ "$(pid_cwd "$session_pid")" != "$expected_cwd" ] || [[ "$command" != *"SCREEN -dmS $session"* ]]; then
      warn "$name screen 会话身份不匹配，未停止"
      return
    fi
    screen -S "$session" -X quit >/dev/null 2>&1 || true
    ok "$name 后台会话已停止"
  fi
}

stop_frontend_dev_shells() {
  local found=false

  while IFS= read -r pid; do
    [ -z "$pid" ] && continue
    found=true
    stop_pid_tree "前端后台会话残留" "$pid"
  done < <(
    ps -axo pid=,command= |
      awk -v dir="$FRONTEND_DIR" '$0 ~ dir && $0 ~ /tail -f \/dev\/null \| npm run dev/ {print $1}'
  )

  if [ "$found" = false ]; then
    ok "前端后台会话无残留"
  fi
}

stop_port() {
  local name="$1"
  local port="$2"
  local expected_cwd="$3"
  local role="$4"
  local found=false
  local foreign=false

  while IFS= read -r pid; do
    [ -z "$pid" ] && continue
    if pid_belongs_to_magicpodcast "$pid" "$expected_cwd" "$role"; then
      found=true
      stop_pid "$name 端口 $port 进程" "$pid"
    else
      foreign=true
      warn "$name 端口 $port 由非 MagicPodcast 进程占用 (PID: $pid)，未停止"
    fi
  done < <(listener_pids "$port")

  if [ "$found" = false ]; then
    if [ "$foreign" = true ]; then
      warn "$name 端口 $port 仍被其他应用占用"
    else
      ok "$name 端口 $port 已释放"
    fi
  fi
}

echo -e "${BLUE}========================================"
echo "  停止 MagicPodcast"
echo "  $(date '+%Y-%m-%d %H:%M:%S')"
echo -e "========================================${NC}"
echo ""

echo "[1] 停止已记录的服务"
stop_pid_file "后端" "$BACKEND_PID_FILE" "$PROJECT_DIR/backend" backend
stop_pid_file "前端" "$FRONTEND_PID_FILE" "$FRONTEND_DIR" frontend
stop_screen_session "后端" "$BACKEND_SCREEN_SESSION" "$PROJECT_DIR/backend"
stop_screen_session "前端" "$FRONTEND_SCREEN_SESSION" "$FRONTEND_DIR"
stop_frontend_dev_shells
echo ""

echo "[2] 清理端口监听"
stop_port "后端" 8080 "$PROJECT_DIR/backend" backend
stop_port "前端" 3000 "$FRONTEND_DIR" frontend
echo ""

ok "MagicPodcast 本地服务已停止"
