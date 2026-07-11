# Phase 1 骨架代码适配说明

**创建日期**: 2026-02-01
**状态**: 待适配

---

## 概述

Phase 1 创建的 Service 层代码使用了简化的 DTO 和字段名，需要在 Phase 2 实际重构时适配到真实的模型结构。

---

## 需要适配的 Service

### 1. WorkflowService

**模型差异**：

```go
// 实际模型 (models.Workflow)
type Workflow struct {
    Name        string  // 不是 Title
    Schedule    string  // 不是 ScheduleConfig (是字符串)
    ScopeConfig ScopeConfig
    RulesConfig RulesConfig
    IsEnabled   bool
}

// Service DTO 中使用
type CreateWorkflowRequest struct {
    Title          string              // ❌ 应该是 Name
    ScheduleConfig ScheduleConfig     // ❌ 应该解析为字符串
}
```

**适配方案**（Phase 2）：

```go
// 方案1: 修改DTO字段名匹配模型
type CreateWorkflowRequest struct {
    Name        string  // ✅
    Schedule    string  // ✅ (直接使用cron表达式字符串)
    ScopeConfig ScopeConfig
    RulesConfig RulesConfig
}

// 方案2: 在Service中做映射
func (s *WorkflowService) CreateWorkflow(req *CreateWorkflowRequest) (*WorkflowResponse, error) {
    workflow := &models.Workflow{
        Name:        req.Title,  // 映射
        Schedule:    req.ScheduleConfig.CronExpression, // 提取cron
        ScopeConfig: req.ScopeConfig,
        RulesConfig: req.RulesConfig,
    }
    // ...
}
```

---

### 2. TagService

**模型差异**：

```go
// 实际模型 (models.Tag)
type Tag struct {
    ID        uint
    Name      string
    Color     string
    // ❌ 没有 Description 字段
}

// Service DTO 中使用
type CreateTagRequest struct {
    Name        string
    Color       string
    Description string  // ❌ 模型中没有
}
```

**适配方案**（Phase 2）：

**方案A**: 移除 Description 字段
```go
type CreateTagRequest struct {
    Name  string `json:"name" binding:"required"`
    Color string `json:"color" binding:"required"`
    // 移除 Description
}
```

**方案B**: 扩展模型添加 Description
```go
// 在 models.Tag 中添加
type Tag struct {
    ID          uint
    Name        string
    Color       string
    Description string  // 新增字段
    // ...
}
```

---

### 3. PodcastService

**模型差异**：

Podcast 模型相对匹配，但需要验证所有字段是否存在。

**适配方案**（Phase 2）：

1. 检查 models.Podcast 的所有字段
2. 确保 DTO 与模型字段匹配
3. 调整字段映射逻辑

---

## Phase 2 适配步骤

### Step 1: 模型审查
```bash
# 查看实际模型定义
cat internal/models/workflow.go
cat internal/models/tag.go
cat internal/models/podcast.go
```

### Step 2: 更新 Service DTO
1. 修改 Request/Response 结构体字段名
2. 添加/删除字段以匹配模型
3. 更新 JSON tag

### Step 3: 更新 Service 方法
1. 修改字段映射逻辑
2. 更新验证逻辑
3. 测试所有 CRUD 操作

### Step 4: 单元测试
```bash
# 为 Service 创建测试
go test ./internal/services/workflow_service_test.go
go test ./internal/services/tag_service_test.go
go test ./internal/services/podcast_service_test.go
```

---

## 优先级

1. **高优先级**: TagService（最简单，先适配）
2. **中优先级**: PodcastService（中等复杂度）
3. **低优先级**: WorkflowService（最复杂，需要在Phase 2后期重构）

---

## 注意事项

1. **不要直接修改 models**
   - Phase 1 只是创建骨架
   - 模型结构变更需要数据迁移

2. **保持 API 兼容性**
   - 前端可能依赖某些字段名
   - 考虑添加字段别名

3. **渐进式适配**
   - 一次适配一个 Service
   - 每个适配后完整测试

---

**文档版本**: v1.0
**最后更新**: 2026-02-01
