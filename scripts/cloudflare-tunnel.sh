#!/bin/bash

# MagicPodcast Cloudflare Tunnel 管理脚本（历史兼容入口，已禁用）
# 生产只能使用 docs/DEPLOYMENT.md 中的命名 Tunnel 和 Cloudflare Access。

set -euo pipefail

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 打印带颜色的消息
print_success() {
    echo -e "${GREEN}✓ $1${NC}"
}

print_error() {
    echo -e "${RED}✗ $1${NC}"
}

print_info() {
    echo -e "${YELLOW}ℹ $1${NC}"
}

print_header() {
    echo -e "${BLUE}========================================${NC}"
    echo -e "${BLUE}$1${NC}"
    echo -e "${BLUE}========================================${NC}"
}

# 拒绝旧的临时 Tunnel 和 Basic Auth 路径。
deny_unsafe_tunnel() {
    print_error "已拒绝执行：Quick Tunnel 和 Basic Auth 不允许用于 MagicPodcast。"
    print_info "请按 docs/DEPLOYMENT.md 中的命名 Tunnel 运行手册操作。"
    exit 1
}

# 启动临时隧道（已禁用）
start_temp_tunnel() {
    deny_unsafe_tunnel
}

# 启动带 Basic Auth 的临时隧道（已禁用）
start_tunnel_with_auth() {
    deny_unsafe_tunnel
}

# 显示使用帮助
show_help() {
    print_header "MagicPodcast Cloudflare Tunnel 管理脚本"
    echo ""
    echo "用法:"
    echo "  ./scripts/cloudflare-tunnel.sh [命令]"
    echo ""
    echo "命令:"
    echo "  start       已禁用"
    echo "  auth        已禁用"
    echo "  help        显示此帮助信息"
    echo ""
    echo "生产请使用 docs/DEPLOYMENT.md 中的命名 Tunnel 运行手册。"
    echo ""
}

# 主逻辑
main() {
    case "${1:-}" in
        start)
            start_temp_tunnel
            ;;
        auth)
            start_tunnel_with_auth
            ;;
        help|--help|-h)
            show_help
            ;;
        *)
            print_error "未知命令: ${1:-}"
            echo ""
            show_help
            exit 1
            ;;
    esac
}

main "$@"
