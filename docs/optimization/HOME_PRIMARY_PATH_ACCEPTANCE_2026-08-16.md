# 首页正式主路径验收记录

日期：2026-08-16

状态：桌面主路径通过；Cloudflare 仅完成 Access 门禁结构检查；手机与故障切换待人审

## 决策

上海私网中继是正式主路径，Cloudflare Access 是公共备用路径。公共路径的跨境绝对时延不再替代主路径验收，但仍保留安全和可用性门禁。

## 运行态

| 项目 | 观察结果 |
| --- | --- |
| 生产 release | `20260815T042300Z-02f0bb1-37483` |
| build mode | `release` |
| 桌面主路径 | `http://127.0.0.1:18089` 健康 |
| Cloudflare 备用 | 未认证 `/health` 返回 Access 302；登录页和认证后应用未验证 |
| 公共暴露 | 未发现绕过 Access 的 200 响应 |

## 桌面主路径冷载

使用 Playwright 启动 20 个全新浏览器上下文，经 `http://127.0.0.1:18089/` 访问同一生产源站。

| 指标 | 结果 | 门槛 |
| --- | ---: | ---: |
| 真实内容成功 | 20 / 20；每次 5 条 | 100% |
| 有效内容 P50 | 0.570s | 记录 |
| 有效内容 P95 | 0.778s | `< 5s` |
| 有效内容最大值 | 0.843s | `< 10s` |
| TTFB P95 | 0.532s | 记录 |
| 控制台错误轮次 | 0 / 20 | 0 |

桌面主路径硬门槛通过。

## 用户可见验收

有头浏览器确认：

- 首页显示真实 Discovery、精选报告和最近更新内容。
- 最近更新从第 1 页切到第 2 页成功。
- 页面无控制台错误。

## 主备状态检查

```bash
./scripts/access-path-status.sh
```

观察：

```text
primary_status=healthy
primary_http_code=200
primary_release_id=20260815T042300Z-02f0bb1-37483
primary_build_mode=release
fallback_status=access_gate_reachable
fallback_http_code=302
fallback_access_gate=present
fallback_login_page=not_checked
fallback_authenticated_app=not_checked
```

## 未完成

- 手机开启 WireGuard 后的同域名冷载与代表性交互。
- 手机关闭 WireGuard 后的 Cloudflare 认证回退。
- 主路径故障时的真实切换演练；本轮未主动中断现有中继。
- 观察窗和证书续期的长期稳定性。

在以上人审项完成前，不关闭父专项或生产验收票。
