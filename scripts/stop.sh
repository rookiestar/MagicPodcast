#!/bin/bash

echo "🛑 停止 MagicPodcast..."
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

# 停止后端
if [ -f logs/backend.pid ]; then
    BACKEND_PID=$(cat logs/backend.pid)
    if kill -0 $BACKEND_PID 2>/dev/null; then
        kill $BACKEND_PID
        echo "✅ 后端已停止 (PID: $BACKEND_PID)"
    else
        echo "⚠️  后端进程不存在"
    fi
    rm logs/backend.pid
else
    echo "⚠️  未找到后端 PID 文件"
fi

# 停止前端
if [ -f logs/frontend.pid ]; then
    FRONTEND_PID=$(cat logs/frontend.pid)
    if kill -0 $FRONTEND_PID 2>/dev/null; then
        kill $FRONTEND_PID
        echo "✅ 前端已停止 (PID: $FRONTEND_PID)"
    else
        echo "⚠️  前端进程不存在"
    fi
    rm logs/frontend.pid
else
    echo "⚠️  未找到前端 PID 文件"
fi

# 清理可能残留的进程
echo ""
echo "🧹 清理残留进程..."

BACKEND残留=$(lsof -ti:8080 2>/dev/null)
if [ -n "$BACKEND残留" ]; then
    echo "  发现后端残留进程，正在清理..."
    echo $BACKEND残留 | xargs kill -9 2>/dev/null
    echo "  ✅ 后端残留进程已清理"
fi

FRONTEND残留=$(lsof -ti:3000 2>/dev/null)
if [ -n "$FRONTEND残留" ]; then
    echo "  发现前端残留进程，正在清理..."
    echo $FRONTEND残留 | xargs kill -9 2>/dev/null
    echo "  ✅ 前端残留进程已清理"
fi

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✅ 所有服务已停止"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
