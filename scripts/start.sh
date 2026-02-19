#!/bin/bash
# MagicPodcast 开发环境启动脚本
# 用法: ./start.sh [--clean]

set -e

PROJECT_DIR="/Users/rookiestar/VSCode/Projects/MagicPodcast"
FRONTEND_DIR="$PROJECT_DIR/frontend"
BACKEND_DIR="$PROJECT_DIR/backend"
FRONTEND_PID_FILE="/tmp/magicpodcast-frontend.pid"
BACKEND_PID_FILE="/tmp/magicpodcast-backend.pid"
FRONTEND_LOG="/tmp/magicpodcast-frontend.log"
BACKEND_LOG="/tmp/magicpodcast-backend.log"

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

print_status() { echo -e "${GREEN}✓${NC} $1"; }
print_warning() { echo -e "${YELLOW}⚠${NC} $1"; }
print_error() { echo -e "${RED}✗${NC} $1"; }

echo -e "${BLUE}========================================"
echo "  MagicPodcast 开发环境启动"
echo "  $(date '+%Y-%m-%d %H:%M:%S')"
echo -e "========================================${NC}"
echo ""

# 可选清理缓存
if [ "$1" = "--clean" ] || [ "$1" = "-c" ]; then
    echo -e "${YELLOW}[0] 清理缓存...${NC}"
    rm -rf "$FRONTEND_DIR/.next/cache/webpack" 2>/dev/null || true
    print_status "Webpack 缓存已清理"
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
if [ -f "$BACKEND_PID_FILE" ] && ps -p $(cat "$BACKEND_PID_FILE" 2>/dev/null) > /dev/null 2>&1; then
    print_warning "后端已在运行 (PID: $(cat $BACKEND_PID_FILE))"
else
    cd "$BACKEND_DIR"

    # 加载环境变量
    if [ -f ".env" ]; then
        export $(cat .env | grep -v '^#' | xargs)
    fi

    # 检查是否需要编译
    if [ ! -f "./api" ]; then
        echo "  编译后端..."
        go build -o api ./cmd/api
    fi

    nohup ./api > "$BACKEND_LOG" 2>&1 &
    echo $! > "$BACKEND_PID_FILE"
    sleep 3

    if ps -p $(cat "$BACKEND_PID_FILE") > /dev/null 2>&1; then
        print_status "后端已启动 (PID: $(cat $BACKEND_PID_FILE))"
    else
        print_error "后端启动失败"
        echo "  查看日志: tail -f $BACKEND_LOG"
        exit 1
    fi
    cd "$PROJECT_DIR"
fi
echo ""

# 启动前端
echo -e "${YELLOW}[3] 启动前端服务...${NC}"
if [ -f "$FRONTEND_PID_FILE" ] && ps -p $(cat "$FRONTEND_PID_FILE" 2>/dev/null) > /dev/null 2>&1; then
    print_warning "前端已在运行 (PID: $(cat $FRONTEND_PID_FILE))"
else
    cd "$FRONTEND_DIR"

    # 检查是否需要安装依赖
    if [ ! -d "node_modules" ]; then
        echo "  安装依赖..."
        npm install
    fi

    nohup npm run dev > "$FRONTEND_LOG" 2>&1 &
    echo $! > "$FRONTEND_PID_FILE"
    sleep 5

    if ps -p $(cat "$FRONTEND_PID_FILE") > /dev/null 2>&1; then
        print_status "前端已启动 (PID: $(cat $FRONTEND_PID_FILE))"
    else
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
BACKEND_HEALTH=$(curl -s http://localhost:8080/health 2>/dev/null)
if [ $? -eq 0 ]; then
    print_status "后端: $BACKEND_HEALTH"
else
    print_warning "后端: 无响应"
fi

FRONTEND_STATUS=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:3000 2>/dev/null)
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
echo "    ./health.sh   - 健康检查"
echo ""
