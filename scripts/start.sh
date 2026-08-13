#!/bin/bash
# MagicPodcast 本地服务启动脚本
# 默认以生产模式启动前端，供 rookiestar.cn / Cloudflare Tunnel 使用。
# 用法: ./start.sh [--clean] [--no-build] [--prod|--dev]

set -euo pipefail

PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
FRONTEND_DIR="$PROJECT_DIR/frontend"
BACKEND_DIR="$PROJECT_DIR/backend"
FRONTEND_PID_FILE="${MAGICPODCAST_FRONTEND_PID_FILE:-/tmp/magicpodcast-frontend.pid}"
BACKEND_PID_FILE="${MAGICPODCAST_BACKEND_PID_FILE:-/tmp/magicpodcast-backend.pid}"
FRONTEND_LOG="${MAGICPODCAST_FRONTEND_LOG:-/tmp/magicpodcast-frontend.log}"
BACKEND_LOG="${MAGICPODCAST_BACKEND_LOG:-/tmp/magicpodcast-backend.log}"
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

pid_cwd() {
    local pid="$1"
    lsof -a -p "$pid" -d cwd -Fn 2>/dev/null |
        awk '/^n/ { sub(/^n/, ""); print; exit }'
}

listener_belongs_to_magicpodcast() {
    local name="$1"
    local port="$2"
    local expected_cwd="$3"
    local pid
    local cwd

    pid="$(listener_pid "$port")"
    [ -n "$pid" ] || return 1

    cwd="$(pid_cwd "$pid")"
    if [ "$cwd" != "$expected_cwd" ]; then
        print_error "$name 端口 $port 已被非 MagicPodcast 进程占用 (PID: ${pid}，工作目录: ${cwd:-未知})" >&2
        print_error "为避免误接管或误暴露服务，启动已停止；请先人工处理该进程。" >&2
        return 2
    fi

    if ! lsof -nP -a -p "$pid" -iTCP:"$port" -sTCP:LISTEN 2>/dev/null |
        grep -Eq "TCP (127\\.0\\.0\\.1|\\[::1\\]):$port \\(LISTEN\\)"; then
        print_error "$name 未仅监听 loopback (PID: ${pid}，端口: $port)" >&2
        print_error "为避免把局域网或公网监听误作为生产服务，启动已停止。" >&2
        return 2
    fi

    echo "$pid"
}

screen_session_pid() {
    local session="$1"
    screen -ls 2>/dev/null |
        awk -v suffix=".${session}" '$1 ~ suffix { split($1, parts, "."); print parts[1]; exit }'
}

screen_session_is_owned() {
    local session="$1"
    local expected_cwd="$2"
    local pid
    local command

    pid="$(screen_session_pid "$session")"
    [ -n "$pid" ] || return 1
    command="$(ps -p "$pid" -o command= 2>/dev/null || true)"
    [ "$(pid_cwd "$pid")" = "$expected_cwd" ] || return 1
    [[ "$command" == *"SCREEN -dmS $session"* ]]
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
    local expected_cwd="$2"

    if command -v screen >/dev/null 2>&1; then
        if screen -ls 2>/dev/null | awk -v suffix=".${session}" '$1 ~ suffix { found=1 } END { exit(found ? 0 : 1) }'; then
            if ! screen_session_is_owned "$session" "$expected_cwd"; then
                print_error "拒绝接管未知 screen 会话: $session" >&2
                return 1
            fi
            screen -S "$session" -X quit >/dev/null 2>&1 || true
            screen -wipe >/dev/null 2>&1 || true
        fi
    fi
}

start_frontend_server() {
    : > "$FRONTEND_LOG"

    if command -v screen >/dev/null 2>&1; then
        stop_screen_session "$FRONTEND_SCREEN_SESSION" "$FRONTEND_DIR"
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
        stop_screen_session "$BACKEND_SCREEN_SESSION" "$BACKEND_DIR"
        screen -dmS "$BACKEND_SCREEN_SESSION" bash -lc "cd '$BACKEND_DIR' && exec ./api >> '$BACKEND_LOG' 2>&1"
    else
        nohup bash -lc "cd '$BACKEND_DIR' && exec ./api" >> "$BACKEND_LOG" 2>&1 &
        echo $! > "$BACKEND_PID_FILE"
    fi
}

FRONTEND_MODE="${MAGICPODCAST_FRONTEND_MODE:-production}"
CLEAN_BUILD=false
NO_BUILD=false
RELEASE_ID="${MAGICPODCAST_RELEASE_ID:-local-$(date -u '+%Y%m%dT%H%M%SZ')-$$}"
FRONTEND_BUILD_ID="${MAGICPODCAST_FRONTEND_BUILD_ID:-}"

for arg in "$@"; do
    case "$arg" in
        --clean|-c)
            CLEAN_BUILD=true
            ;;
        --no-build)
            NO_BUILD=true
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
            echo "用法: ./scripts/start.sh [--clean] [--no-build] [--prod|--dev]"
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
if [ -n "$backend_listener" ]; then
    backend_listener="$(listener_belongs_to_magicpodcast "后端" 8080 "$BACKEND_DIR")" || exit 1
    echo "$backend_listener" > "$BACKEND_PID_FILE"
    print_warning "后端已在运行 (PID: $backend_listener)"
else
    cd "$BACKEND_DIR"

    # 加载环境变量
    if [ -f ".env" ]; then
        export $(cat .env | grep -v '^#' | xargs)
    fi

    if [ "$FRONTEND_MODE" = "production" ]; then
        export MAGICPODCAST_SERVER_MODE="${MAGICPODCAST_SERVER_MODE:-release}"
        export MAGICPODCAST_DATABASE_DEBUG="${MAGICPODCAST_DATABASE_DEBUG:-false}"
        export MAGICPODCAST_DATA_PROFILE="${MAGICPODCAST_DATA_PROFILE:-production}"
        export MAGICPODCAST_PRODUCTION_PROFILE_CONFIRM="${MAGICPODCAST_PRODUCTION_PROFILE_CONFIRM:-I_UNDERSTAND_THIS_USES_PRODUCTION_DATA}"
    fi

    if [ "$NO_BUILD" = true ]; then
        if [ ! -x "api" ]; then
            print_error "--no-build 要求已有可执行后端产物: $BACKEND_DIR/api"
            exit 1
        fi
        print_status "复用已验证后端产物"
    else
        echo "  编译后端..."
        go build -o api ./cmd/api
    fi

    export MAGICPODCAST_RELEASE_ID="$RELEASE_ID"
    export MAGICPODCAST_FRONTEND_BUILD_ID="$FRONTEND_BUILD_ID"

    start_backend_server

    if backend_listener="$(wait_for_listener 8080 60)"; then
        backend_listener="$(listener_belongs_to_magicpodcast "后端" 8080 "$BACKEND_DIR")" || exit 1
        if [ -n "$backend_listener" ]; then
            echo "$backend_listener" > "$BACKEND_PID_FILE"
        fi
        print_status "后端已启动 (PID: $(cat $BACKEND_PID_FILE))"
    else
        stop_screen_session "$BACKEND_SCREEN_SESSION" "$BACKEND_DIR" || true
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
if [ -n "$frontend_listener" ]; then
    frontend_listener="$(listener_belongs_to_magicpodcast "前端" 3000 "$FRONTEND_DIR")" || exit 1
    echo "$frontend_listener" > "$FRONTEND_PID_FILE"
    print_warning "前端已在运行 (PID: $frontend_listener)"
else
    rm -f "$FRONTEND_PID_FILE"
    cd "$FRONTEND_DIR"

    # 检查是否需要安装依赖
    if [ ! -d "node_modules" ]; then
        if [ "$NO_BUILD" = true ]; then
            print_error "--no-build 要求已有前端依赖目录: $FRONTEND_DIR/node_modules"
            exit 1
        fi
        echo "  安装依赖..."
        npm install
    fi

    if [ "$FRONTEND_MODE" = "production" ]; then
        if [ "$NO_BUILD" = true ]; then
            if [ ! -f ".next/BUILD_ID" ]; then
                print_error "--no-build 要求已有已验证前端产物: $FRONTEND_DIR/.next/BUILD_ID"
                exit 1
            fi
            if [ -z "$FRONTEND_BUILD_ID" ]; then
                FRONTEND_BUILD_ID="$(cat .next/BUILD_ID)"
                export MAGICPODCAST_FRONTEND_BUILD_ID="$FRONTEND_BUILD_ID"
            fi
            print_status "复用已验证前端产物 (BUILD_ID: $FRONTEND_BUILD_ID)"
        else
            export NEXT_PUBLIC_IMAGE_OPTIMIZER_PATH="/_next/image.webp"
            echo "  图片优化路径: $NEXT_PUBLIC_IMAGE_OPTIMIZER_PATH"
            echo "  清理旧生产构建..."
            rm -rf .next
            echo "  构建生产版本..."
            npm run build
            if ! node "$PROJECT_DIR/scripts/verify-image-optimizer-build.mjs" \
                "$FRONTEND_DIR/.next" "$NEXT_PUBLIC_IMAGE_OPTIMIZER_PATH"; then
                print_error "前端图片优化路径校验失败"
                exit 1
            fi
            if [ -z "$FRONTEND_BUILD_ID" ] && [ -f ".next/BUILD_ID" ]; then
                FRONTEND_BUILD_ID="$(cat .next/BUILD_ID)"
                export MAGICPODCAST_FRONTEND_BUILD_ID="$FRONTEND_BUILD_ID"
            fi
        fi
    fi

    start_frontend_server

    if frontend_listener="$(wait_for_listener 3000 60)"; then
        frontend_listener="$(listener_belongs_to_magicpodcast "前端" 3000 "$FRONTEND_DIR")" || exit 1
        echo "$frontend_listener" > "$FRONTEND_PID_FILE"
        print_status "前端已启动 (PID: $(cat $FRONTEND_PID_FILE))"
    else
        stop_screen_session "$FRONTEND_SCREEN_SESSION" "$FRONTEND_DIR" || true
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
