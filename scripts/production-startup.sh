#!/bin/bash
# MagicPodcast 生产环境开机自启脚本
# 由 launchd 在登录时调用

PROJECT_DIR="/Users/rookiestar/VSCode/Projects/MagicPodcast"
BACKEND_DIR="$PROJECT_DIR/backend"
FRONTEND_DIR="$PROJECT_DIR/frontend"
LOG_FILE="/tmp/magicpodcast-production.log"
BACKEND_PID_FILE="/tmp/magicpodcast-backend.pid"
FRONTEND_PID_FILE="/tmp/magicpodcast-frontend.pid"

log() {
    echo "$(date '+%Y-%m-%d %H:%M:%S') - $1" >> "$LOG_FILE"
}

log "=== MagicPodcast 生产环境启动开始 ==="

cd "$PROJECT_DIR" || exit 1

# 等待网络就绪
sleep 3

# 检查并启动后端
if [ -f "$BACKEND_PID_FILE" ] && ps -p $(cat "$BACKEND_PID_FILE" 2>/dev/null) > /dev/null 2>&1; then
    log "后端已在运行 (PID: $(cat $BACKEND_PID_FILE))"
else
    log "启动后端服务..."
    cd "$BACKEND_DIR"

    # 加载环境变量
    if [ -f ".env" ]; then
        export $(cat .env | grep -v '^#' | xargs)
    fi

    # 使用编译好的二进制文件
    if [ -f "./api" ]; then
        nohup ./api >> "$LOG_FILE" 2>&1 &
        echo $! > "$BACKEND_PID_FILE"
        sleep 2
        if ps -p $(cat "$BACKEND_PID_FILE") > /dev/null 2>&1; then
            log "后端已启动 (PID: $(cat $BACKEND_PID_FILE))"
        else
            log "错误: 后端启动失败"
        fi
    else
        log "错误: 后端二进制文件不存在，请先编译: cd backend && go build -o api ./cmd/api"
    fi
    cd "$PROJECT_DIR"
fi

# 检查并启动前端
if [ -f "$FRONTEND_PID_FILE" ] && ps -p $(cat "$FRONTEND_PID_FILE" 2>/dev/null) > /dev/null 2>&1; then
    log "前端已在运行 (PID: $(cat $FRONTEND_PID_FILE))"
else
    log "启动前端服务..."
    cd "$FRONTEND_DIR"

    if [ ! -d "node_modules" ]; then
        log "安装前端依赖..."
        npm install >> "$LOG_FILE" 2>&1
    fi

    log "构建前端生产版本..."
    export NEXT_PUBLIC_IMAGE_OPTIMIZER_PATH="${NEXT_PUBLIC_IMAGE_OPTIMIZER_PATH:-/_next/image.webp}"
    rm -rf .next
    if ! npm run build >> "$LOG_FILE" 2>&1; then
        log "错误: 前端构建失败"
        cd "$PROJECT_DIR"
        exit 1
    fi

    # 使用生产模式启动
    nohup npm run start >> "$LOG_FILE" 2>&1 &
    echo $! > "$FRONTEND_PID_FILE"
    sleep 3
    if ps -p $(cat "$FRONTEND_PID_FILE") > /dev/null 2>&1; then
        log "前端已启动 (PID: $(cat $FRONTEND_PID_FILE))"
    else
        log "错误: 前端启动失败"
    fi
    cd "$PROJECT_DIR"
fi

log "=== MagicPodcast 生产环境启动完成 ==="
