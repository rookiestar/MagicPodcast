#!/bin/bash
# MagicPodcast 开机自启脚本
# 由 launchd 在登录时调用

PROJECT_DIR="/Users/rookiestar/VSCode/Projects/MagicPodcast"
LOG_FILE="/tmp/magicpodcast-startup.log"

log() {
    echo "$(date '+%Y-%m-%d %H:%M:%S') - $1" >> "$LOG_FILE"
}

log "=== MagicPodcast 启动开始 ==="

cd "$PROJECT_DIR" || exit 1

# 等待网络就绪
sleep 5

# 启动后端和前端
log "启动本地服务..."
./dev.sh start >> "$LOG_FILE" 2>&1

# 等待服务启动
sleep 5

# 启动 Cloudflare Tunnel
log "启动 Cloudflare Tunnel..."
./scripts/cloudflare-tunnel-named.sh start >> "$LOG_FILE" 2>&1

log "=== MagicPodcast 启动完成 ==="
