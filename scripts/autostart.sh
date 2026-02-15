#!/bin/bash
# MagicPodcast 开机自启管理脚本
# 使用方法: ./scripts/autostart.sh enable|disable|status

PLIST_NAME="com.magicpodcast"
PLIST_PATH="$HOME/Library/LaunchAgents/${PLIST_NAME}.plist"

enable() {
    if [ ! -f "$PLIST_PATH" ]; then
        echo "❌ plist 文件不存在: $PLIST_PATH"
        exit 1
    fi

    echo "正在启用开机自启..."
    launchctl load "$PLIST_PATH" 2>/dev/null

    if launchctl list | grep -q "$PLIST_NAME"; then
        echo "✅ 开机自启已启用"
        echo "   下次登录时将自动启动 MagicPodcast"
    else
        echo "❌ 启用失败，请检查日志: /tmp/magicpodcast-launchd.log"
    fi
}

disable() {
    echo "正在禁用开机自启..."
    launchctl unload "$PLIST_PATH" 2>/dev/null

    if ! launchctl list | grep -q "$PLIST_NAME"; then
        echo "✅ 开机自启已禁用"
    else
        echo "❌ 禁用失败"
    fi
}

status() {
    if launchctl list | grep -q "$PLIST_NAME"; then
        echo "✅ 开机自启: 已启用"
        echo ""
        echo "服务状态:"
        launchctl list | grep "$PLIST_NAME"
    else
        echo "❌ 开机自启: 未启用"
    fi

    echo ""
    echo "相关文件:"
    echo "  plist: $PLIST_PATH"
    echo "  日志:  /tmp/magicpodcast-launchd.log"
    echo "  启动:  /tmp/magicpodcast-startup.log"
}

case "$1" in
    enable)
        enable
        ;;
    disable)
        disable
        ;;
    status)
        status
        ;;
    *)
        echo "MagicPodcast 开机自启管理"
        echo ""
        echo "用法: $0 {enable|disable|status}"
        echo ""
        echo "命令:"
        echo "  enable   - 启用开机自启"
        echo "  disable  - 禁用开机自启"
        echo "  status   - 查看状态"
        exit 1
        ;;
esac
