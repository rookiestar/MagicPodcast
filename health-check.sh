#!/bin/bash

echo "🔍 MagicPodcast 部署健康检查"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# 颜色定义
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 检查计数器
PASS_COUNT=0
FAIL_COUNT=0
WARN_COUNT=0

check_pass() {
    echo -e "${GREEN}✅ $1${NC}"
    ((PASS_COUNT++))
}

check_fail() {
    echo -e "${RED}❌ $1${NC}"
    ((FAIL_COUNT++))
}

check_warn() {
    echo -e "${YELLOW}⚠️  $1${NC}"
    ((WARN_COUNT++))
}

echo "📋 环境检查"
echo "─────────────────────────────────────────"

# 检查 Go
if command -v go &> /dev/null; then
    GO_VERSION=$(go version | awk '{print $3}')
    check_pass "Go 已安装: $GO_VERSION"
else
    check_fail "Go 未安装"
fi

# 检查 Node.js
if command -v node &> /dev/null; then
    NODE_VERSION=$(node --version)
    check_pass "Node.js 已安装: $NODE_VERSION"
else
    check_fail "Node.js 未安装"
fi

# 检查 npm
if command -v npm &> /dev/null; then
    NPM_VERSION=$(npm --version)
    check_pass "npm 已安装: $NPM_VERSION"
else
    check_fail "npm 未安装"
fi

# 检查 Git
if command -v git &> /dev/null; then
    GIT_VERSION=$(git --version)
    check_pass "Git 已安装: $GIT_VERSION"
else
    check_fail "Git 未安装"
fi

echo ""
echo "📂 项目结构检查"
echo "─────────────────────────────────────────"

# 检查关键目录
[ -d "backend" ] && check_pass "backend 目录存在" || check_fail "backend 目录不存在"
[ -d "frontend" ] && check_pass "frontend 目录存在" || check_fail "frontend 目录不存在"
[ -f "backend/go.mod" ] && check_pass "Go 模块文件存在" || check_fail "Go 模块文件不存在"
[ -f "frontend/package.json" ] && check_pass "前端配置文件存在" || check_fail "前端配置文件不存在"
[ -f "backend/configs/config.yaml" ] && check_pass "后端配置文件存在" || check_warn "后端配置文件不存在 (需要创建)"
[ -d "backend/data" ] && check_pass "数据目录存在" || check_warn "数据目录不存在 (需要创建)"
[ -d "logs" ] && check_pass "日志目录存在" || check_warn "日志目录不存在 (运行 start.sh 会自动创建)"

echo ""
echo "📦 依赖检查"
echo "─────────────────────────────────────────"

# 检查 Go 依赖
if [ -d "backend/vendor" ] || grep -q "require" backend/go.mod 2>/dev/null; then
    check_pass "Go 依赖已配置"
else
    check_warn "Go 依赖可能未安装 (运行: cd backend && go mod download)"
fi

# 检查前端依赖
if [ -d "frontend/node_modules" ]; then
    check_pass "前端依赖已安装"
else
    check_warn "前端依赖未安装 (运行: cd frontend && npm install)"
fi

echo ""
echo "🗄️  数据库检查"
echo "─────────────────────────────────────────"

# 检查主数据库
if [ -f "backend/data/magicpodcast.db" ]; then
    DB_SIZE=$(du -h "backend/data/magicpodcast.db" | cut -f1)
    check_pass "主数据库存在 (大小: $DB_SIZE)"
else
    check_warn "主数据库不存在 (首次启动会自动创建)"
fi

# 检查 PodcastIndex 数据库
if [ -f "backend/data/podcastindex_feeds.db" ]; then
    PI_SIZE=$(du -h "backend/data/podcastindex_feeds.db" | cut -f1)
    check_pass "PodcastIndex 数据库存在 (大小: $PI_SIZE)"
else
    check_warn "PodcastIndex 数据库不存在 (可选，功能会受限)"
fi

echo ""
echo "🌐 服务检查"
echo "─────────────────────────────────────────"

# 检查后端服务
BACKEND_RUNNING=false
if lsof -Pi :8080 -sTCP:LISTEN -t >/dev/null 2>&1; then
    check_pass "后端服务正在运行 (端口 8080)"
    BACKEND_RUNNING=true

    # 测试健康检查
    if command -v curl &> /dev/null; then
        HEALTH_RESPONSE=$(curl -s http://localhost:8080/health 2>/dev/null)
        if echo "$HEALTH_RESPONSE" | grep -q '"status":"ok"'; then
            check_pass "后端健康检查通过"
        else
            check_fail "后端健康检查失败"
        fi
    fi
else
    check_warn "后端服务未运行 (运行: ./start.sh)"
fi

# 检查前端服务
if lsof -Pi :3000 -sTCP:LISTEN -t >/dev/null 2>&1; then
    check_pass "前端服务正在运行 (端口 3000)"
else
    check_warn "前端服务未运行 (运行: ./start.sh)"
fi

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "📊 检查结果统计"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo -e "${GREEN}✅ 通过: $PASS_COUNT${NC}"
echo -e "${YELLOW}⚠️  警告: $WARN_COUNT${NC}"
echo -e "${RED}❌ 失败: $FAIL_COUNT${NC}"

if [ $FAIL_COUNT -eq 0 ] && [ $WARN_COUNT -eq 0 ]; then
    echo ""
    echo -e "${GREEN}🎉 所有检查通过！部署状态良好。${NC}"
    exit 0
elif [ $FAIL_COUNT -eq 0 ]; then
    echo ""
    echo -e "${YELLOW}⚠️  部分功能需要配置，但可以正常开发。${NC}"
    exit 0
else
    echo ""
    echo -e "${RED}❌ 发现问题，请根据上述提示进行修复。${NC}"
    exit 1
fi
