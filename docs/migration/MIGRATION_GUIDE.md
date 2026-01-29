# MagicPodcast 项目迁移指南

## 迁移完成时间
2026-01-21

## 迁移原因
原项目位于 iCloud Drive 目录下，Next.js 的 `.next/server` 构建目录在 iCloud Drive 上无法正常工作，导致动态路由返回 404/500 错误。

## 当前项目位置
```
/Users/rookiestar/VSCode/Projects/MagicPodcast
```

## 原项目位置（保留作为备份）
```
~/Library/Mobile Documents/com~apple~CloudDocs/Projects/Play with AI/MagicPodcast
```

## 启动服务

### 前端
```bash
cd /Users/rookiestar/VSCode/Projects/MagicPodcast/frontend
npm run dev
# 访问: http://localhost:3000
```

### 后端
```bash
cd /Users/rookiestar/VSCode/Projects/MagicPodcast/backend
go run main.go
# 或使用预编译二进制:
./main
# API: http://localhost:8080
```

## 已部署的优化

### 工作流详情页批量查询优化

**性能提升**:
- API 调用次数: 7次 → 3次 (减少57%)
- 查询时间: 178ms → 46ms (提升3.9倍)
- 数据传输量: 1MB → 44KB (减少95.6%)
- 总加载时间: ~269ms → ~176ms (提升35%)

**后端变更**:
- 新增批量查询接口: `POST /api/v1/podcasts/batch`
- 文件: `backend/internal/handlers/podcast.go` (lines 222-275)
- 路由: `backend/internal/router/router.go` (line 61)

**前端变更**:
- 修改工作流详情页使用批量查询
- 文件: `frontend/src/app/workflows/[id]/page.tsx` (lines 99-108)
- API方法: `frontend/src/lib/api/podcast.ts` (lines 37-44)

## 测试验证

运行性能测试脚本:
```bash
~/MagicPodcast/test_batch_performance.sh
```

## Git 状态

优化代码已在新位置生效，建议提交更改:
```bash
cd /Users/rookiestar/VSCode/Projects/MagicPodcast
git add .
git commit -m "feat: 实现工作流详情页批量查询优化

- 新增批量查询API接口 (POST /api/v1/podcasts/batch)
- 优化工作流详情页加载性能 (35%提升)
- 减少API调用57% (7次→3次)
- 减少数据传输95.6% (1MB→44KB)

性能指标:
- 查询时间: 178ms → 46ms (3.9x提升)
- 总加载时间: 269ms → 176ms

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>"
```

## 下一步建议

1. **清理旧位置** (可选):
   - 确认新位置工作正常后，可以删除 iCloud Drive 上的旧项目
   - 或保留作为备份

2. **更新 VSCode 工作区**:
   - 在新位置打开项目: `code ~/MagicPodcast`

3. **更新环境变量** (如有):
   - 检查 `frontend/.env.local` 配置
   - 检查 `backend/configs/config.yaml` 配置

## 注意事项

- 新位置不在 iCloud Drive 上，不会自动同步到云端
- 建议定期手动备份到 iCloud 或其他云存储
- `.next` 目录已添加到 `.gitignore`，不会被提交到 Git
- 数据库文件 `MagicPodcast.db` 已复制到新位置

## 问题排查

如果遇到端口冲突:
```bash
# 查看占用端口的进程
lsof -ti:3000  # 前端
lsof -ti:8080  # 后端

# 杀死进程
kill -9 $(lsof -ti:3000)
kill -9 $(lsof -ti:8080)
```

如果遇到缓存问题:
```bash
cd /Users/rookiestar/VSCode/Projects/MagicPodcast/frontend
rm -rf .next
npm run dev
```
