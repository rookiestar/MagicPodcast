# 阶段 1 测试指南

## 环境检查结果

### ✅ Node.js 环境
- Node.js: v24.12.0
- npm: 11.6.2
- 状态: **已安装，可以使用**

### ❌ Go 环境
- 状态: **未安装**
- 需要: 安装 Go 1.21+

---

## 安装 Go 环境

### 方法 1: 使用 Homebrew（推荐）

```bash
# 安装 Go
brew install go

# 验证安装
go version
```

### 方法 2: 官方安装包

访问 https://go.dev/dl/ 下载 macOS 安装包并安装。

### 验证安装

```bash
go version
# 应该显示: go version go1.21.x darwin/arm64
```

---

## 测试步骤

### 步骤 1: 初始化数据库

```bash
cd backend
go run scripts/init_db.go
```

**预期输出**:
```
🚀 MagicPodcast Database Initialization
======================================
✅ Config loaded from: ./configs/config.yaml
   Database: ./data/magicpodcast.db

📊 Running migrations...
🔄 Running database migrations...
   ✅ Migrated models.Tag
   ✅ Migrated models.Workflow
   ✅ Migrated models.Podcast
   ✅ Migrated models.Episode
   ✅ Migrated models.Job
   ✅ Migrated models.JobExecution
   ✅ Migrated models.Report
✅ All migrations completed successfully
🔄 Creating custom indexes...
✅ Custom indexes created successfully

🌱 Seeding initial data...
   ✅ Created tag: 科技
   ✅ Created tag: 教育
   ✅ Created tag: 娱乐
   ✅ Created tag: 商业
   ✅ Created tag: 健康

✅ Database initialization completed successfully!
   You can now start the API server with:
   go run cmd/api/main.go
```

**验证**:
```bash
ls -lh data/magicpodcast.db
# 应该看到数据库文件，大小约 8-16 KB
```

---

### 步骤 2: 启动后端服务

```bash
cd backend
go run cmd/api/main.go
```

**预期输出**:
```
🚀 MagicPodcast Backend starting...
📝 Loading config from: /Users/.../configs/config.yaml
✅ Config loaded successfully
   Server Mode: debug
   Server Port: 8080
   Database: ./data/magicpodcast.db
   XYZ API: http://localhost:8081

📊 Initializing database...
✅ Database connected: ./data/magicpodcast.db

🔧 Setting up routes...

🎉 Server started successfully!
   Listening on: http://localhost:8080
   Health check: http://localhost:8080/health
   API endpoint: http://localhost:8080/api/v1/podcasts
```

**验证（新开一个终端）**:
```bash
# 测试健康检查
curl http://localhost:8080/health

# 测试 ping
curl http://localhost:8080/ping

# 测试 API
curl http://localhost:8080/api/v1/podcasts
```

---

### 步骤 3: 安装前端依赖

```bash
cd frontend
npm install
```

**预期输出**:
```
added 200+ packages, and audited 300+ packages in 10s
...
found 0 vulnerabilities
```

---

### 步骤 4: 启动前端服务

```bash
cd frontend
npm run dev
```

**预期输出**:
```
   ▲ Next.js 14.2.0
   - Local:        http://localhost:3000

 ✓ Ready in 2.3s
```

---

### 步骤 5: 浏览器测试

1. **打开浏览器访问**: http://localhost:3000

2. **检查首页**:
   - 应该看到 "🎧 MagicPodcast" 标题
   - 3个功能卡片（我的订阅管理、本地标签与备注、自动化工作流）
   - 两个按钮："查看播客列表" 和 "API 健康检查"

3. **点击"查看播客列表"**:
   - 跳转到 http://localhost:3000/podcasts
   - 应该看到 3 个示例播客卡片
   - 标题："科技杂谈"、"商业洞察"、"健康生活"

4. **点击任意播客卡片**:
   - 跳转到详情页
   - 显示完整的播客信息

---

## 常见问题排查

### 问题 1: Go 命令找不到

**错误**: `go: command not found`

**解决**:
1. 安装 Go（见上方说明）
2. 重启终端
3. 验证: `go version`

### 问题 2: 数据库初始化失败

**错误**: `failed to open database: unable to open database file`

**解决**:
```bash
# 创建数据目录
mkdir -p backend/data

# 重新初始化
cd backend
go run scripts/init_db.go
```

### 问题 3: 端口被占用

**错误**: `bind: address already in use`

**解决**:
```bash
# 查找占用端口的进程
lsof -i :8080

# 杀死进程
kill -9 <PID>

# 或修改配置文件中的端口
vim backend/configs/config.yaml
# 将 port: 8080 改为其他端口
```

### 问题 4: npm install 失败

**错误**: 网络错误或依赖安装失败

**解决**:
```bash
# 清除缓存
cd frontend
rm -rf node_modules package-lock.json
npm cache clean --force

# 重新安装
npm install
```

### 问题 5: 前端无法连接后端

**现象**: 前端页面显示"加载失败"

**排查**:
1. 检查后端是否正常运行
2. 检查浏览器控制台错误（F12）
3. 验证 API 地址:
   ```bash
   curl http://localhost:8080/api/v1/podcasts
   ```
4. 检查环境变量:
   ```bash
   cat frontend/.env.local
   # 确认 NEXT_PUBLIC_API_URL=http://localhost:8080
   ```

---

## 测试检查清单

- [ ] Go 环境已安装（go version）
- [ ] 数据库初始化成功（data/magicpodcast.db 存在）
- [ ] 后端服务启动成功（http://localhost:8080 可访问）
- [ ] 前端依赖安装成功（node_modules 存在）
- [ ] 前端服务启动成功（http://localhost:3000 可访问）
- [ ] 首页显示正常
- [ ] 播客列表页显示 3 个示例
- [ ] 播客详情页显示正常
- [ ] 浏览器控制台无错误

---

## 测试成功标志

如果看到以下内容，说明测试成功：

✅ 后端终端显示:
```
🎉 Server started successfully!
   Listening on: http://localhost:8080
```

✅ 前端终端显示:
```
✓ Ready in 2.3s
```

✅ 浏览器显示:
- 首页正常
- 播客列表页显示 3 个卡片
- 点击卡片可跳转详情页

✅ API 测试成功:
```bash
$ curl http://localhost:8080/health
{"status":"ok","service":"magicpodcast-backend","database":"ok"}

$ curl http://localhost:8080/api/v1/podcasts
{"success":true,"data":[{...},{...},{...}]}
```

---

**测试时间**: 预计 15-20 分钟
**下一步**: 测试成功后进入阶段 2
