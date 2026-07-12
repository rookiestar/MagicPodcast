# MagicPodcast 部署和运维

最后更新：2026-05-31

本文记录当前可用的本地运行、生产模式启动、健康检查和备份恢复入口。旧 Docker 配置已移入根目录 `archive/`，只作历史参考。

## 快速启动

生产模式：

```bash
./scripts/start.sh --prod
```

开发模式：

```bash
./scripts/start.sh --dev
```

重启：

```bash
./scripts/restart.sh --prod
./scripts/restart.sh --dev
```

生产重启会先进入可回退发布流程：新版本先写入 `.magicpodcast-releases/<release-id>/` 并完成后端编译、前端生产构建和配对校验；只有验证通过后才停止当前服务。运行版本可从本机 `/health` 的 `release_id`、`frontend_build_id` 和 `build_mode` 字段确认。切换失败时脚本会自动恢复上一版本；也可以用单一步骤手动回退：

```bash
./scripts/release.sh --rollback
./scripts/release.sh --dry-run
```

发布日志只记录版本、构建、验证、切换和回退状态，不记录环境变量、令牌或配置内容，默认写入 `logs/release.log`。构建失败或最低验证失败会在停止当前服务前退出。启停脚本会核对工作目录、进程命令、PID 文件和 loopback 监听，拒绝接管未知进程。

停止：

```bash
./scripts/stop.sh
```

项目根目录也保留了软链接，可使用：

```bash
./start.sh --prod
./restart.sh --prod
./stop.sh
```

## 服务地址

| 服务 | 地址 |
| --- | --- |
| 前端 | `http://localhost:3000` |
| 后端 | `http://localhost:8080` |
| 后端健康检查 | `http://localhost:8080/health` |

## 健康检查

```bash
./scripts/health-check.sh
```

或使用根目录软链接：

```bash
./health.sh
```

健康检查会覆盖：

- 前端和后端端口。
- 后端 `/health`。
- 生产/开发模式状态。
- 数据库文件和备份状态。
- 常用脚本入口。

## 生产模式说明

`./scripts/start.sh --prod` 会：

1. 编译后端为 `backend/api`。
2. 默认设置后端为 `release` 模式。
3. 默认关闭数据库 SQL 调试日志。
4. 构建 Next.js 生产版本。
5. 使用后台会话启动前后端，并记录监听端口对应的进程。
6. 等待端口监听并执行健康检查。

发布切换完成后使用 `./scripts/start.sh --prod --no-build` 复用已验证的前后端产物；该参数在缺少 `backend/api` 或 `frontend/.next/BUILD_ID` 时直接失败，不会现场构建。

日志位置：

```text
/tmp/magicpodcast-backend.log
/tmp/magicpodcast-frontend.log
```

PID 文件位置：

```text
/tmp/magicpodcast-backend.pid
/tmp/magicpodcast-frontend.pid
```

## 环境配置

后端配置：

```text
backend/configs/config.yaml
backend/.env
```

前端配置：

```text
frontend/.env.local
```

详细说明见 [ENV_SETUP.md](ENV_SETUP.md)。

## Cloudflare Tunnel

> **安全切换状态（2026-07-12）：部分完成且当前默认拒绝。** Mac mini 的 Nginx、前端和后端已收紧为回环监听，Tunnel 配置已改为仅连接 `127.0.0.1:8088`，历史 `scripts/cloudflare-tunnel.sh` 已拒绝 Quick Tunnel 和 Basic Auth。由于 Cloudflare Access、HTTPS 跳转和 HSTS 尚未在控制台完成并通过运行态验收，命名 Tunnel 已停止且其 macOS 启动项已禁用，`rookiestar.cn` 应返回 Cloudflare 530，而不是继续裸露应用。本机访问仅限 Mac mini 本机或临时 SSH 转发；在完成 [Issue #2](https://github.com/rookiestar/MagicPodcast/issues/2) 的控制台步骤前，不得重新启用 Tunnel。

### 公网访问安全切换运行手册（Issue #2）

此手册不包含 Google 地址、Cloudflare 凭据、恢复代码或 Tunnel 凭据。这些资料只能保存在所有者受控的密码管理器或本机受限目录，不能写入仓库、Issue 或日志。

#### 变更前条件

1. 取得所有者对 Cloudflare 策略、DNS、路由器检查、生产构建和服务重启的明确授权。
2. 在 Cloudflare 控制台中记录当前 DNS、Access、HTTPS 和 Tunnel 设置，存放在受控位置，作为回退依据。
3. 确认允许登录的唯一 Google 身份已启用 MFA 或 Passkey，并确认 Cloudflare 与 Google 的恢复材料可用。
4. 在 Mac mini 上确认服务进程归属后再停止或重启；不得仅按端口终止进程。确认路由器没有把 `3000` 或 `8080` 转发到公网。

#### 仓库与本机变更

1. 后端配置增加监听地址，生产值固定为 `127.0.0.1`；缺省值也安全回落到 `127.0.0.1`，并拒绝通配地址及非 loopback 地址。补充配置加载和校验测试。
2. 前端开发与生产启动命令显式绑定 `127.0.0.1`。前端浏览器请求始终使用当前 HTTPS 域名的相对路径；服务端转发只使用 `BACKEND_URL=http://127.0.0.1:8080`，不再让 `NEXT_PUBLIC_API_URL` 指向局域网或后端地址。
3. `scripts/cloudflare-tunnel.sh` 已拒绝 Quick Tunnel 和 Basic Auth；获得删除授权后移除该历史入口。文档不再推荐 Quick Tunnel、默认 Basic Auth 或局域网直连；移动设备日常访问统一为 HTTPS 域名。
4. 生产本机配置不提交。Cloudflare 目录设为仅所有者可读，Tunnel 配置和凭据文件权限为 `600`。

#### Cloudflare 精确设置

1. 在 Mac mini 以仅所有者可读的环境创建一个命名 Tunnel：`magicpodcast-prod`。记录其 UUID，但不把 UUID 凭据文件或登录证书提交到仓库。
2. 将 `rookiestar.cn` 的 Tunnel 公共主机名指向 `http://127.0.0.1:8088`。这是 Nginx 的仅本机入口；Nginx 再转发页面与框架资源到 `127.0.0.1:3000`，并转发 API、图片和 `/health` 到 `127.0.0.1:8080`。不得为前端或后端另开公网主机名或直连端口。Tunnel 配置只有一个允许的主机名，其他主机名返回 `404`。
3. 在 Cloudflare DNS 中让 `rookiestar.cn` 指向该 Tunnel 的 `*.cfargotunnel.com` 目标。若现有记录冲突，只能在已保存原记录且获得授权后替换。
4. 创建一个 Self-hosted Access 应用，应用域名为 `rookiestar.cn`，不设置路径例外。策略默认拒绝，仅有一个 `Allow` 策略：精确匹配所有者的 Google 身份，并只允许 Google 登录方式。应用会话时长设为 30 天；不创建服务令牌、旁路策略或公开路径。
5. 在 Cloudflare 的 HTTPS 设置中启用 HTTP 到 HTTPS 的永久跳转；启用 HSTS，`max-age=31536000`。在完成全部子域盘点前，不启用 `includeSubDomains` 或预加载。
6. 实施阶段使用 `cloudflared tunnel run magicpodcast-prod` 启动命名 Tunnel。由 Issue #9 把该命令纳入可恢复的 macOS 守护配置；不得改回 Quick Tunnel。

#### 回退与紧急恢复

1. Access 规则配错或 Tunnel 故障时，只能在 Mac mini 本机或通过临时 SSH 转发进入应用并修正配置。不得为了恢复便利而关闭 Access、启用 Basic Auth 或开放局域网端口。
2. 先恢复已保存的 Access/DNS/Tunnel 设置中的最小一项，并在无登录浏览器中复测 Access；不自动恢复到当前的裸露公网状态。
3. 如确实需要把 DNS 恢复为切换前的公网入口，必须另行取得所有者明确批准，因为这会重新暴露服务。
4. 所有回退后都重新检查 HTTP 跳转、Access 全路径拦截、Tunnel 连通和 loopback 监听。

#### 默认拒绝后的受控恢复

当前 macOS 启动项 `com.cloudflare.cloudflared` 已被停用，避免 Mac mini 重启后重新暴露未受 Access 保护的域名。只有在 Access、HTTPS 跳转和 HSTS 已由控制台完成、并且准备立即做下方的公网验收时，才可在 Mac mini 上执行：

```bash
launchctl enable "gui/$(id -u)/com.cloudflare.cloudflared"
launchctl bootstrap "gui/$(id -u)" "$HOME/Library/LaunchAgents/com.cloudflare.cloudflared.plist"
launchctl kickstart -k "gui/$(id -u)/com.cloudflare.cloudflared"
```

任一验收失败，立刻执行 `launchctl bootout "gui/$(id -u)/com.cloudflare.cloudflared"`，再执行 `launchctl disable "gui/$(id -u)/com.cloudflare.cloudflared"`；不得以裸露公网入口作为回退。

#### 只读验收

1. 使用未登录的隐私浏览器或不带 Cookie 的请求访问首页、API、图片、框架静态资源和 `/health`；每一项都必须进入 Access 登录，而不是返回应用内容。
2. 用非允许 Google 身份验证被拒绝；用允许身份并完成 MFA 或 Passkey 后，确认页面和只读 API 可用。
3. 在 Access 控制台撤销该用户现有会话，随后用原浏览器再次请求，必须重新登录或被拒绝。
4. 验证 HTTP 返回永久跳转到 HTTPS，HTTPS 响应包含 `Strict-Transport-Security: max-age=31536000`。
5. 在 Mac mini 上确认 `8088`、`3000` 与 `8080` 均仅监听 `127.0.0.1`；再从另一台局域网设备确认无法直连这三个端口。检查路由器没有对应端口转发。
6. 确认 `cloudflared tunnel info magicpodcast-prod` 显示已连接，仓库与部署文档不再出现 Quick Tunnel、默认 Basic Auth 或局域网绕过入口。

## 开机自启

当前保留 `scripts/production-startup.sh`，用于本机登录后启动生产服务。该脚本只负责健康判断，并委托当前标准入口 `scripts/start.sh --prod` 或 `scripts/restart.sh --prod` 执行真实启动，避免单独维护第二套启动流程。

可用环境变量：

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `MAGICPODCAST_PROJECT_DIR` | 脚本所在项目根目录 | 项目路径 |
| `MAGICPODCAST_STARTUP_LOG` | `/tmp/magicpodcast-production.log` | 开机启动日志 |
| `MAGICPODCAST_STARTUP_DELAY` | `3` | 等待网络和系统服务就绪的秒数 |

配置 launchd 前应先确认本机路径、端口和环境变量都正确。

## 备份和恢复

创建备份：

```bash
./scripts/backup-db.sh
```

安装每日备份任务：

```bash
./scripts/install-backup-agent.sh
```

验证数据库：

```bash
./scripts/verify-db.sh backend/data/magicpodcast.db
```

恢复备份：

```bash
./scripts/restore-db.sh backend/data/backups/magicpodcast_YYYYMMDD_HHMMSS.db.gz
```

恢复会要求服务停止，除非显式使用 `--force`。

## 性能和发布前检查

```bash
(cd backend && go test ./...)
(cd backend && go vet ./...)
(cd frontend && npm run type-check)
(cd frontend && npm run lint)
(cd frontend && npm run test:run)
(cd frontend && npm run build)
```

页面和 API 巡检：

```bash
node scripts/performance-audit.mjs \
  --base-url http://localhost:3000 \
  --api-url http://localhost:8080 \
  --runs 3
```

详细性能检查见 [PERFORMANCE_TESTING_GUIDE.md](PERFORMANCE_TESTING_GUIDE.md)。

## 清理

清理当前可用命令见 [CLEAN_GUIDE.md](CLEAN_GUIDE.md)。遇到前端缓存异常时优先使用：

```bash
./scripts/restart.sh --clean --prod
```

## 历史 Docker 配置

根目录 `archive/` 下保留旧 Dockerfile、docker-compose 和 Nginx 配置。当前默认运维入口是脚本方式；是否继续维护 Docker 部署需要人工确认。
