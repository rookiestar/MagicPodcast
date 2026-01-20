#!/bin/bash

# MagicPodcast 快速启动脚本
# 在新项目位置使用

PROJECT_DIR="/Users/rookiestar/VSCode/Projects/MagicPodcast"

echo "🚀 MagicPodcast 快速启动"
echo "======================"
echo ""

# 检查端口占用
check_port() {
    local port=$1
    if lsof -ti:$port > /dev/null 2>&1; then
        echo "⚠️  端口 $port 已被占用"
        read -p "是否停止占用该端口的进程? (y/n) " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            lsof -ti:$port | xargs kill -9
            echo "✅ 已停止端口 $port 上的进程"
        else
            echo "❌ 无法启动服务，端口 $port 被占用"
            exit 1
        fi
    fi
}

# 停止现有服务
stop_services() {
    echo "🛑 停止现有服务..."
    check_port 3000
    check_port 3001
    check_port 8080
}

# 启动后端
start_backend() {
    echo ""
    echo "📦 启动后端服务..."
    cd "$PROJECT_DIR/backend"

    # 检查是否有预编译二进制
    if [ -f "./main" ]; then
        echo "使用预编译二进制..."
        ./main > /tmp/magicpodcast_backend.log 2>&1 &
        BACKEND_PID=$!
    else
        echo "使用 go run 启动..."
        go run main.go > /tmp/magicpodcast_backend.log 2>&1 &
        BACKEND_PID=$!
    fi

    sleep 2

    if ps -p $BACKEND_PID > /dev/null; then
        echo "✅ 后端启动成功 (PID: $BACKEND_PID)"
        echo "   API地址: http://localhost:8080"
        echo "   日志文件: /tmp/magicpodcast_backend.log"
    else
        echo "❌ 后端启动失败，查看日志:"
        tail -20 /tmp/magicpodcast_backend.log
        exit 1
    fi
}

# 启动前端
start_frontend() {
    echo ""
    echo "🎨 启动前端服务..."
    cd "$PROJECT_DIR/frontend"

    # 清理构建缓存（可选）
    if [ "$1" == "--clean" ]; then
        echo "清理构建缓存..."
        rm -rf .next
    fi

    npm run dev > /tmp/magicpodcast_frontend.log 2>&1 &
    FRONTEND_PID=$!

    sleep 5

    if ps -p $FRONTEND_PID > /dev/null; then
        # 从日志中提取实际端口
        FRONTEND_PORT=$(grep -o "Local:.*http://localhost:[0-9]*" /tmp/magicpodcast_frontend.log | head -1 | grep -o "[0-9]*$")
        if [ -z "$FRONTEND_PORT" ]; then
            FRONTEND_PORT="3000 (默认)"
        fi
        echo "✅ 前端启动成功 (PID: $FRONTEND_PID)"
        echo "   访问地址: http://localhost:$FRONTEND_PORT"
        echo "   日志文件: /tmp/magicpodcast_frontend.log"
    else
        echo "❌ 前端启动失败，查看日志:"
        tail -30 /tmp/magicpodcast_frontend.log
        exit 1
    fi
}

# 主流程
main() {
    stop_services
    start_backend
    start_frontend "$@"

    echo ""
    echo "======================"
    echo "✅ 所有服务启动完成！"
    echo ""
    echo "📝 查看日志:"
    echo "  后端: tail -f /tmp/magicpodcast_backend.log"
    echo "  前端: tail -f /tmp/magicpodcast_frontend.log"
    echo ""
    echo "🛑 停止服务:"
    echo "  运行: $PROJECT_DIR/stop_services.sh"
    echo "  或: kill $BACKEND_PID $FRONTEND_PID"
    echo ""
}

# 执行
main "$@"
