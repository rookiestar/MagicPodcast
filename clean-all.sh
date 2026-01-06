#!/bin/bash
# MagicPodcast 项目统一清理脚本
# 清理前端和后端的所有缓存

set -e

echo "🧹 开始清理整个项目..."

# 颜色定义
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 获取脚本所在目录
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
cd "$SCRIPT_DIR"

echo -e "${BLUE}================================================${NC}"
echo -e "${BLUE}   MagicPodcast 项目清理工具${NC}"
echo -e "${BLUE}================================================${NC}"
echo ""

# 解析参数
DEEP_CLEAN=false
VACUUM_DB=false

for arg in "$@"; do
    case $arg in
        --deep)
            DEEP_CLEAN=true
            ;;
        --vacuum)
            VACUUM_DB=true
            ;;
        --help)
            echo "用法: ./clean-all.sh [选项]"
            echo ""
            echo "选项:"
            echo "  --deep    深度清理（包括 node_modules/.cache）"
            echo "  --vacuum  优化数据库（回收空间）"
            echo "  --help    显示此帮助信息"
            echo ""
            echo "示例:"
            echo "  ./clean-all.sh              # 常规清理"
            echo "  ./clean-all.sh --deep       # 深度清理"
            echo "  ./clean-all.sh --vacuum     # 清理并优化数据库"
            echo "  ./clean-all.sh --deep --vacuum  # 深度清理并优化数据库"
            exit 0
            ;;
    esac
done

# 1. 清理前端
echo -e "${YELLOW}📱 清理前端...${NC}"
if [ -d "frontend" ]; then
    cd frontend

    # 构建参数
    CLEAN_ARGS=""
    if [ "$DEEP_CLEAN" = true ]; then
        CLEAN_ARGS="--deep"
    fi

    if [ -x "clean.sh" ]; then
        ./clean.sh $CLEAN_ARGS
    else
        echo -e "${YELLOW}警告: frontend/clean.sh 不可执行，尝试直接执行...${NC}"
        bash clean.sh $CLEAN_ARGS
    fi

    cd ..
else
    echo -e "${YELLOW}⚠️  frontend 目录不存在${NC}"
fi

echo ""
echo "================================================"
echo ""

# 2. 清理后端
echo -e "${YELLOW}🔧 清理后端...${NC}"
if [ -d "backend" ]; then
    cd backend

    # 构建参数
    CLEAN_ARGS=""
    if [ "$VACUUM_DB" = true ]; then
        CLEAN_ARGS="--vacuum"
    fi

    if [ -x "clean.sh" ]; then
        ./clean.sh $CLEAN_ARGS
    else
        echo -e "${YELLOW}警告: backend/clean.sh 不可执行，尝试直接执行...${NC}"
        bash clean.sh $CLEAN_ARGS
    fi

    cd ..
else
    echo -e "${YELLOW}⚠️  backend 目录不存在${NC}"
fi

echo ""
echo "================================================"
echo ""

# 3. 清理 Git 相关（可选）
echo -e "${YELLOW}📊 Git 状态：${NC}"
if [ -d ".git" ]; then
    echo "当前分支: $(git branch --show-current 2>/dev/null || echo '未知')"
    echo "未提交的更改: $(git status --short 2>/dev/null | wc -l | xargs) 个文件"
fi

echo ""
echo -e "${GREEN}✅ 整个项目清理完成！${NC}"
echo ""
echo -e "${BLUE}💡 下一步操作：${NC}"
echo -e "   前端: cd frontend && npm run dev"
echo -e "   后端: cd backend && go run cmd/api/main.go"
echo ""
echo -e "${BLUE}💡 其他选项：${NC}"
echo -e "   深度清理: ./clean-all.sh --deep"
echo -e "   优化数据库: ./clean-all.sh --vacuum"
echo -e "   查看帮助: ./clean-all.sh --help"
