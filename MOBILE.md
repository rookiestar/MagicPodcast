# MagicPodcast 移动端访问说明

最后更新：2026-05-31

移动设备访问本地开发服务时，不能使用 `localhost`，需要使用 Mac 的局域网 IP。

## 快速设置

1. 确认 iPhone/iPad 和 Mac 在同一个 Wi-Fi。
2. 在 Mac 上查看局域网 IP：

   ```bash
   ifconfig | grep "inet " | grep -v 127.0.0.1
   ```

3. 配置前端 API 地址：

   ```bash
   echo "NEXT_PUBLIC_API_URL=http://你的局域网IP:8080" > frontend/.env.local
   ```

4. 启动服务：

   ```bash
   ./scripts/start.sh --dev
   ```

5. 在移动设备浏览器访问：

   ```text
   http://你的局域网IP:3000
   ```

## 手动检查

```bash
# 后端健康检查
curl "http://你的局域网IP:8080/health"

# 前端可访问性检查
curl -I "http://你的局域网IP:3000"

# 端口监听检查
lsof -i :8080
lsof -i :3000
```

## 常见问题

### 移动端显示 Network Error

通常是 `frontend/.env.local` 仍然写着 `localhost:8080`。移动设备上的 `localhost` 指的是手机自己，不是 Mac。改成 Mac 的局域网 IP 后重启前端服务。

### 服务无法访问

优先检查：

- Mac 和移动设备是否在同一 Wi-Fi。
- 端口 `3000` 和 `8080` 是否正在监听。
- macOS 防火墙是否拦截了 Node 或后端服务。
- VPN 是否改变了网络路由。

### IP 变化

如果路由器重新分配了地址，需要重新查看 IP 并更新 `frontend/.env.local`。长期使用可以在路由器里给 Mac 绑定固定局域网地址。

## Safari 调试

1. iPhone：设置 → Safari → 高级 → 打开 Web 检查器。
2. Mac Safari：设置 → 高级 → 打开“在菜单栏中显示开发菜单”。
3. 用 USB 连接 iPhone 和 Mac。
4. Mac Safari：开发 → 选择设备和当前页面，查看 Console。

## 移动端检查清单

- 播客列表单列显示，无横向滚动。
- 卡片、按钮和输入框触摸区域足够大。
- 标签筛选、搜索、同步和工作流页面可正常使用。
- 弹窗可打开、提交和关闭。
- 图片加载正常，页面滚动不卡顿。
