#!/bin/bash
# MagicPodcast Frontend 清理脚本
# 清理 Next.js 缓存、构建产物等

set -e

echo "🧹 开始清理前端缓存..."

# 颜色定义
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# 1. 清理 Next.js 缓存
echo -e "${YELLOW}清理 Next.js 缓存...${NC}"
if [ -d ".next" ]; then
    rm -rf .next
    echo -e "${GREEN}✓ 已删除 .next 目录${NC}"
else
    echo -e "${GREEN}✓ .next 目录不存在，跳过${NC}"
fi

# 2. 清理构建输出
echo -e "${YELLOW}清理构建输出...${NC}"
if [ -d "out" ]; then
    rm -rf out
    echo -e "${GREEN}✓ 已删除 out 目录${NC}"
else
    echo -e "${GREEN}✓ out 目录不存在，跳过${NC}"
fi

# 3. 清理 TypeScript 缓存
echo -e "${YELLOW}清理 TypeScript 缓存...${NC}"
if [ -d "*.tsbuildinfo" ]; then
    rm -f *.tsbuildinfo
    echo -e "${GREEN}✓ 已删除 .tsbuildinfo 文件${NC}"
else
    echo -e "${GREEN}✓ 无 TypeScript 缓存文件${NC}"
fi

# 4. 清理 ESLint 缓存
echo -e "${YELLOW}清理 ESLint 缓存...${NC}"
if [ -f ".eslintcache" ]; then
    rm -f .eslintcache
    echo -e "${GREEN}✓ 已删除 .eslintcache 文件${NC}"
else
    echo -e "${GREEN}✓ 无 ESLint 缓存文件${NC}"
fi

# 5. 清理 node_modules/.cache (可选，需要参数)
if [ "$1" == "--deep" ]; then
    echo -e "${YELLOW}深度清理：清理 node_modules/.cache...${NC}"
    if [ -d "node_modules/.cache" ]; then
        rm -rf node_modules/.cache
        echo -e "${GREEN}✓ 已删除 node_modules/.cache 目录${NC}"
    else
        echo -e "${GREEN}✓ node_modules/.cache 不存在${NC}"
    fi
fi

echo ""
echo -e "${GREEN}✅ 前端清理完成！${NC}"
echo -e "${YELLOW}💡 提示：运行 'npm run dev' 重新启动开发服务器${NC}"
echo -e "${YELLOW}💡 深度清理请使用: ./clean.sh --deep${NC}"
