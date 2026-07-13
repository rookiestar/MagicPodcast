# MagicPodcast 移动端访问说明

最后更新：2026-07-12

日常移动设备访问只能使用生产 HTTPS 域名，并通过 Cloudflare Access 的 Google 登录和独立 MFA。前端和后端不得为手机、平板或局域网设备开放直连端口。

> **安全切换状态（2026-07-12）：部分完成且当前默认拒绝。** Mac mini 已关闭局域网直连；由于 Cloudflare Access、HTTPS 跳转和 HSTS 尚未在控制台完成并验收，公网 Tunnel 已暂停，`rookiestar.cn` 暂不可用。在 Issue #2 完成前，不得恢复裸露公网入口，也不得用局域网 IP 作为替代入口。

## 日常访问

1. 在移动设备浏览器打开 `https://rookiestar.cn`。
2. 仅使用所有者指定的 Google 身份登录 Cloudflare Access，并完成已注册的安全密钥或设备生物识别验证；Google 登录不能替代 Access 的独立 MFA。
3. 不在移动设备上保存、设置或访问 `http://<局域网 IP>:3000`、`http://<局域网 IP>:8080`、`localhost` 或任何替代公网主机名。

## 本机开发与调试

在 Mac mini 本机上，开发服务可使用 `http://127.0.0.1:3000` 和 `http://127.0.0.1:8080`。需要检查移动端页面时，使用 Safari 的 USB 远程检查器连接已通过 HTTPS 域名打开的页面；不把开发服务暴露到 Wi-Fi。

## 常见问题

### 移动端显示 Network Error

确认地址是 HTTPS 生产域名，并确认 Cloudflare Access 已完成登录。不要把 `NEXT_PUBLIC_API_URL` 改成局域网 IP 来绕过问题；该做法会绕过统一登录边界。

### 服务无法访问

优先检查：

- Cloudflare Access 是否允许当前 Google 身份，以及该设备是否已注册安全密钥或设备生物识别验证。
- HTTPS 域名是否指向命名 Tunnel。
- Mac mini 上的前端、后端和 Tunnel 是否健康。
- 路由器是否仍然没有转发 `3000` 或 `8080`。

### 需要紧急恢复

只允许在 Mac mini 本机或使用临时 SSH 转发进行恢复。不得开启永久局域网入口、Quick Tunnel 或 Basic Auth。

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
