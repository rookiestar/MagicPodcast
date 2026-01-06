#!/bin/bash
# MagicPodcast Backend 清理脚本
# 清理 Go 构建缓存、日志文件、临时数据库等

set -e

echo "🧹 开始清理后端缓存..."

# 颜色定义
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# 1. 清理 Go 构建缓存
echo -e "${YELLOW}清理 Go 构建缓存...${NC}"
if [ -d "bin" ]; then
    rm -rf bin/*
    echo -e "${GREEN}✓ 已清空 bin 目录${NC}"
else
    echo -e "${GREEN}✓ bin 目录不存在，跳过${NC}"
fi

# 2. 清理日志文件（保留最新的一个）
echo -e "${YELLOW}清理旧日志文件...${NC}"
if [ -f "api.log" ]; then
    # 只删除超过 7 天的日志备份
    find . -name "api.log.backup_*" -mtime +7 -delete 2>/dev/null || true
    echo -e "${GREEN}✓ 已清理 7 天前的日志备份${NC}"
else
    echo -e "${GREEN}✓ 无日志文件需要清理${NC}"
fi

# 3. 清理数据目录中的临时文件
echo -e "${YELLOW}清理数据目录中的临时文件...${NC}"
cd data

# 清理 SQLite WAL 和 SHM 文件（这些会在数据库关闭时自动清理）
find . -name "*.db-wal" -o -name "*.db-shm" | while read file; do
    # 检查是否有对应的 .db 文件
    dbfile="${file%-wal}"
    dbfile="${dbfile%-shm}"
    if [ -f "$dbfile" ]; then
        # 尝试删除（如果数据库未打开）
        rm -f "$file" 2>/dev/null && echo -e "${GREEN}✓ 已删除 $file${NC}" || true
    fi
done

# 清理空的临时目录
if [ -d "temp" ]; then
    # 只删除空目录
    find temp -type d -empty -delete 2>/dev/null || true
    echo -e "${GREEN}✓ 已清理空的临时目录${NC}"
fi

cd ..

# 4. 清理测试文件
echo -e "${YELLOW}清理测试文件...${NC}"
find . -maxdepth 1 -name "test_*.go" -type f | while read file; do
    echo -e "${GREEN}✓ 发现测试文件: $file${NC}"
done

# 5. 数据库维护（需要参数 --vacuum）
if [ "$1" == "--vacuum" ]; then
    echo -e "${YELLOW}执行数据库 VACUUM（回收空间）...${NC}"
    cd data
    for db in *.db; do
        if [ -f "$db" ]; then
            echo "正在优化 $db ..."
            sqlite3 "$db" "VACUUM;"
            echo -e "${GREEN}✓ 已优化 $db${NC}"
        fi
    done
    cd ..
fi

# 6. 显示数据库状态
echo -e "${YELLOW}数据库状态：${NC}"
cd data
for db in *.db; do
    if [ -f "$db" ]; then
        size=$(du -h "$db" | cut -f1)
        echo -e "  📦 $db: $size"
    fi
done
cd ..

echo ""
echo -e "${GREEN}✅ 后端清理完成！${NC}"
echo -e "${YELLOW}💡 提示：运行 'go run cmd/api/main.go' 重新启动服务器${NC}"
echo -e "${YELLOW}💡 数据库优化请使用: ./clean.sh --vacuum${NC}"
