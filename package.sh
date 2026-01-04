#!/bin/bash

# MagicPodcast 快速部署打包脚本
# 用于创建可传输到其他开发机的部署包

OUTPUT_FILE="magicpodcast-deploy"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
ARCHIVE_NAME="${OUTPUT_FILE}_${TIMESTAMP}.tar.gz"

echo "📦 MagicPodcast 部署包打包工具"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# 检查是否在项目根目录
if [ ! -f "backend/go.mod" ] || [ ! -f "frontend/package.json" ]; then
    echo "❌ 错误：请在项目根目录运行此脚本"
    echo "   应该能看到 backend/go.mod 和 frontend/package.json"
    exit 1
fi

echo "📋 打包配置："
echo "   输出文件: $ARCHIVE_NAME"
echo ""

# 询问是否包含 PodcastIndex.db
echo "❓ 是否包含 PodcastIndex.db (4.5GB)?"
echo "   [1] 不包含 (推荐，可稍后单独传输)"
echo "   [2] 包含 (如果两台机器间网络速度快)"
read -p "   请选择 (1 或 2): " include_podcastindex

if [ "$include_podcastindex" = "2" ]; then
    echo "   ⚠️  将包含 PodcastIndex.db，打包会比较慢..."
    EXCLUDE_PODCASTINDEX=""
else
    echo "   ✓ 将排除 PodcastIndex.db"
    EXCLUDE_PODCASTINDEX="--exclude='backend/data/podcastindex_feeds.db'"
fi

echo ""
echo "📦 开始打包..."

# 创建临时目录
TEMP_DIR=$(mktemp -d)
mkdir -p "$TEMP_DIR/MagicPodcast"

# 复制文件
echo "📋 正在复制文件..."

# 排除列表
EXCLUDES=(
    "--exclude='.git'"
    "--exclude='node_modules'"
    "--exclude='frontend/.next'"
    "--exclude='frontend/.turbo'"
    "--exclude='__pycache__'"
    "--exclude='*.pyc'"
    "--exclude='.DS_Store'"
    "--exclude='logs/*'"
    "--exclude='*.log'"
    "--exclude='backend/data/*.db.backup'"
    "--exclude='backend/data/temp/*'"
)

# 复制项目文件
rsync -av \
    "${EXCLUDES[@]}" \
    $EXCLUDE_PODCASTINDEX \
    ./ \
    "$TEMP_DIR/MagicPodcast/" \
    > /dev/null 2>&1

# 复制 .git 目录但排除 .git/objects（节省空间）
mkdir -p "$TEMP_DIR/MagicPodcast/.git"
cp .git/config .git/HEAD "$TEMP_DIR/MagicPodcast/.git/" 2>/dev/null

# 创建必要的空目录
mkdir -p "$TEMP_DIR/MagicPodcast/backend/data"
mkdir -p "$TEMP_DIR/MagicPodcast/backend/data/temp"
mkdir -p "$TEMP_DIR/MagicPodcast/logs"

# 创建 README
cat > "$TEMP_DIR/MagicPodcast/DEPLOY_INSTRUCTIONS.txt" << 'EOF'
MagicPodcast 快速部署包
====================

解压后的部署步骤：

1. 解压文件：
   tar -xzf magicpodcast-deploy_*.tar.gz
   cd MagicPodcast

2. 安装 Go 依赖：
   cd backend
   go mod download

3. 安装前端依赖：
   cd ../frontend
   npm install

4. 启动服务：
   cd ..
   chmod +x start.sh stop.sh
   ./start.sh

5. 访问服务：
   前端: http://localhost:3000
   后端: http://localhost:8080
   健康检查: http://localhost:8080/health

详细文档请参考：DEPLOYMENT.md

注意事项：
- 如果本包不包含 PodcastIndex.db，可以稍后单独传输
- 配置文件: backend/configs/config.yaml
- 日志目录: logs/

祝开发顺利！
EOF

# 打包
echo "🗜️  正在压缩..."
cd "$TEMP_DIR"
tar -czf "$OLDPWD/$ARCHIVE_NAME" MagicPodcast/
cd "$OLDPWD"

# 获取文件大小
FILE_SIZE=$(du -h "$ARCHIVE_NAME" | cut -f1)

# 清理临时目录
rm -rf "$TEMP_DIR"

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✅ 打包完成！"
echo ""
echo "📦 部署包信息："
echo "   文件名: $ARCHIVE_NAME"
echo "   大小: $FILE_SIZE"
echo "   位置: $(pwd)/$ARCHIVE_NAME"
echo ""
echo "📤 传输方式："
echo "   1. USB 驱动器: 直接复制文件"
echo "   2. scp: scp $ARCHIVE_NAME user@new-machine:/path/to/destination/"
echo "   3. 云存储: 上传到 iCloud Drive / Google Drive / Dropbox"
echo ""
echo "📥 在新机器上解压："
echo "   tar -xzf $ARCHIVE_NAME"
echo "   cd MagicPodcast"
echo "   chmod +x start.sh stop.sh"
echo "   ./start.sh"
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
