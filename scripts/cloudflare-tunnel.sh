#!/bin/bash

# MagicPodcast Cloudflare Tunnel管理脚本
# 用途：快速启动、停止、查看状态

set -e

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

# 检查cloudflared是否安装
check_cloudflared() {
    if ! command -v cloudflared &> /dev/null; then
        print_error "cloudflared未安装"
        print_info "请运行: brew install cloudflared"
        exit 1
    fi
}

# 检查Nginx代理是否运行
check_nginx() {
    if ! curl -f -s http://localhost:8088/health > /dev/null 2>&1; then
        print_error "Nginx代理未运行或无法访问"
        print_info "请先启动Docker服务："
        echo "  docker-compose -f docker-compose.cloudflare.yml up -d"
        exit 1
    fi
}

# 启动临时隧道
start_temp_tunnel() {
    print_header "🚀 启动Cloudflare临时隧道"

    check_nginx

    print_info "隧道信息："
    echo "  - 目标地址: http://localhost:8088"
    echo "  - 协议: HTTP"
    echo "  - 类型: 临时隧道（重启后URL会变）"
    echo ""
    print_info "正在创建隧道..."
    echo ""

    # 启动隧道（保持在前台）
    cloudflared tunnel --url http://localhost:8088
}

# 启动带Basic Auth的临时隧道
start_tunnel_with_auth() {
    print_header "🔐 启动带Basic Auth的Cloudflare临时隧道"

    check_nginx

    # 配置Basic Auth
    local USERNAME="${CLOUDFLARE_USERNAME:-admin}"
    local PASSWORD="${CLOUDFLARE_PASSWORD:-magicpodcast}"

    print_info "认证信息："
    echo "  用户名: ${USERNAME}"
    echo "  密码: ${PASSWORD}"
    echo ""
    print_info "正在创建隧道..."
    echo ""

    # 启动带认证的隧道
    cloudflared tunnel --url http://localhost:8088 \
        --basic-auth="${USERNAME}:${PASSWORD}"
}

# 显示使用帮助
show_help() {
    print_header "MagicPodcast Cloudflare Tunnel管理脚本"
    echo ""
    echo "用法:"
    echo "  ./scripts/cloudflare-tunnel.sh [命令]"
    echo ""
    echo "命令:"
    echo "  start       启动临时隧道（推荐）"
    echo "  auth        启动带Basic Auth的临时隧道"
    echo "  help        显示此帮助信息"
    echo ""
    echo "环境变量:"
    echo "  CLOUDFLARE_USERNAME    Basic Auth用户名（默认: admin）"
    echo "  CLOUDFLARE_PASSWORD    Basic Auth密码（默认: magicpodcast）"
    echo ""
    echo "示例:"
    echo "  ./scripts/cloudflare-tunnel.sh start"
    echo "  CLOUDFLARE_PASSWORD=mypassword ./scripts/cloudflare-tunnel.sh auth"
    echo ""
}

# 主逻辑
main() {
    case "${1:-}" in
        start)
            check_cloudflared
            start_temp_tunnel
            ;;
        auth)
            check_cloudflared
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
