#!/bin/bash

# MagicPodcast 停止服务脚本

echo "🛑 停止 MagicPodcast 服务"
echo "========================="
echo ""

# 停止前端
if lsof -ti:3000 > /dev/null 2>&1 || lsof -ti:3001 > /dev/null 2>&1 || lsof -ti:3002 > /dev/null 2>&1; then
    echo "📱 停止前端服务..."
    lsof -ti:3000,3001,3002 2>/dev/null | xargs kill -9 2>/dev/null
    echo "✅ 前端已停止"
else
    echo "ℹ️  前端服务未运行"
fi

# 停止后端
if lsof -ti:8080 > /dev/null 2>&1; then
    echo "🔧 停止后端服务..."
    lsof -ti:8080 | xargs kill -9
    echo "✅ 后端已停止"
else
    echo "ℹ️  后端服务未运行"
fi

echo ""
echo "✅ 所有服务已停止"
