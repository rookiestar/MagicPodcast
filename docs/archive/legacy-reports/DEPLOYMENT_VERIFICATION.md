# 部署验证指南

## 🚀 部署前准备

### 1. 环境检查

```bash
# 检查Go版本
cd backend
go version

# 检查Node版本
cd frontend
node --version
npm --version
```

**要求**:
- Go >= 1.21
- Node >= 18
- npm >= 9

### 2. 依赖安装

**后端**:
```bash
cd backend
go mod download
```

**前端**:
```bash
cd frontend
npm install
```

---

## 🚀 启动服务

### 后端服务

**开发模式**:
```bash
cd backend
go run cmd/api/main.go
```

**生产模式**:
```bash
cd backend
go build -o bin/api cmd/api/main.go
./bin/api
```

**预期输出**:
```
[GIN] Listening and serving HTTP on :8080
🔧 API_URL: http://localhost:8080
```

### 前端服务

**开发模式**:
```bash
cd frontend
npm run dev
```

**生产模式**:
```bash
cd frontend
npm run build
npm start
```

**预期输出**:
```
✓ Ready in 2.3s
○ → Local:   http://localhost:3000
```

---

## ✅ 验证清单

### Phase 1: 后端API验证

#### 1.1 健康检查
```bash
curl http://localhost:8080/health
```

**预期响应**:
```json
{
  "status": "ok",
  "database": "connected"
}
```

#### 1.2 搜索API测试
```bash
# 测试搜索API
curl "http://localhost:8080/api/v1/search?q=科技&type=all&page=1&page_size=10"
```

**验证点**:
- ✅ 响应时间 < 2秒
- ✅ 返回正确的播客和单集
- ✅ 分页信息正确

#### 1.3 工作流API测试
```bash
# 获取工作流列表
curl http://localhost:8080/api/v1/workflows

# 创建测试工作流
curl -X POST http://localhost:8080/api/v1/workflows \
  -H "Content-Type: application/json" \
  -d '{
    "name": "测试工作流",
    "description": "部署验证测试",
    "schedule": "0 0 2 * * *",
    "scope_config": {
      "type": "all_subscribed"
    },
    "rules_config": {
      "time_range": 7
    }
  }'
```

**验证点**:
- ✅ 创建成功
- ✅ 数据库正确存储
- ✅ 触发器正确注册

#### 1.4 标签关联API测试
```bash
# 测试播客标签API
curl -X POST http://localhost:8080/api/v1/podcasts/1/tags \
  -H "Content-Type: application/json" \
  -d '{"tag_id": 1}'

# 获取播客标签
curl http://localhost:8080/api/v1/podcasts/1/tags
```

**验证点**:
- ✅ 标签关联成功
- ✅ 重复添加被拒绝
- ✅ 删除功能正常

---

### Phase 2: 前端功能验证

#### 2.1 页面加载测试

**访问页面**:
1. http://localhost:3000 - 首页
2. http://localhost:3000/podcasts - 播客列表
3. http://localhost:3000/workflows - 工作流列表

**验证点**:
- ✅ 页面正常加载
- ✅ 无控制台错误
- ✅ 样式渲染正常

#### 2.2 播客功能测试

**测试用例**:
1. 查看播客列表（无限滚动）
2. 搜索播客
3. 筛选播客（按标签）
4. 排序播客
5. 查看播客详情
6. 添加/删除标签
7. 添加/编辑备注

**验证点**:
- ✅ 分页加载正常
- ✅ 搜索结果正确
- ✅ 筛选生效
- ✅ 标签操作成功
- ✅ 备注保存成功

#### 2.3 工作流功能测试

**测试用例**:
1. 创建新工作流（4步流程）
2. 编辑现有工作流
3. 删除工作流
4. 启用/禁用工作流
5. 手动触发工作流
6. 查看执行历史
7. 查看执行报告

**验证点**:
- ✅ 4步导航流畅
- ✅ 表单验证正确
- ✅ Cron表达式验证生效
- ✅ 创建/编辑成功
- ✅ 执行历史显示
- ✅ 报告渲染正确

#### 2.4 同步功能测试

**测试用例**:
1. 导入OPML文件
2. 手动同步订阅
3. 查看同步进度（SSE）
4. 查看同步状态

**验证点**:
- ✅ OPML导入成功
- ✅ SSE实时更新
- ✅ 进度显示正确
- ✅ 错误处理友好

---

### Phase 3: 性能验证

#### 3.1 API响应时间

**关键接口**:
- GET /api/v1/podcasts (P95 < 200ms)
- GET /api/v1/search (P95 < 500ms)
- POST /api/v1/workflows (P95 < 300ms)

**测试工具**:
```bash
# Apache Bench测试
ab -n 100 -c 10 http://localhost:8080/health

# 测试搜索API
ab -n 50 -c 5 "http://localhost:8080/api/v1/search?q=测试"
```

#### 3.2 前端性能

**关键指标**:
- 首屏加载时间 < 2秒
- 页面交互响应 < 100ms
- 无明显卡顿

**测试工具**:
- Chrome DevTools Performance
- Lighthouse评分 > 80

---

## 🔍 问题排查

### 常见问题及解决方案

#### 问题1: 后端启动失败

**症状**:
```
panic: failed to initialize database
```

**解决方案**:
```bash
# 检查数据库文件权限
ls -la backend/data/

# 删除锁文件（如果有）
rm backend/data/*.db-shm
rm backend/data/*.db-wal
```

#### 问题2: 前端API调用失败

**症状**:
```
ERR_CONNECTION_REFUSED
```

**解决方案**:
1. 检查后端服务是否运行: `curl http://localhost:8080/health`
2. 检查API_URL配置: frontend/.env.local
3. 检查CORS配置

#### 问题3: 搜索结果不正确

**症状**:
- 搜索结果为空
- 排序错误

**解决方案**:
1. 检查SearchService日志
2. 验证数据库连接
3. 检查搜索参数

#### 问题4: 工作流创建失败

**症状**:
- 提交后无响应
- 验证错误

**解决方案**:
1. 检查Cron表达式格式
2. 检查scope_config配置
3. 查看后端日志

---

## 📋 验证报告模板

### 测试环境

- **后端版本**: __________
- **前端版本**: __________
- **Go版本**: __________
- **Node版本**: __________

### 功能验证结果

#### 后端API ✅/❌

- [ ] 健康检查
- [ ] 搜索API
- [ ] 工作流API
- [ ] 标签关联API
- [ ] 同步API

#### 前端功能 ✅/❌

- [ ] 页面加载
- [ ] 播客功能
- [ ] 工作流功能
- [ ] 同步功能
- [ ] 标签功能

#### 性能指标 ✅/❌

- [ ] API P95 < 200ms
- [ ] 首屏加载 < 2s
- [ ] Lighthouse > 80

---

## 🎯 验证通过标准

### 必须通过（P0）

- ✅ 所有页面正常加载
- ✅ 核心CRUD功能正常
- ✅ 无阻塞性错误
- ✅ API响应时间可接受

### 应该通过（P1）

- ✅ 搜索结果准确
- ✅ 分页加载流畅
- ✅ 实时更新正常
- ✅ 错误提示友好

### 可选优化（P2）

- ⏳ Lighthouse评分 > 90
- ⏳ API P95 < 100ms
- ⏳ 前端首屏 < 1.5s

---

## 🚀 下一步

验证通过后：

1. **生成技术文档** - 记录API文档、架构文档
2. **性能基准测试** - 建立性能基线
3. **CI/CD配置** - 自动化测试和部署
4. **开始新功能开发** - 基于重构后的代码

---

**准备好开始验证了吗？**
