# MagicPodcast 正式访问路径

最后更新：2026-08-16

## 当前策略

上海私网中继是所有者的正式主路径；Cloudflare Access 是公共备用路径。

| 设备 | 主路径 | 备用路径 |
| --- | --- | --- |
| 当前电脑 | `http://127.0.0.1:18089` | `https://rookiestar.cn` |
| 手机 | 开启 MagicPodcast WireGuard 后访问 `https://rookiestar.cn` | 关闭 WireGuard 后访问同一 URL |

这是访问策略调整，不迁移源站、不修改公共 DNS，也不关闭 Cloudflare。Mac mini 仍是唯一生产源站。

## 不变量

- 主路径只允许 loopback 或 WireGuard 私网访问，不新增公网 Web 端口。
- 手机只路由 `10.89.0.1/32`，不得改成全局 VPN。
- Cloudflare 备用路径必须继续由 Access 拦截；未认证请求不得直接返回应用内容。
- 主路径故障时手动切换，不做无提示的自动公网降级。
- 两条路径使用同一生产 release 和数据库，不建立第二套应用状态。

## 验收标准

### 主路径

- 冷载至少 20 次，真实内容成功率 100%。
- 有效内容时间 P95 `< 5s`，最大 `< 10s`。
- 桌面和手机均验证首页、完整列表刷新与代表性交互。
- `/health`、`release_id`、`frontend_build_id` 与生产运行态一致。

### Cloudflare 备用路径

- 未认证访问必须进入 Cloudflare Access。
- 已认证页面和 API 可用，失败时能明确识别。
- 继续记录 P50/P95/最大值，但公共跨境绝对时延不作为主路径关闭门槛。

不得把两条路径的样本混在一起计算百分位。

## 日常检查

当前电脑：

```bash
./scripts/access-path-status.sh
```

成功输出必须同时包含：

```text
primary_status=healthy
fallback_status=access_gate_reachable
fallback_access_gate=present
fallback_login_page=not_checked
fallback_authenticated_app=not_checked
```

该命令只验证未认证请求确实被 Cloudflare Access 拦截；`access_gate_reachable` 不等于备用路径已可用。登录页和认证后应用必须在真实手机浏览器中单独验收，并分别记录结果。

手机主路径：

1. 开启 MagicPodcast WireGuard。
2. 访问 `https://rookiestar.cn/health`。
3. 确认返回生产 `release_id`，且没有出现 Cloudflare 登录页。

手机备用路径：

1. 关闭 WireGuard。
2. 重新访问 `https://rookiestar.cn`。
3. 确认进入 Cloudflare Access，认证后应用可用。

## 故障切换

### 当前电脑

本地中继不可用时，直接打开 `https://rookiestar.cn`。不要停止生产服务、修改公共 DNS 或关闭 Access。

### 手机

私网超时、502 或证书异常时，关闭 MagicPodcast WireGuard，再重新打开同一 URL。

证书异常不得点击绕过。Cloudflare 备用路径也异常时，先检查 Mac mini `/health`，再分别检查反向隧道和 Cloudflare Tunnel。

## 回退

如决定取消“中继为主”的策略，只需恢复日常使用 Cloudflare URL；不要直接删除 WireGuard、证书、SSH 密钥、LaunchAgent 或阿里云配置。

完整拓扑、服务路径和停用步骤见[上海中继专项文档](../optimization/PODCASTS_PERFORMANCE_AND_SHANGHAI_RELAY_2026-08-09.md)。
