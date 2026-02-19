#!/bin/bash
# MagicPodcast 服务重启脚本
# 安全停止并重启前后端服务

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

PROJECT_DIR="/Users/rookiestar/VSCode/Projects/MagicPodcast"
BACKEND_DIR="$PROJECT_DIR/backend"
FRONTEND_DIR="$PROJECT_DIR/frontend"

echo -e "${BLUE}========================================"
echo "  MagicPodcast 服务重启"
echo "  $(date '+%Y-%m-%d %H:%M:%S')"
echo -e "========================================${NC}"
echo ""

# 1. 停止现有服务
echo -e "${YELLOW}[1] 停止现有服务...${NC}"

# 停止后端
if pgrep -f "backend/api" > /dev/null 2>&1; then
    echo "  停止后端服务..."
    pkill -f "backend/api" 2>/dev/null || true
    sleep 1
    # 强制杀死残留进程
    if pgrep -f "backend/api" > /dev/null 2>&1; then
        echo "  强制停止后端..."
        pkill -9 -f "backend/api" 2>/dev/null || true
    fi
    echo -e "  ${GREEN}✓ 后端已停止${NC}"
else
    echo "  后端未运行"
fi

# 停止前端
if pgrep -f "next" > /dev/null 2>&1; then
    echo "  停止前端服务..."
    pkill -f "next" 2>/dev/null || true
    sleep 2
    # 强制杀死残留进程
    if pgrep -f "next" > /dev/null 2>&1; then
        echo "  强制停止前端..."
        pkill -9 -f "next" 2>/dev/null || true
        sleep 1
    fi
    echo -e "  ${GREEN}✓ 前端已停止${NC}"
else
    echo "  前端未运行"
fi

# 释放端口
echo ""
echo -e "${YELLOW}[2] 检查端口...${NC}"
for port in 8080 3000 3001; do
    if lsof -i :$port -P -n 2>/dev/null | grep -q "LISTEN"; then
        PID=$(lsof -i :$port -P -n 2>/dev/null | grep "LISTEN" | awk '{print $2}' | head -1)
        echo "  释放端口 $port (PID: $PID)..."
        kill -9 $PID 2>/dev/null || true
    fi
done
echo -e "  ${GREEN}✓ 端口已清理${NC}"

# 2. 可选：清理缓存
if [ "$1" = "--clean" ] || [ "$1" = "-c" ]; then
    echo ""
    echo -e "${YELLOW}[3] 清理缓存...${NC}"

    # 清理 webpack 缓存
    if [ -d "$FRONTEND_DIR/.next/cache/webpack" ]; then
        rm -rf "$FRONTEND_DIR/.next/cache/webpack"
        echo "  ✓ 已清理 webpack 缓存"
    fi

    # 清理整个 .next 目录（更彻底）
    if [ "$1" = "--full-clean" ]; then
        if [ -d "$FRONTEND_DIR/.next" ]; then
            rm -rf "$FRONTEND_DIR/.next"
            echo "  ✓ 已清理 .next 目录"
        fi
    fi
fi

# 3. 启动服务
echo ""
echo -e "${YELLOW}[3] 启动服务...${NC}"

# 启动后端
echo "  启动后端服务..."
cd "$BACKEND_DIR"
if [ ! -f "./api" ]; then
    echo -e "  ${YELLOW}编译后端...${NC}"
    go build -o api ./cmd/api
fi

# 加载环境变量
if [ -f ".env" ]; then
    export $(cat .env | grep -v '^#' | xargs)
fi

nohup ./api > /tmp/magicpodcast-backend.log 2>&1 &
BACKEND_PID=$!
echo "  后端启动中... (PID: $BACKEND_PID)"

# 等待后端启动
sleep 3
if curl -s http://localhost:8080/health > /dev/null 2>&1; then
    echo -e "  ${GREEN}✓ 后端启动成功${NC}"
else
    echo -e "  ${RED}✗ 后端启动失败，检查日志: /tmp/magicpodcast-backend.log${NC}"
fi

# 启动前端
echo ""
echo "  启动前端服务..."
cd "$FRONTEND_DIR"

# 检查是否需要构建
if [ ! -f ".next/BUILD_ID" ]; then
    echo -e "  ${YELLOW}首次运行，构建前端...${NC}"
    npm run build
fi

nohup npm run start > /tmp/magicpodcast-frontend.log 2>&1 &
FRONTEND_PID=$!
echo "  前端启动中... (PID: $FRONTEND_PID)"

# 等待前端启动
sleep 4
FRONTEND_STATUS=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:3000 2>/dev/null)
if [ "$FRONTEND_STATUS" = "200" ]; then
    echo -e "  ${GREEN}✓ 前端启动成功${NC}"
else
    echo -e "  ${RED}✗ 前端启动失败 (HTTP $FRONTEND_STATUS)，检查日志: /tmp/magicpodcast-frontend.log${NC}"
fi

# 4. 显示状态
echo ""
echo -e "${BLUE}========================================"
echo "  服务状态"
echo -e "========================================${NC}"

echo ""
echo "  后端: http://localhost:8080"
echo "  前端: http://localhost:3000"
echo ""
echo "  日志位置:"
echo "    后端: /tmp/magicpodcast-backend.log"
echo "    前端: /tmp/magicpodcast-frontend.log"
echo ""

# 健康检查
echo -e "${YELLOW}健康检查:${NC}"
HEALTH=$(curl -s http://localhost:8080/health 2>/dev/null)
if [ $? -eq 0 ]; then
    echo -e "  后端: ${GREEN}$HEALTH${NC}"
else
    echo -e "  后端: ${RED}无响应${NC}"
fi

FRONTEND_STATUS=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:3000 2>/dev/null)
if [ "$FRONTEND_STATUS" = "200" ]; then
    echo -e "  前端: ${GREEN}HTTP $FRONTEND_STATUS${NC}"
else
    echo -e "  前端: ${RED}HTTP $FRONTEND_STATUS${NC}"
fi

echo ""
echo -e "${GREEN}完成！${NC}"
