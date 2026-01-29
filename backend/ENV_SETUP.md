# 环境变量配置说明

本项目使用环境变量来管理敏感配置信息（如API Key、SMTP凭证等），避免将这些信息硬编码到配置文件中。

## 配置方式

### 方式一：使用 .env 文件（推荐开发环境）

1. **创建 .env 文件**

```bash
cd backend
cp .env.example .env
```

2. **编辑 .env 文件，填入实际值**

```bash
# LLM 配置
MAGICPODCAST_LLM_API_KEY=your_zhipuai_api_key_here

# 邮件通知配置（可选）
MAGICPODCAST_SMTP_HOST=smtp.gmail.com
MAGICPODCAST_SMTP_PORT=587
MAGICPODCAST_SMTP_USERNAME=your_email@gmail.com
MAGICPODCAST_SMTP_PASSWORD=your_app_password
```

3. **启动服务**

使用 dev.sh 脚本启动会自动加载 .env 文件：

```bash
./dev.sh start
```

或者手动加载环境变量：

```bash
cd backend
export $(cat .env | grep -v '^#' | xargs)
go run cmd/api/main.go
```

### 方式二：直接设置环境变量（推荐生产环境）

```bash
export MAGICPODCAST_LLM_API_KEY=your_api_key
go run cmd/api/main.go
```

或在 systemd/supervisor 配置中设置：

```ini
[Service]
Environment="MAGICPODCAST_LLM_API_KEY=your_api_key"
Environment="MAGICPODCAST_SMTP_PASSWORD=your_password"
```

### 方式三：使用 Docker 环境变量

在 `docker-compose.yml` 中配置：

```yaml
services:
  backend:
    environment:
      - MAGICPODCAST_LLM_API_KEY=${MAGICPODCAST_LLM_API_KEY}
      - MAGICPODCAST_SMTP_PASSWORD=${MAGICPODCAST_SMTP_PASSWORD}
```

## 支持的环境变量

### LLM 配置

| 环境变量 | 说明 | 示例 |
|---------|------|------|
| `MAGICPODCAST_LLM_API_KEY` | 智谱AI API Key | `206b0413f993c20b160d38e55755f7b4.EavtgA0dfIXB4Sn8` |

### 邮件通知配置（SMTP）

| 环境变量 | 说明 | 示例 |
|---------|------|------|
| `MAGICPODCAST_SMTP_HOST` | SMTP服务器地址 | `smtp.gmail.com` |
| `MAGICPODCAST_SMTP_PORT` | SMTP端口 | `587` |
| `MAGICPODCAST_SMTP_USERNAME` | 发件邮箱账号 | `user@gmail.com` |
| `MAGICPODCAST_SMTP_PASSWORD` | 邮箱授权码/密码 | `abcd1234efgh5678` |

### 小宇宙用户凭证

| 环境变量 | 说明 | 示例 |
|---------|------|------|
| `MAGICPODCAST_USER_PHONE` | 小宇宙手机号 | `13800138000` |
| `MAGICPODCAST_USER_ACCESS_TOKEN` | 访问令牌 | `your_access_token` |
| `MAGICPODCAST_USER_REFRESH_TOKEN` | 刷新令牌 | `your_refresh_token` |

**注意**：小宇宙凭证通常通过同步功能自动获取，不需要手动配置。

## 优先级

环境变量的优先级高于配置文件中的值。例如：

1. 如果设置了 `MAGICPODCAST_LLM_API_KEY` 环境变量，将使用环境变量的值
2. 否则，使用 `config.yaml` 中 `llm.api_key` 的值

## 安全建议

1. **永远不要提交 .env 文件到 git**
   - .gitignore 已配置忽略 .env 文件
   - 仅提交 .env.example 作为参考模板

2. **生产环境使用密钥管理服务**
   - AWS Secrets Manager
   - HashiCorp Vault
   - 云平台的密钥管理服务

3. **定期轮换 API Key**
   - 定期更换智谱AI API Key
   - 使用不同的 Key 用于开发/测试/生产环境

4. **限制 API Key 权限**
   - 仅授予必要的权限范围
   - 设置合理的额度限制

## 故障排查

### LLM 认证失败

**错误信息**：`LLM API错误: 令牌已过期或验证不正确 (code: 401)`

**排查步骤**：

1. 检查环境变量是否正确设置：
   ```bash
   cd backend
   cat .env | grep LLM_API_KEY
   ```

2. 检查服务是否读取到环境变量：
   ```bash
   # 查看后端日志中的 [ZhipuAI] 开头的调试信息
   tail -50 /tmp/backend.log | grep ZhipuAI
   ```

3. 验证 API Key 格式是否正确：
   - 智谱AI API Key 格式应为：`id.secret`
   - 示例：`206b0413f993c20b160d38e55755f7b4.EavtgA0dfIXB4Sn8`

4. 测试 API Key 是否有效：
   ```bash
   # 使用 curl 测试
   curl -X POST https://open.bigmodel.cn/api/paas/v4/chat/completions \
     -H "Authorization: Bearer YOUR_JWT_TOKEN" \
     -H "Content-Type: application/json" \
     -d '{"model":"glm-4.5-air","messages":[{"role":"user","content":"test"}]}'
   ```

### 邮件发送失败

**错误信息**：`authentication failed` 或 `connection refused`

**排查步骤**：

1. 确认 SMTP 配置正确：
   - Gmail 需要使用"应用专用密码"
   - QQ邮箱需要使用 SMTP 授权码
   - 端口通常为 587（STARTTLS）或 465（SSL）

2. 测试 SMTP 连接：
   ```bash
   telnet smtp.gmail.com 587
   ```

3. 检查防火墙和网络连接

## 参考链接

- [智谱AI开放平台](https://open.bigmodel.cn/)
- [Gmail SMTP 配置指南](https://support.google.com/mail/answer/7126229)
- [QQ邮箱 SMTP 设置](https://service.mail.qq.com/cgi-bin/help?subtype=1&id=28&no=1001256)
