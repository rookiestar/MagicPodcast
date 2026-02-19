#!/bin/bash
# MagicPodcast 健康检查脚本
# 在调试前运行，快速诊断环境状态

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 项目目录
PROJECT_DIR="/Users/rookiestar/VSCode/Projects/MagicPodcast"

echo "========================================"
echo "  MagicPodcast 健康检查"
echo "  $(date '+%Y-%m-%d %H:%M:%S')"
echo "========================================"
echo ""

# 1. 端口检查
echo -e "${YELLOW}[1] 端口检查${NC}"
echo "-------------------------------------------"

check_port() {
    local port=$1
    local name=$2
    if lsof -i :$port -P -n 2>/dev/null | grep -q "LISTEN"; then
        local pid=$(lsof -i :$port -P -n 2>/dev/null | grep "LISTEN" | awk '{print $2}' | head -1)
        local cmd=$(ps -p $pid -o comm= 2>/dev/null || echo "unknown")
        echo -e "  端口 $port ($name): ${RED}被占用${NC} [PID: $pid, 进程: $cmd]"
        return 1
    else
        echo -e "  端口 $port ($name): ${GREEN}可用${NC}"
        return 0
    fi
}

PORT_8080_OK=true
PORT_3000_OK=true
check_port 8080 "后端" || PORT_8080_OK=false
check_port 3000 "前端" || PORT_3000_OK=false
echo ""

# 2. 进程检查
echo -e "${YELLOW}[2] 进程检查${NC}"
echo "-------------------------------------------"

check_process() {
    local pattern=$1
    local name=$2
    if pgrep -f "$pattern" > /dev/null 2>&1; then
        local pid=$(pgrep -f "$pattern" | head -1)
        echo -e "  $name: ${GREEN}运行中${NC} [PID: $pid]"
        return 0
    else
        echo -e "  $name: ${RED}未运行${NC}"
        return 1
    fi
}

BACKEND_RUNNING=false
FRONTEND_RUNNING=false
check_process "backend/api" "后端服务" && BACKEND_RUNNING=true
check_process "next start" "前端服务" && FRONTEND_RUNNING=true
echo ""

# 3. 缓存检查
echo -e "${YELLOW}[3] 缓存检查${NC}"
echo "-------------------------------------------"

WEBPACK_CACHE="$PROJECT_DIR/frontend/.next/cache/webpack"
if [ -d "$WEBPACK_CACHE" ]; then
    SIZE=$(du -sh "$WEBPACK_CACHE" 2>/dev/null | cut -f1)
    echo -e "  Webpack 缓存: ${YELLOW}存在${NC} (大小: $SIZE)"
    echo -e "    清理命令: rm -rf frontend/.next/cache/webpack"
else
    echo -e "  Webpack 缓存: ${GREEN}不存在${NC}"
fi

NEXT_BUILD="$PROJECT_DIR/frontend/.next/BUILD_ID"
if [ -f "$NEXT_BUILD" ]; then
    BUILD_ID=$(cat "$NEXT_BUILD")
    echo -e "  Next.js 构建: ${GREEN}存在${NC} (BUILD_ID: $BUILD_ID)"
else
    echo -e "  Next.js 构建: ${RED}不存在${NC} (需要运行 npm run build)"
fi
echo ""

# 4. 数据库检查
echo -e "${YELLOW}[4] 数据库检查${NC}"
echo "-------------------------------------------"

DB_FILE="$PROJECT_DIR/backend/data/magicpodcast.db"
if [ -f "$DB_FILE" ]; then
    SIZE=$(du -sh "$DB_FILE" | cut -f1)
    echo -e "  数据库文件: ${GREEN}存在${NC} (大小: $SIZE)"

    # 检查表记录数
    PODCASTS=$(sqlite3 "$DB_FILE" "SELECT COUNT(*) FROM podcasts;" 2>/dev/null || echo "N/A")
    EPISODES=$(sqlite3 "$DB_FILE" "SELECT COUNT(*) FROM episodes;" 2>/dev/null || echo "N/A")
    TAGS=$(sqlite3 "$DB_FILE" "SELECT COUNT(*) FROM tags;" 2>/dev/null || echo "N/A")
    TAG_RELS=$(sqlite3 "$DB_FILE" "SELECT COUNT(*) FROM podcasts_tags;" 2>/dev/null || echo "N/A")

    echo "    播客数: $PODCASTS"
    echo "    单集数: $EPISODES"
    echo "    标签数: $TAGS"
    echo "    标签关联数: $TAG_RELS"

    # 检查外键状态
    FK_STATUS=$(sqlite3 "$DB_FILE" "PRAGMA foreign_keys;" 2>/dev/null || echo "N/A")
    echo "    外键状态: $([ "$FK_STATUS" = "1" ] && echo "启用" || echo "禁用")"
else
    echo -e "  数据库文件: ${RED}不存在${NC}"
fi
echo ""

# 5. 最近备份检查
echo -e "${YELLOW}[5] 备份检查${NC}"
echo "-------------------------------------------"

BACKUP_DIR="$PROJECT_DIR/backend/data"
LATEST_BACKUP=$(ls -t "$BACKUP_DIR"/*.db.bak 2>/dev/null | head -1)
if [ -n "$LATEST_BACKUP" ]; then
    BACKUP_TIME=$(stat -f "%Sm" "$LATEST_BACKUP" 2>/dev/null || stat -c "%y" "$LATEST_BACKUP" 2>/dev/null)
    echo -e "  最新备份: ${GREEN}存在${NC}"
    echo "    文件: $LATEST_BACKUP"
    echo "    时间: $BACKUP_TIME"
else
    echo -e "  最新备份: ${YELLOW}未找到${NC}"
fi
echo ""

# 6. API 健康检查
echo -e "${YELLOW}[6] API 健康检查${NC}"
echo "-------------------------------------------"

if [ "$BACKEND_RUNNING" = true ]; then
    HEALTH=$(curl -s http://localhost:8080/health 2>/dev/null)
    if [ $? -eq 0 ]; then
        echo -e "  /health: ${GREEN}正常${NC}"
        echo "    响应: $HEALTH"
    else
        echo -e "  /health: ${RED}无响应${NC}"
    fi

    if [ "$FRONTEND_RUNNING" = true ]; then
        FRONTEND_STATUS=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:3000 2>/dev/null)
        if [ "$FRONTEND_STATUS" = "200" ]; then
            echo -e "  前端页面: ${GREEN}正常${NC} (HTTP $FRONTEND_STATUS)"
        else
            echo -e "  前端页面: ${YELLOW}异常${NC} (HTTP $FRONTEND_STATUS)"
        fi
    fi
else
    echo -e "  API: ${YELLOW}服务未运行，跳过检查${NC}"
fi
echo ""

# 7. 总结和建议
echo "========================================"
echo -e "${YELLOW}诊断总结${NC}"
echo "========================================"

ISSUES=0

if [ "$PORT_8080_OK" = false ]; then
    echo -e "  ${RED}⚠${NC} 端口 8080 被占用"
    ISSUES=$((ISSUES + 1))
fi

if [ "$PORT_3000_OK" = false ]; then
    echo -e "  ${RED}⚠${NC} 端口 3000 被占用"
    ISSUES=$((ISSUES + 1))
fi

if [ ! -f "$NEXT_BUILD" ]; then
    echo -e "  ${RED}⚠${NC} 前端需要构建"
    ISSUES=$((ISSUES + 1))
fi

if [ -d "$WEBPACK_CACHE" ]; then
    echo -e "  ${YELLOW}ℹ${NC} 存在 webpack 缓存（如有热更新问题可清理）"
fi

if [ $ISSUES -eq 0 ]; then
    echo -e "  ${GREEN}✓ 环境状态良好${NC}"
else
    echo ""
    echo -e "  ${YELLOW}建议操作:${NC}"
    [ "$PORT_8080_OK" = false ] && echo "    - 运行: ./scripts/stop.sh 或 pkill -f 'backend/api'"
    [ "$PORT_3000_OK" = false ] && echo "    - 运行: ./scripts/stop.sh 或 pkill -f 'next'"
    [ ! -f "$NEXT_BUILD" ] && echo "    - 运行: cd frontend && npm run build"
fi

echo ""
