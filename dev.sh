#!/bin/bash

# MagicPodcast 开发环境管理脚本

set -e

FRONTEND_DIR="./frontend"
BACKEND_DIR="./backend"
FRONTEND_PID_FILE="/tmp/frontend-dev.pid"
BACKEND_PID_FILE="/tmp/backend-dev.pid"
FRONTEND_LOG="/tmp/frontend-dev.log"
BACKEND_LOG="/tmp/backend-dev.log"

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

print_status() {
    echo -e "${GREEN}✓${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}⚠${NC} $1"
}

print_error() {
    echo -e "${RED}✗${NC} $1"
}

# 启动服务
start() {
    echo "🚀 启动 MagicPodcast 开发环境..."

    # 清理缓存
    echo "🧹 清理缓存..."
    rm -rf "$FRONTEND_DIR/.next"
    rm -rf "$FRONTEND_DIR/node_modules/.cache"
    print_status "缓存已清理"

    # 启动后端
    if [ -f "$BACKEND_PID_FILE" ]; then
        if ps -p $(cat "$BACKEND_PID_FILE") > /dev/null 2>&1; then
            print_warning "后端已在运行中"
        else
            rm -f "$BACKEND_PID_FILE"
        fi
    fi

    if [ ! -f "$BACKEND_PID_FILE" ]; then
        echo "🔧 启动后端服务器..."

        # 加载环境变量
        if [ -f "$BACKEND_DIR/.env" ]; then
            print_status "加载环境变量从 .env"
            export $(cat "$BACKEND_DIR/.env" | grep -v '^#' | xargs)
        else
            print_warning ".env 文件不存在，LLM功能可能无法使用"
        fi

        cd "$BACKEND_DIR"
        nohup go run ./cmd/api/main.go > "$BACKEND_LOG" 2>&1 &
        echo $! > "$BACKEND_PID_FILE"
        sleep 3
        if ps -p $(cat "$BACKEND_PID_FILE") > /dev/null 2>&1; then
            print_status "后端服务器已启动 (PID: $(cat $BACKEND_PID_FILE))"
        else
            print_error "后端启动失败，查看日志: $BACKEND_LOG"
            return 1
        fi
        cd ..
    fi

    # 启动前端
    if [ -f "$FRONTEND_PID_FILE" ]; then
        if ps -p $(cat "$FRONTEND_PID_FILE") > /dev/null 2>&1; then
            print_warning "前端已在运行中"
        else
            rm -f "$FRONTEND_PID_FILE"
        fi
    fi

    if [ ! -f "$FRONTEND_PID_FILE" ]; then
        echo "🎨 启动前端开发服务器..."
        cd "$FRONTEND_DIR"
        nohup npm run dev > "$FRONTEND_LOG" 2>&1 &
        echo $! > "$FRONTEND_PID_FILE"
        sleep 5
        if ps -p $(cat "$FRONTEND_PID_FILE") > /dev/null 2>&1; then
            print_status "前端开发服务器已启动 (PID: $(cat $FRONTEND_PID_FILE))"
        else
            print_error "前端启动失败，查看日志: $FRONTEND_LOG"
            return 1
        fi
        cd ..
    fi

    echo ""
    echo "🎉 所有服务已启动！"
    echo ""
    echo "📍 访问地址："
    echo "   前端: http://localhost:3000"
    echo "   后端: http://localhost:8080"
    echo "   健康检查: http://localhost:8080/health"
    echo ""
    echo "📋 查看日志："
    echo "   tail -f $FRONTEND_LOG"
    echo "   tail -f $BACKEND_LOG"
}

# 停止服务
stop() {
    echo "🛑 停止 MagicPodcast 开发环境..."

    if [ -f "$FRONTEND_PID_FILE" ]; then
        PID=$(cat "$FRONTEND_PID_FILE")
        if ps -p $PID > /dev/null 2>&1; then
            kill $PID
            print_status "前端已停止"
        fi
        rm -f "$FRONTEND_PID_FILE"
    fi

    if [ -f "$BACKEND_PID_FILE" ]; then
        PID=$(cat "$BACKEND_PID_FILE")
        if ps -p $PID > /dev/null 2>&1; then
            kill $PID
            print_status "后端已停止"
        fi
        rm -f "$BACKEND_PID_FILE"
    fi

    # 清理可能残留的进程
    pkill -f "next dev" 2>/dev/null || true
    pkill -f "go run ./cmd/api/main.go" 2>/dev/null || true

    echo "✅ 所有服务已停止"
}

# 重启服务
restart() {
    stop
    sleep 2
    start
}

# 查看状态
status() {
    echo "📊 MagicPodcast 服务状态："
    echo ""

    # 检查后端
    if [ -f "$BACKEND_PID_FILE" ]; then
        PID=$(cat "$BACKEND_PID_FILE")
        if ps -p $PID > /dev/null 2>&1; then
            echo -e "${GREEN}✓${NC} 后端运行中 (PID: $PID)"
            curl -s http://localhost:8080/health > /dev/null 2>&1 && echo "  └─ 健康检查: 通过" || echo "  └─ 健康检查: 失败"
        else
            echo -e "${RED}✗${NC} 后端未运行"
        fi
    else
        echo -e "${RED}✗${NC} 后端未运行"
    fi

    # 检查前端
    if [ -f "$FRONTEND_PID_FILE" ]; then
        PID=$(cat "$FRONTEND_PID_FILE")
        if ps -p $PID > /dev/null 2>&1; then
            echo -e "${GREEN}✓${NC} 前端运行中 (PID: $PID)"
        else
            echo -e "${RED}✗${NC} 前端未运行"
        fi
    else
        echo -e "${RED}✗${NC} 前端未运行"
    fi
}

# 查看日志
logs() {
    echo "选择要查看的日志："
    echo "1) 前端日志"
    echo "2) 后端日志"
    read -p "请输入选项 (1/2): " choice

    case $choice in
        1)
            tail -f "$FRONTEND_LOG"
            ;;
        2)
            tail -f "$BACKEND_LOG"
            ;;
        *)
            print_error "无效选项"
            ;;
    esac
}

# 清理缓存
clean() {
    echo "🧹 清理缓存..."
    rm -rf "$FRONTEND_DIR/.next"
    rm -rf "$FRONTEND_DIR/node_modules/.cache"
    print_status "缓存已清理"
}

# 显示帮助
help() {
    echo "MagicPodcast 开发环境管理脚本"
    echo ""
    echo "用法: ./dev.sh [命令]"
    echo ""
    echo "命令:"
    echo "  start    启动所有服务"
    echo "  stop     停止所有服务"
    echo "  restart  重启所有服务"
    echo "  status   查看服务状态"
    echo "  logs     查看日志"
    echo "  clean    清理缓存"
    echo "  help     显示此帮助信息"
}

# 主程序
case "$1" in
    start)
        start
        ;;
    stop)
        stop
        ;;
    restart)
        restart
        ;;
    status)
        status
        ;;
    logs)
        logs
        ;;
    clean)
        clean
        ;;
    help)
        help
        ;;
    *)
        help
        ;;
esac
