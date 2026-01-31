# 部署和运维指南

本文档提供 MagicPodcast 项目的完整部署和运维指南，包括本地开发环境配置、生产环境部署、配置管理、数据库迁移和最佳实践。

## 目录

- [本地开发](#1-本地开发)
- [生产部署](#2-生产部署)
- [配置管理](#3-配置管理)
- [定时任务配置](#4-定时任务配置)
- [数据库迁移](#5-数据库迁移)
- [运维脚本](#6-运维脚本)
- [部署最佳实践](#7-部署最佳实践)

---

## 1. 本地开发

### 快速启动

**方式一：使用管理脚本（推荐）**

```bash
./dev.sh start          # 启动所有服务
./dev.sh stop           # 停止所有服务
./dev.sh restart        # 重启所有服务
./dev.sh status         # 查看服务状态
./dev.sh logs           # 查看日志
./dev.sh clean          # 清理缓存
```

**方式二：分别启动**

```bash
# 启动后端
cd backend
go run cmd/api/main.go

# 启动前端（新终端）
cd frontend
npm run dev
```

### 环境配置

1. **后端配置**：`backend/configs/config.yaml`
   - 服务器端口和模式
   - 数据库路径
   - 小宇宙API配置
   - 同步和调度设置

2. **环境变量**：支持 `.env` 文件
   - 通过 `MAGICPODCAST_` 前缀覆盖配置
   - 详见 [环境变量配置说明](ENV_SETUP.md)

3. **小宇宙Cookie**：
   - 需要在配置中设置 `xyz_api.cookie`
   - 或通过环境变量 `MAGICPODCAST_USER_***` 设置

### 健康检查

运行健康检查脚本：

```bash
./health-check.sh
```

检查项包括：
- ✅ 环境检查（Go、Node.js、npm、Git）
- ✅ 项目结构检查
- ✅ 依赖检查
- ✅ 数据库检查
- ✅ 服务运行状态检查

---

## 2. 生产部署

### Docker容器化部署

#### Docker镜像配置

**1. 后端镜像**（`Dockerfile.backend`）

- 多阶段构建：golang:1.21-alpine（构建）+ alpine:latest（运行）
- 暴露端口：8080
- 包含：SQLite、ca-certificates
- 优化：镜像体积小、安全性高

**2. 前端镜像**（`Dockerfile.frontend`）

- 多阶段构建：node:18-alpine（构建）+ node:18-alpine（运行）
- 暴露端口：3000
- 包含：.next构建产物、静态资源

**3. Docker Compose编排**（`docker-compose.yml`）

- 三个服务：xyz-api（小宇宙API代理）、backend、frontend
- 网络隔离：magicpodcast-network
- 数据卷挂载：./data目录持久化数据库
- 健康检查：xyz-api服务配置了健康检查
- 服务依赖：frontend → backend → xyz-api

#### 部署命令

```bash
# 构建并启动所有服务
docker-compose up -d

# 查看服务状态
docker-compose ps

# 查看日志
docker-compose logs -f

# 停止服务
docker-compose down

# 重启服务
docker-compose restart
```

#### 服务访问

- 前端：http://localhost:3000
- 后端API：http://localhost:8080
- 小宇宙API代理：http://localhost:8081

---

## 3. 配置管理

### 配置文件结构

- **主配置**：`backend/configs/config.yaml`
- **示例配置**：`backend/configs/config.example.yaml`
- **环境变量**：`.env` 文件（开发环境）
- **配置加载**：使用Viper库，支持环境变量覆盖

### 核心配置项

```yaml
server:
  port: 8080
  mode: release          # debug/release
  read_timeout: 600      # 支持长时间SSE
  write_timeout: 600
  cors: 完整的跨域配置

database:
  path: ./data/magicpodcast.db
  debug: false
  连接池配置

xyz_api:
  url: http://localhost:8081
  timeout: 30
  重试机制

sync:
  enabled: true
  schedule: "0 2 * * *"  # 默认每天凌晨2点
  concurrency: 5         # 并发数
  request_interval: 2000  # 请求间隔（毫秒）

logging:
  level: info
  format: json           # text/json
  文件轮转配置

# 可选功能配置
email:                   # 邮件通知
llm:                     # LLM智能摘要
search:                  # 搜索权重配置
user:                    # 用户认证信息
```

### 配置管理特性

- ✅ **环境变量支持**：通过 `MAGICPODCAST_` 前缀覆盖敏感配置
- ✅ **配置验证**：内置配置合法性检查
- ✅ **敏感信息保护**：API Key、密码等通过环境变量设置
- ✅ **多环境支持**：开发/生产配置分离

详细的环境变量配置请参考 [环境变量配置说明](ENV_SETUP.md)。

---

## 4. 定时任务配置

### 调度器特性

- 基于 robfig/cron v3（支持6位Cron表达式，秒级精度）
- 自动兼容5位和6位表达式
- 本地时区支持
- 任务执行状态跟踪
- 错过任务补偿执行
- 连续失败告警（3次阈值）
- 热重载支持（运行时重新加载配置）

### 调度功能

- **工作流调度**：基于Cron表达式的定时任务
- **并发控制**：避免重复执行
- **失败处理**：连续失败告警
- **状态持久化**：下次执行时间记录

### 调度器管理API

```bash
# 重载调度器配置
POST /api/v1/scheduler/reload

# 获取调度器状态
GET /api/v1/scheduler/status

# 暂停工作流
POST /api/v1/scheduler/workflows/:id/pause

# 恢复工作流
POST /api/v1/scheduler/workflows/:id/resume
```

### Cron表达式格式

```
# 秒级精度（6位）
* * * * * *
│ │ │ │ │ │
│ │ │ │ │ └─ 星期几 (0-6, 0=周日)
│ │ │ │ └─── 月份 (1-12)
│ │ │ └───── 日期 (1-31)
│ │ └─────── 小时 (0-23)
│ └───────── 分钟 (0-59)
└─────────── 秒 (0-59)

# 示例
"0 0 2 * * *"      # 每天凌晨2点
"0 */30 * * * *"   # 每30分钟
"0 0 9-18 * * 1-5" # 工作日9-18点每小时
```

---

## 5. 数据库迁移

### 迁移方式

#### 1. GORM自动迁移（推荐）

- **位置**：`backend/internal/database/migrate.go`
- **启动时**：自动执行
- **特点**：
  - 按依赖关系排序迁移（避免外键约束问题）
  - 安全创建索引（IF NOT EXISTS）
  - 无需手动干预

#### 2. 手动迁移工具

```bash
# 执行手动迁移
go run cmd/migrate/main.go
```

- **功能**：处理特定的schema变更
- **数据迁移**：完整的表重建和数据复制
- **回滚机制**：失败时可以回滚

### 数据库模型

**10个核心表**：

- Podcast（播客节目，50+字段）
- Episode（播客单集）
- Tag（标签）
- Workflow（工作流）
- Job（任务）
- JobExecution（任务执行详情）
- Report（工作流执行报告）
- SchedulerRun（调度器运行记录）
- SyncConfig（同步配置）

**关联表**：

- podcasts_tags（播客-标签多对多）
- episodes_tags（单集-标签多对多）

**特性**：

- ✅ 软删除支持（BaseModel）
- ✅ 时间戳自动管理（created_at, updated_at）
- ✅ 外键关联
- ✅ 自定义索引优化

---

## 6. 运维脚本

### 生产环境脚本

| 脚本 | 功能 | 说明 |
|------|------|------|
| `start.sh` | 启动服务 | 日志文件管理、进程PID管理 |
| `stop.sh` | 停止服务 | 优雅关闭 |
| `health-check.sh` | 健康检查 | 环境、项目结构、依赖、数据库、服务状态 |
| `dev.sh` | 开发环境管理 | 启动、停止、重启、状态、日志、清理缓存 |

### 使用示例

```bash
# 启动生产服务
./start.sh

# 停止服务
./stop.sh

# 检查服务健康状态
./health-check.sh

# 开发环境管理
./dev.sh start
./dev.sh status
./dev.sh logs
./dev.sh stop
```

### 日志管理

- **日志框架**：logrus
- **日志级别**：debug、info、warn、error
- **文件轮转**：按大小和时间轮转
- **日志格式**：支持text和json格式
- **远程日志**：方便未来对接远程日志服务

---

## 7. 部署最佳实践

### 容器编排建议

- 使用Docker Compose进行本地开发和测试
- 生产环境可使用Kubernetes进行编排
- 服务健康检查：配置liveness和readiness探针
- 资源限制：设置CPU和内存限制

### 监控告警配置（建议）

**集成Prometheus + Grafana进行监控**

**监控指标**：
- 服务可用性（健康检查）
- API响应时间
- 调度任务执行状态
- 数据库连接数
- 错误率和日志级别统计

**告警规则**：
- 服务连续失败超过阈值
- 调度任务连续失败3次
- 数据库连接异常
- 磁盘空间不足

### 备份策略（建议）

- **数据库备份**：定期备份SQLite数据库文件（./data目录）
- **配置备份**：备份configs/config.yaml配置文件
- **备份频率**：每天备份一次，保留7天
- **备份方式**：使用cron任务或rsync同步到远程服务器

**示例备份脚本**：

```bash
#!/bin/bash
# backup.sh

DATE=$(date +%Y%m%d_%H%M%S)
BACKUP_DIR="/backup/magicpodcast"
DATA_DIR="./data"

# 创建备份目录
mkdir -p $BACKUP_DIR

# 备份数据库
cp $DATA_DIR/magicpodcast.db $BACKUP_DIR/magicpodcast_$DATE.db

# 备份配置
cp backend/configs/config.yaml $BACKUP_DIR/config_$DATE.yaml

# 删除7天前的备份
find $BACKUP_DIR -name "*.db" -mtime +7 -delete
find $BACKUP_DIR -name "*.yaml" -mtime +7 -delete
```

### 安全加固建议

- **API认证**：添加JWT或API Key认证机制
- **数据加密**：敏感数据（Cookie、API Key）加密存储
- **访问控制**：细粒度的权限控制
- **HTTPS配置**：添加nginx反向代理，配置SSL/TLS证书
- **网络隔离**：使用防火墙限制访问
- **依赖更新**：定期更新依赖包，修复安全漏洞

### 性能优化建议

- **数据库查询优化**：避免N+1问题，添加适当的索引
- **API响应缓存**：对不常变化的数据添加缓存层
- **前端性能优化**：虚拟滚动、懒加载、代码分割
- **图片优化**：使用CDN、图片压缩、WebP格式
- **并发控制**：合理设置并发数和请求间隔

---

## 附录

### 相关文档

- [环境变量配置说明](ENV_SETUP.md)
- [项目总览](../CLAUDE.md)
- [数据库索引指南](DATABASE_INDEX_GUIDE.md)

### 故障排查

**问题：服务无法启动**

1. 检查端口是否被占用：`lsof -i :8080`
2. 查看日志：`tail -f /tmp/backend.log`
3. 运行健康检查：`./health-check.sh`

**问题：调度任务未执行**

1. 检查调度器状态：`GET /api/v1/scheduler/status`
2. 查看工作流是否启用
3. 检查Cron表达式格式

**问题：数据库错误**

1. 检查数据库文件权限
2. 确认数据库文件路径正确
3. 查看日志中的数据库错误信息

---

**最后更新**: 2025-01-31
