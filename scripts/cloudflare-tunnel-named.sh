#!/bin/bash
# MagicPodcast Cloudflare Named Tunnel 启动脚本
# 使用方法: ./scripts/cloudflare-tunnel-named.sh start|stop|status

TUNNEL_NAME="magicpodcast"
LOG_FILE="/tmp/cloudflared-named.log"
PID_FILE="/tmp/cloudflared-named.pid"

start() {
    if [ -f "$PID_FILE" ] && kill -0 $(cat "$PID_FILE") 2>/dev/null; then
        echo "Tunnel already running (PID: $(cat $PID_FILE))"
        return 1
    fi

    echo "Starting Cloudflare Tunnel: $TUNNEL_NAME"
    nohup cloudflared tunnel run "$TUNNEL_NAME" > "$LOG_FILE" 2>&1 &
    echo $! > "$PID_FILE"
    sleep 3

    if kill -0 $(cat "$PID_FILE") 2>/dev/null; then
        echo "✅ Tunnel started successfully (PID: $(cat $PID_FILE))"
        echo "   Log file: $LOG_FILE"
    else
        echo "❌ Failed to start tunnel. Check log: $LOG_FILE"
    fi
}

stop() {
    if [ -f "$PID_FILE" ]; then
        PID=$(cat "$PID_FILE")
        if kill -0 "$PID" 2>/dev/null; then
            echo "Stopping tunnel (PID: $PID)..."
            kill "$PID"
            rm -f "$PID_FILE"
            echo "✅ Tunnel stopped"
        else
            echo "Tunnel process not found"
            rm -f "$PID_FILE"
        fi
    else
        echo "No PID file found. Tunnel may not be running."
        # Try to find and kill by name
        pkill -f "cloudflared tunnel run $TUNNEL_NAME" && echo "✅ Killed tunnel process"
    fi
}

status() {
    if [ -f "$PID_FILE" ] && kill -0 $(cat "$PID_FILE") 2>/dev/null; then
        echo "✅ Tunnel running (PID: $(cat $PID_FILE))"
        # Show the tunnel URL
        cloudflared tunnel list 2>/dev/null | grep "$TUNNEL_NAME" || true
    else
        echo "❌ Tunnel not running"
    fi
}

case "$1" in
    start)
        start
        ;;
    stop)
        stop
        ;;
    status)
        status
        ;;
    restart)
        stop
        sleep 2
        start
        ;;
    *)
        echo "Usage: $0 {start|stop|status|restart}"
        exit 1
        ;;
esac
