# MagicPodcast 环境配置

最后更新：2026-05-31

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

生产模式下，`./scripts/start.sh --prod` 会默认设置：

```bash
MAGICPODCAST_SERVER_MODE=release
MAGICPODCAST_DATABASE_DEBUG=false
```

如果已显式设置同名环境变量，脚本不会覆盖。

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

## 安全建议

- 不提交 `.env`、`config.yaml`、数据库、日志和备份。
- 不在文档或代码中写真实 API Key、邮箱授权码或访问令牌。
- 生产和开发使用不同密钥。
- 轮换密钥后重启后端服务。
