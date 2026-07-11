#!/bin/bash
# MagicPodcast 本地服务启动脚本
# 默认以生产模式启动前端，供 rookiestar.cn / Cloudflare Tunnel 使用。
# 用法: ./start.sh [--clean] [--prod|--dev]

set -euo pipefail

PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
FRONTEND_DIR="$PROJECT_DIR/frontend"
BACKEND_DIR="$PROJECT_DIR/backend"
FRONTEND_PID_FILE="/tmp/magicpodcast-frontend.pid"
BACKEND_PID_FILE="/tmp/magicpodcast-backend.pid"
FRONTEND_LOG="/tmp/magicpodcast-frontend.log"
BACKEND_LOG="/tmp/magicpodcast-backend.log"
FRONTEND_SCREEN_SESSION="magicpodcast-frontend"
BACKEND_SCREEN_SESSION="magicpodcast-backend"

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

print_status() { echo -e "${GREEN}✓${NC} $1"; }
print_warning() { echo -e "${YELLOW}⚠${NC} $1"; }
print_error() { echo -e "${RED}✗${NC} $1"; }

listener_pid() {
    local port="$1"
    lsof -ti :"$port" -sTCP:LISTEN 2>/dev/null | head -1 || true
}

pid_is_running() {
    local pid_file="$1"
    [ -f "$pid_file" ] && ps -p "$(cat "$pid_file" 2>/dev/null)" >/dev/null 2>&1
}

wait_for_listener() {
    local port="$1"
    local attempts="$2"
    local pid=""

    for _ in $(seq 1 "$attempts"); do
        pid="$(listener_pid "$port")"
        if [ -n "$pid" ]; then
            echo "$pid"
            return 0
        fi
        sleep 1
    done

    return 1
}

stop_screen_session() {
    local session="$1"

    if command -v screen >/dev/null 2>&1; then
        screen -S "$session" -X quit >/dev/null 2>&1 || true
        screen -wipe >/dev/null 2>&1 || true
    fi
}

start_frontend_server() {
    : > "$FRONTEND_LOG"

    if command -v screen >/dev/null 2>&1; then
        stop_screen_session "$FRONTEND_SCREEN_SESSION"
        if [ "$FRONTEND_MODE" = "development" ]; then
            screen -dmS "$FRONTEND_SCREEN_SESSION" bash -lc "cd '$FRONTEND_DIR' && exec npm run dev >> '$FRONTEND_LOG' 2>&1"
        else
            screen -dmS "$FRONTEND_SCREEN_SESSION" bash -lc "cd '$FRONTEND_DIR' && exec npm run start >> '$FRONTEND_LOG' 2>&1"
        fi
    else
        if [ "$FRONTEND_MODE" = "development" ]; then
            nohup bash -lc "cd '$FRONTEND_DIR' && exec npm run dev" >> "$FRONTEND_LOG" 2>&1 &
        else
            nohup bash -lc "cd '$FRONTEND_DIR' && exec npm run start" >> "$FRONTEND_LOG" 2>&1 &
        fi
        echo $! > "$FRONTEND_PID_FILE"
    fi
}

start_backend_server() {
    : > "$BACKEND_LOG"

    if command -v screen >/dev/null 2>&1; then
        stop_screen_session "$BACKEND_SCREEN_SESSION"
        screen -dmS "$BACKEND_SCREEN_SESSION" bash -lc "cd '$BACKEND_DIR' && exec ./api >> '$BACKEND_LOG' 2>&1"
    else
        nohup bash -lc "cd '$BACKEND_DIR' && exec ./api" >> "$BACKEND_LOG" 2>&1 &
        echo $! > "$BACKEND_PID_FILE"
    fi
}

FRONTEND_MODE="${MAGICPODCAST_FRONTEND_MODE:-production}"
CLEAN_BUILD=false

for arg in "$@"; do
    case "$arg" in
        --clean|-c)
            CLEAN_BUILD=true
            ;;
        --prod|--production)
            FRONTEND_MODE="production"
            ;;
        --dev|--development)
            FRONTEND_MODE="development"
            ;;
        "")
            ;;
        *)
            print_error "未知参数: $arg"
            echo "用法: ./scripts/start.sh [--clean] [--prod|--dev]"
            exit 1
            ;;
    esac
done

if [ "$FRONTEND_MODE" != "production" ] && [ "$FRONTEND_MODE" != "development" ]; then
    print_error "无效前端模式: $FRONTEND_MODE"
    echo "请使用 production 或 development"
    exit 1
fi

echo -e "${BLUE}========================================"
echo "  MagicPodcast 服务启动"
echo "  $(date '+%Y-%m-%d %H:%M:%S')"
echo "  前端模式: $FRONTEND_MODE"
echo -e "========================================${NC}"
echo ""

# 可选清理缓存
if [ "$CLEAN_BUILD" = true ]; then
    echo -e "${YELLOW}[0] 清理缓存...${NC}"
    rm -rf "$FRONTEND_DIR/.next" 2>/dev/null || true
    print_status "Next.js 临时构建目录已清理"
    echo ""
fi

# 检查端口
echo -e "${YELLOW}[1] 检查端口...${NC}"
for port in 8080 3000; do
    if lsof -i :$port -P -n 2>/dev/null | grep -q "LISTEN"; then
        print_warning "端口 $port 已被占用"
        lsof -i :$port -P -n 2>/dev/null | grep "LISTEN" | head -1
    else
        print_status "端口 $port 可用"
    fi
done
echo ""

# 启动后端
echo -e "${YELLOW}[2] 启动后端服务...${NC}"
backend_listener="$(listener_pid 8080)"
if pid_is_running "$BACKEND_PID_FILE"; then
    print_warning "后端已在运行 (PID: $(cat $BACKEND_PID_FILE))"
elif [ -n "$backend_listener" ]; then
    echo "$backend_listener" > "$BACKEND_PID_FILE"
    print_warning "端口 8080 已有服务在运行 (PID: $backend_listener)，跳过后端启动"
else
    cd "$BACKEND_DIR"

    # 加载环境变量
    if [ -f ".env" ]; then
        export $(cat .env | grep -v '^#' | xargs)
    fi

    if [ "$FRONTEND_MODE" = "production" ]; then
        export MAGICPODCAST_SERVER_MODE="${MAGICPODCAST_SERVER_MODE:-release}"
        export MAGICPODCAST_DATABASE_DEBUG="${MAGICPODCAST_DATABASE_DEBUG:-false}"
    fi

    echo "  编译后端..."
    go build -o api ./cmd/api

    start_backend_server

    if backend_listener="$(wait_for_listener 8080 60)"; then
        if [ -n "$backend_listener" ]; then
            echo "$backend_listener" > "$BACKEND_PID_FILE"
        fi
        print_status "后端已启动 (PID: $(cat $BACKEND_PID_FILE))"
    else
        stop_screen_session "$BACKEND_SCREEN_SESSION"
        print_error "后端启动失败"
        echo "  查看日志: tail -f $BACKEND_LOG"
        exit 1
    fi
    cd "$PROJECT_DIR"
fi
echo ""

# 启动前端
echo -e "${YELLOW}[3] 启动前端服务...${NC}"
frontend_listener="$(listener_pid 3000)"
if pid_is_running "$FRONTEND_PID_FILE" && [ -n "$frontend_listener" ]; then
    print_warning "前端已在运行 (PID: $(cat $FRONTEND_PID_FILE))"
elif [ -n "$frontend_listener" ]; then
    echo "$frontend_listener" > "$FRONTEND_PID_FILE"
    print_warning "端口 3000 已有服务在运行 (PID: $frontend_listener)，跳过前端启动"
else
    rm -f "$FRONTEND_PID_FILE"
    cd "$FRONTEND_DIR"

    # 检查是否需要安装依赖
    if [ ! -d "node_modules" ]; then
        echo "  安装依赖..."
        npm install
    fi

    if [ "$FRONTEND_MODE" = "production" ]; then
        export NEXT_PUBLIC_IMAGE_OPTIMIZER_PATH="${NEXT_PUBLIC_IMAGE_OPTIMIZER_PATH:-/_next/image.webp}"
        echo "  图片优化路径: $NEXT_PUBLIC_IMAGE_OPTIMIZER_PATH"
        echo "  清理旧生产构建..."
        rm -rf .next
        echo "  构建生产版本..."
        npm run build
    fi

    start_frontend_server

    if frontend_listener="$(wait_for_listener 3000 60)"; then
        echo "$frontend_listener" > "$FRONTEND_PID_FILE"
        print_status "前端已启动 (PID: $(cat $FRONTEND_PID_FILE))"
    else
        stop_screen_session "$FRONTEND_SCREEN_SESSION"
        print_error "前端启动失败"
        echo "  查看日志: tail -f $FRONTEND_LOG"
        exit 1
    fi
    cd "$PROJECT_DIR"
fi
echo ""

# 健康检查
echo -e "${YELLOW}[4] 健康检查...${NC}"
sleep 2
if BACKEND_HEALTH=$(curl -s http://localhost:8080/health 2>/dev/null); then
    print_status "后端: $BACKEND_HEALTH"
else
    print_warning "后端: 无响应"
fi

FRONTEND_STATUS=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:3000 2>/dev/null || true)
if [ "$FRONTEND_STATUS" = "200" ]; then
    print_status "前端: HTTP $FRONTEND_STATUS"
else
    print_warning "前端: HTTP $FRONTEND_STATUS (开发服务器可能还在启动)"
fi
echo ""

# 显示访问信息
echo -e "${BLUE}========================================"
echo "  服务已启动"
echo -e "========================================${NC}"
echo ""
echo "  访问地址:"
echo "    前端: http://localhost:3000"
echo "    后端: http://localhost:8080"
echo "    健康检查: http://localhost:8080/health"
echo ""
echo "  日志位置:"
echo "    前端: $FRONTEND_LOG"
echo "    后端: $BACKEND_LOG"
echo ""
echo "  其他命令:"
echo "    ./stop.sh     - 停止服务"
echo "    ./restart.sh  - 重启服务"
echo "    ./restart.sh --dev - 以开发模式重启前端"
echo "    ./health.sh   - 健康检查"
echo ""
