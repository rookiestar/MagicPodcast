#!/bin/bash

echo "🚀 启动 MagicPodcast..."
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

# 确保日志目录存在
mkdir -p logs backend/data

# 启动后端
echo "📡 启动后端服务..."
cd backend
nohup go run cmd/api/main.go > ../logs/backend.log 2>&1 &
BACKEND_PID=$!
echo $BACKEND_PID > ../logs/backend.pid

# 等待后端启动
sleep 2

# 检查后端是否启动成功
if kill -0 $BACKEND_PID 2>/dev/null; then
    echo "✅ 后端启动成功 (PID: $BACKEND_PID)"
else
    echo "❌ 后端启动失败，请检查日志: logs/backend.log"
    exit 1
fi

# 启动前端
echo "🎨 启动前端服务..."
cd ../frontend
nohup npm run dev > ../logs/frontend.log 2>&1 &
FRONTEND_PID=$!
echo $FRONTEND_PID > ../logs/frontend.pid

# 等待前端启动
sleep 3

# 检查前端是否启动成功
if kill -0 $FRONTEND_PID 2>/dev/null; then
    echo "✅ 前端启动成功 (PID: $FRONTEND_PID)"
else
    echo "❌ 前端启动失败，请检查日志: logs/frontend.log"
    exit 1
fi

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✅ 所有服务启动完成！"
echo ""
echo "📍 服务地址："
echo "   - 后端 API: http://localhost:8080"
echo "   - 前端界面: http://localhost:3000"
echo "   - 健康检查: http://localhost:8080/health"
echo ""
echo "📝 日志文件："
echo "   - 后端: logs/backend.log"
echo "   - 前端: logs/frontend.log"
echo ""
echo "💡 停止服务: ./stop.sh"
echo "💡 查看日志: tail -f logs/backend.log"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
