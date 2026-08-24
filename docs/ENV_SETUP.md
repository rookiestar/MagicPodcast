# MagicPodcast 环境配置

最后更新：2026-08-24

本项目使用 `MAGICPODCAST_` 前缀的环境变量覆盖本机配置。真实 `.env`、`config.yaml`、数据库和日志不进入版本库。

## 后端配置

后端默认读取：

```text
backend/configs/config.yaml
```

示例文件：

```text
backend/configs/config.example.yaml
```

本地敏感配置可放在：

```text
backend/.env
```

启动脚本会在启动后端前加载 `backend/.env`：

```bash
./scripts/start.sh --prod
```

## 常用环境变量

| 环境变量 | 说明 |
| --- | --- |
| `MAGICPODCAST_SERVER_HOST` | 后端监听的数值 loopback 地址，生产使用 `127.0.0.1`；未设置时同样安全回落到该地址 |
| `MAGICPODCAST_SERVER_MODE` | 后端模式，`debug` 或 `release` |
| `MAGICPODCAST_SERVER_PORT` | 后端端口 |
| `MAGICPODCAST_DATABASE_DEBUG` | 是否打印数据库 SQL 调试日志 |
| `MAGICPODCAST_DATABASE_PATH` | 迁移或维护时指定目标 SQLite 文件路径 |
| `MAGICPODCAST_DATA_PROFILE` | 数据 Profile；正式服务必须为 `production` |
| `MAGICPODCAST_PRODUCTION_PROFILE_CONFIRM` | 仅规范生产启动脚本设置的内部启动门禁 |
| `MAGICPODCAST_DATABASE_BUSY_TIMEOUT_MS` | SQLite 写竞争等待时间，低于 100ms 时回落到 5000ms |
| `MAGICPODCAST_LLM_API_KEY` | LLM API Key |
| `MAGICPODCAST_LLM_PROVIDER` | LLM 提供商 |
| `MAGICPODCAST_LLM_BASE_URL` | LLM API 地址 |
| `MAGICPODCAST_LLM_DEFAULT_MODEL` | 默认模型 |
| `MAGICPODCAST_SMTP_HOST` | SMTP 服务器 |
| `MAGICPODCAST_SMTP_PORT` | SMTP 端口 |
| `MAGICPODCAST_SMTP_USERNAME` | SMTP 用户名 |
| `MAGICPODCAST_SMTP_PASSWORD` | SMTP 密码或授权码 |
| `MAGICPODCAST_USER_PHONE` | 小宇宙手机号 |
| `MAGICPODCAST_USER_ACCESS_TOKEN` | 小宇宙访问令牌 |
| `MAGICPODCAST_USER_REFRESH_TOKEN` | 小宇宙刷新令牌 |
| `MAGICPODCAST_PROCESSING_ENABLED` | 是否显式启动 Focus 加工 Worker；默认关闭 |
| `MAGICPODCAST_PROCESSING_PIPELINE_VERSION` | 当前加工管道版本 |
| `MAGICPODCAST_PROCESSING_AUDIO_ROOT` | 受管原音频绝对目录 |
| `MAGICPODCAST_PROCESSING_ARTIFACT_ROOT` | 逐字稿与纪要产物绝对目录 |
| `MAGICPODCAST_PROCESSING_LARK_CLI` | `lark-cli` 可执行文件路径 |
| `MAGICPODCAST_PROCESSING_LARK_WORK_ROOT` | 飞书 CLI 检查点文件绝对目录 |
| `MAGICPODCAST_PROCESSING_WORKER_SCAN_INTERVAL` | 持久队列扫描间隔 |
| `MAGICPODCAST_PROCESSING_EXTERNAL_POLL_INTERVAL` | 飞书妙记异步轮询间隔 |
| `MAGICPODCAST_PROCESSING_WORKER_BATCH_SIZE` | 单轮最多处理的受管音频/运行数 |
| `MAGICPODCAST_PROCESSING_RUNTIME_PYTHON` | 固定 Codex SDK 虚拟环境 Python |
| `MAGICPODCAST_PROCESSING_RUNTIME_HOST_SCRIPT` | `runtime_host.py` 绝对路径 |
| `MAGICPODCAST_PROCESSING_RUNTIME_WORK_ROOT` | Codex Runtime 受管工作绝对目录 |

生产模式下，`./scripts/start.sh --prod` 会默认设置：

```bash
MAGICPODCAST_SERVER_MODE=release
MAGICPODCAST_DATABASE_DEBUG=false
MAGICPODCAST_DATA_PROFILE=production
```

生产启动脚本同时设置内部启动门禁；如果已显式设置同名环境变量，脚本不会覆盖，值不正确时后端拒绝启动。该固定值只证明进程来自规范生产启动路径，不代表用户已授权部署或生产操作。

## Focus 加工启用前置

`processing.enabled` 缺省为 `false`。启用前必须同时满足：

1. 显式应用当前数据库迁移；普通 API 启动不会自动迁移。
2. 按 [Codex Runtime Host Runbook](runbooks/CODEX_RUNTIME_HOST.md) 安装固定 SDK/Runtime。
3. 安装 `lark-cli`，并以 user 身份完成云空间与妙记最小权限授权。
4. 为音频、飞书工作、Codex 工作和产物配置互相独立的绝对目录。

启用只会启动本机 Worker；不等于已授权迁移、部署或上传真实播客音频。飞书不可用时不会自动切换到本地 ASR。

## 前端配置

前端本地配置文件：

```text
frontend/.env.local
```

生产环境不得设置 `NEXT_PUBLIC_API_URL` 为局域网 IP、后端端口或任何替代公网主机名。Issue #2 的本机运行态已使用浏览器同域相对路径，服务端只使用 `BACKEND_URL=http://127.0.0.1:8080`。当前公网访问统一经过 Cloudflare Access、HTTPS 跳转和 HSTS；不得通过配置或重启 Tunnel 绕过该边界。详见 [../MOBILE.md](../MOBILE.md)。

## 手动启动

通常使用统一脚本：

```bash
./scripts/start.sh --prod
./scripts/start.sh --dev
```

需要手动启动后端时：

```bash
cd backend
export $(grep -v '^#' .env | xargs) 2>/dev/null || true
go run ./cmd/api
```

需要手动启动前端时：

```bash
cd frontend
npm run dev
```

## 隔离网络开发

不要手工改 `.env` 指向不同数据库。使用统一数据 Profile 命令：

```bash
./scripts/data-profile.sh status
./scripts/data-profile.sh use fixture
./scripts/data-profile.sh use snapshot latest
```

托管 Fixture/Snapshot 使用独立安全配置，显式跳过 `.env`、不继承生产凭据并禁用后台调度。完整说明见 [DATA_PROFILES.md](DATA_PROFILES.md)。

正式服务的 `release` 模式必须显式声明 `MAGICPODCAST_DATA_PROFILE=production` 并通过规范生产启动门禁；缺失时启动失败。该配置不能通过本地切换命令生成，也不替代部署前的独立用户授权。

## 安全建议

- 不提交 `.env`、`config.yaml`、数据库、日志和备份。
- 不在文档或代码中写真实 API Key、邮箱授权码或访问令牌。
- 生产和开发使用不同密钥。
- 轮换密钥后重启后端服务。
