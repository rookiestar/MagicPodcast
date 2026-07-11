# 工作流定时调度器 - 设计方案

## 📋 问题分析

### 当前状态
- ✅ 工作流模型包含 `Schedule` 字段，支持cron表达式
- ✅ 手动触发工作流功能已实现（`/api/v1/workflows/:id/trigger`）
- ✅ 工作流执行器（Executor）已完善，支持并发同步
- ❌ **缺少自动调度服务**，无法按计划自动执行工作流
- ❌ 全局配置 `sync.schedule` 仅适用于全量同步，不支持工作流级别的调度

### 用户需求
1. 每个工作流可以配置独立的cron表达式（如每天凌晨2点、每周一早上9点）
2. 系统自动按照计划执行已启用的工作流
3. 避免重复执行：如果上一次任务还在运行，跳过本次调度
4. 记录调度历史和执行结果
5. 支持暂停/恢复单个工作流的调度

---

## 🎯 设计方案

### 方案对比

| 方案 | 优点 | 缺点 | 推荐度 |
|------|------|------|--------|
| **A. robfig/cron（进程内）** | • 实现简单<br>• 无外部依赖<br>• 与现有代码集成容易 | • 进程重启丢失任务<br>• 单点故障<br>• 难以扩展 | ⭐⭐⭐⭐⭐ |
| **B. 系统cron + HTTP调用** | • 系统级稳定性<br>• 进程独立 | • 依赖外部配置<br>• 难以动态管理<br>• 错误处理复杂 | ⭐⭐ |
| **C. 分布式任务队列（如Celery）** | • 高可用<br>• 易扩展 | • 架构复杂<br>• 引入新依赖<br>• 过度设计 | ⭐⭐ |

### 推荐方案：**A. robfig/cron（进程内）**

**理由**：
1. 本项目为个人使用，无高可用需求
2. 与现有后端服务集成简单
3. robfig/cron库成熟稳定，被广泛使用
4. 支持标准cron表达式和秒级精度

---

## 🏗️ 架构设计

### 系统架构

```
┌─────────────────────────────────────────────────────────┐
│                   Backend Service                        │
│                                                          │
│  ┌──────────────────────────────────────────────────┐  │
│  │           Scheduler Service (NEW)                │  │
│  │                                                   │  │
│  │  • Load enabled workflows with schedules         │  │
│  │  • Register cron jobs with robfig/cron           │  │
│  │  • Check for running jobs before execution       │  │
│  │  • Call workflow.Executor for each scheduled run │  │
│  │  • Log execution events                          │  │
│  └───────────────┬──────────────────────────────────┘  │
│                  │                                       │
│                  ▼                                       │
│  ┌──────────────────────────────────────────────────┐  │
│  │           Workflow Executor (EXISTING)           │  │
│  │                                                   │  │
│  │  • Execute workflow logic                        │  │
│  │  • Manage Job records                            │  │
│  │  • Coordinate podcast sync                       │  │
│  └──────────────────────────────────────────────────┘  │
│                                                          │
│  ┌──────────────────────────────────────────────────┐  │
│  │           Database (SQLite)                      │  │
│  │                                                   │  │
│  │  workflows                                         │
│  │  jobs                                              │  │
│  │  job_executions                                    │  │
│  └──────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────┘

API Endpoints:
  POST /api/v1/scheduler/reload   - Reload workflow schedules
  GET  /api/v1/scheduler/status   - Get scheduler status
  POST /api/v1/scheduler/pause    - Pause scheduler
  POST /api/v1/scheduler/resume   - Resume scheduler
```

### 数据库模型变更

**现有 Workflow 模型已足够**，无需新增字段：
```go
type Workflow struct {
    // ...
    Schedule      string `json:"schedule"`      // Cron表达式: "0 2 * * *"
    IsEnabled     bool   `json:"is_enabled"`    // 是否启用
    LastJobID     *uint  `json:"last_job_id"`   // 最后执行的Job ID
    // ...
}
```

**可选增强**（Phase 2）：
```go
// 添加到 Workflow 表
NextRunAt     *time.Time `json:"next_run_at"`     // 下次执行时间
LastRunAt     *time.Time `json:"last_run_at"`     // 上次执行时间
```

---

## 💻 核心实现

### 文件结构

```
backend/
├── internal/
│   ├── scheduler/
│   │   ├── scheduler.go       # Scheduler主逻辑
│   │   ├── job.go             # Cron job封装
│   │   └── logger.go          # Scheduler专用日志
│   ├── handlers/
│   │   └── scheduler.go       # HTTP handlers
│   └── router/
│       └── router.go          # 添加scheduler初始化
```

### 1. Scheduler Service

**文件**: `backend/internal/scheduler/scheduler.go`

```go
package scheduler

import (
    "context"
    "fmt"
    "log"
    "sync"

    "magicpodcast/internal/models"
    "magicpodcast/internal/workflow"

    "github.com/robfig/cron/v3"
    "gorm.io/gorm"
)

// Scheduler 工作流调度器
type Scheduler struct {
    db       *gorm.DB
    executor *workflow.Executor
    cron     *cron.Cron
    jobIDs   map[uint]cron.EntryID // workflowID -> cron.EntryID
    mu       sync.RWMutex
}

// NewScheduler 创建调度器
func NewScheduler(db *gorm.DB, executor *workflow.Executor) *Scheduler {
    // 使用秒级精度的cron解析器
    return &Scheduler{
        db:       db,
        executor: executor,
        cron:     cron.New(cron.WithSeconds()),
        jobIDs:   make(map[uint]cron.EntryID),
    }
}

// Start 启动调度器
func (s *Scheduler) Start() error {
    log.Println("🕐 启动工作流调度器")

    // 加载所有已启用的工作流
    var workflows []models.Workflow
    if err := s.db.Where("is_enabled = ? AND schedule != ?", true, "").
        Find(&workflows).Error; err != nil {
        return fmt.Errorf("加载工作流失败: %w", err)
    }

    // 注册每个工作流的cron任务
    for _, wf := range workflows {
        if err := s.registerWorkflow(&wf); err != nil {
            log.Printf("⚠️  注册工作流失败 [ID=%d]: %v", wf.ID, err)
            continue
        }
        log.Printf("✅ 已注册工作流 [ID=%d, Schedule=%s]", wf.ID, wf.Schedule)
    }

    s.cron.Start()
    log.Printf("🚀 调度器已启动，共注册 %d 个工作流", len(workflows))
    return nil
}

// Stop 停止调度器
func (s *Scheduler) Stop() {
    log.Println("🛑 停止工作流调度器")
    s.cron.Stop()
    s.mu.Lock()
    s.jobIDs = make(map[uint]cron.EntryID)
    s.mu.Unlock()
}

// registerWorkflow 注册单个工作流
func (s *Scheduler) registerWorkflow(workflow *models.Workflow) error {
    schedule := workflow.Schedule
    if schedule == "" {
        return fmt.Errorf("schedule为空")
    }

    // 封装执行逻辑
    jobFunc := func() {
        s.executeWorkflow(workflow.ID)
    }

    // 添加到cron
    entryID, err := s.cron.AddFunc(schedule, jobFunc)
    if err != nil {
        return fmt.Errorf("添加cron任务失败: %w", err)
    }

    // 记录映射
    s.mu.Lock()
    s.jobIDs[workflow.ID] = entryID
    s.mu.Unlock()

    return nil
}

// executeWorkflow 执行工作流（带重复执行检查）
func (s *Scheduler) executeWorkflow(workflowID uint) {
    log.Printf("⏰ [调度] 触发工作流 [ID=%d]", workflowID)

    // 重新加载工作流（确保获取最新状态）
    var workflow models.Workflow
    if err := s.db.First(&workflow, workflowID).Error; err != nil {
        log.Printf("❌ [调度] 工作流不存在 [ID=%d]: %v", workflowID, err)
        return
    }

    // 检查是否启用
    if !workflow.IsEnabled {
        log.Printf("⏭️  [调度] 工作流已禁用，跳过执行 [ID=%d]", workflowID)
        return
    }

    // 检查是否有正在运行的任务
    if workflow.LastJobID != nil {
        var lastJob models.Job
        if err := s.db.Where("id = ?", *workflow.LastJobID).First(&lastJob).Error; err == nil {
            if lastJob.Status == models.JobStatusRunning {
                log.Printf("⏭️  [调度] 工作流正在运行，跳过本次执行 [ID=%d, JobID=%d]",
                    workflowID, lastJob.ID)
                return
            }
        }
    }

    // 执行工作流
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
    defer cancel()

    job, err := s.executor.Execute(ctx, &workflow, "cron")
    if err != nil {
        log.Printf("❌ [调度] 工作流执行失败 [ID=%d]: %v", workflowID, err)
    } else {
        log.Printf("✅ [调度] 工作流执行完成 [ID=%d, JobID=%d, 状态=%s]",
            workflowID, job.ID, job.Status)
    }
}

// Reload 重新加载所有工作流
func (s *Scheduler) Reload() error {
    log.Println("🔄 重新加载调度器")

    // 停止现有任务
    for wfID, entryID := range s.jobIDs {
        s.cron.Remove(entryID)
        log.Printf("🗑️  移除工作流 [ID=%d]", wfID)
    }

    // 清空映射
    s.mu.Lock()
    s.jobIDs = make(map[uint]cron.EntryID)
    s.mu.Unlock()

    // 重新启动
    return s.Start()
}

// PauseWorkflow 暂停指定工作流的调度
func (s *Scheduler) PauseWorkflow(workflowID uint) error {
    s.mu.Lock()
    defer s.mu.Unlock()

    entryID, exists := s.jobIDs[workflowID]
    if !exists {
        return fmt.Errorf("工作流未注册调度")
    }

    s.cron.Remove(entryID)
    delete(s.jobIDs, workflowID)

    log.Printf("⏸️  已暂停工作流调度 [ID=%d]", workflowID)
    return nil
}

// ResumeWorkflow 恢复指定工作流的调度
func (s *Scheduler) ResumeWorkflow(workflowID uint) error {
    var workflow models.Workflow
    if err := s.db.First(&workflow, workflowID).Error; err != nil {
        return fmt.Errorf("工作流不存在: %w", err)
    }

    if !workflow.IsEnabled {
        return fmt.Errorf("工作流未启用")
    }

    if workflow.Schedule == "" {
        return fmt.Errorf("工作流未配置schedule")
    }

    return s.registerWorkflow(&workflow)
}

// GetStatus 获取调度器状态
func (s *Scheduler) GetStatus() map[string]interface{} {
    s.mu.RLock()
    defer s.mu.RUnlock()

    entries := s.cron.Entries()

    return map[string]interface{}{
        "is_running":   true,
        "total_jobs":   len(entries),
        "workflows":    s.jobIDs,
    }
}
```

### 2. HTTP Handlers

**文件**: `backend/internal/handlers/scheduler.go`

```go
package handlers

import (
    "net/http"

    "magicpodcast/internal/scheduler"

    "github.com/gin-gonic/gin"
)

type SchedulerHandler struct {
    scheduler *scheduler.Scheduler
}

func NewSchedulerHandler(scheduler *scheduler.Scheduler) *SchedulerHandler {
    return &SchedulerHandler{scheduler: scheduler}
}

// Reload 重新加载调度器
func (h *SchedulerHandler) Reload(c *gin.Context) {
    if err := h.scheduler.Reload(); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{
            "success": false,
            "error": gin.H{
                "code":    "SCHEDULER_RELOAD_FAILED",
                "message": "重新加载调度器失败",
            },
        })
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "调度器已重新加载",
    })
}

// GetStatus 获取调度器状态
func (h *SchedulerHandler) GetStatus(c *gin.Context) {
    status := h.scheduler.GetStatus()

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "data":    status,
    })
}

// PauseWorkflow 暂停工作流调度
func (h *SchedulerHandler) PauseWorkflow(c *gin.Context) {
    workflowID := c.Param("id")
    // TODO: 验证workflowID格式
    // ...

    if err := h.scheduler.PauseWorkflow(workflowID); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{
            "success": false,
            "error": gin.H{
                "code":    "PAUSE_FAILED",
                "message": err.Error(),
            },
        })
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "工作流调度已暂停",
    })
}

// ResumeWorkflow 恢复工作流调度
func (h *SchedulerHandler) ResumeWorkflow(c *gin.Context) {
    workflowID := c.Param("id")
    // TODO: 验证workflowID格式
    // ...

    if err := h.scheduler.ResumeWorkflow(workflowID); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{
            "success": false,
            "error": gin.H{
                "code":    "RESUME_FAILED",
                "message": err.Error(),
            },
        })
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "message": "工作流调度已恢复",
    })
}
```

### 3. Router 集成

**文件**: `backend/internal/router/router.go`

```go
func SetupRouter() *gin.Engine {
    // ... 现有代码 ...

    // 创建scheduler
    schedulerSvc := scheduler.NewScheduler(db, workflowExecutor)

    // 启动调度器（在独立goroutine中）
    go func() {
        if err := schedulerSvc.Start(); err != nil {
            log.Printf("❌ 启动调度器失败: %v", err)
        }
    }()

    // 注册handlers
    schedulerHandler := handlers.NewSchedulerHandler(schedulerSvc)

    // 注册路由
    api := r.Group("/api/v1")
    {
        // ... 现有路由 ...

        // 调度器管理接口
        scheduler := api.Group("/scheduler")
        {
            scheduler.POST("/reload", schedulerHandler.Reload)
            scheduler.GET("/status", schedulerHandler.GetStatus)
            scheduler.POST("/workflows/:id/pause", schedulerHandler.PauseWorkflow)
            scheduler.POST("/workflows/:id/resume", schedulerHandler.ResumeWorkflow)
        }
    }

    return r
}
```

---

## 📝 API 文档

### 1. 重新加载调度器
```http
POST /api/v1/scheduler/reload
```

**响应**:
```json
{
  "success": true,
  "message": "调度器已重新加载"
}
```

### 2. 获取调度器状态
```http
GET /api/v1/scheduler/status
```

**响应**:
```json
{
  "success": true,
  "data": {
    "is_running": true,
    "total_jobs": 3,
    "workflows": {
      "1": 45,
      "2": 46,
      "5": 47
    }
  }
}
```

### 3. 暂停工作流调度
```http
POST /api/v1/scheduler/workflows/:id/pause
```

**响应**:
```json
{
  "success": true,
  "message": "工作流调度已暂停"
}
```

### 4. 恢复工作流调度
```http
POST /api/v1/scheduler/workflows/:id/resume
```

**响应**:
```json
{
  "success": true,
  "message": "工作流调度已恢复"
}
```

---

## 🧪 测试方案

### 单元测试

**文件**: `backend/internal/scheduler/scheduler_test.go`

```go
package scheduler

import (
    "testing"
    "time"

    "github.com/robfig/cron/v3"
    "github.com/stretchr/testify/assert"
)

func TestCronExpression(t *testing.T) {
    tests := []struct {
        schedule   string
        shouldPass bool
    }{
        {"0 2 * * *", true},          // 每天凌晨2点
        {"0 */6 * * *", true},        // 每6小时
        {"0 9 * * 1-5", true},        // 工作日早上9点
        {"invalid", false},
    }

    for _, tt := range tests {
        _, err := cron.ParseStandard(tt.schedule)
        if tt.shouldPass {
            assert.NoError(t, err)
        } else {
            assert.Error(t, err)
        }
    }
}

func TestSchedulerStartStop(t *testing.T) {
    // TODO: 实现启动/停止测试
}

func TestDuplicateExecutionPrevention(t *testing.T) {
    // TODO: 测试重复执行检查逻辑
}
```

### 集成测试

1. **创建测试工作流**，设置schedule为 `*/1 * * * *`（每分钟执行）
2. **等待2分钟**，检查数据库中是否创建2个Job
3. **修改工作流为禁用**，验证不再创建新Job
4. **重新启用工作流**，验证调度恢复

---

## 📊 实施计划

### Phase 1: 核心功能（1-2天）
- [ ] 安装依赖 `go get github.com/robfig/cron/v3`
- [ ] 实现 `Scheduler` 基础结构
- [ ] 实现 `Start()` 和 `Stop()` 方法
- [ ] 实现 `registerWorkflow()` 方法
- [ ] 实现 `executeWorkflow()` 方法（含重复执行检查）
- [ ] 在 `router.go` 中初始化和启动scheduler
- [ ] 集成测试：手动创建工作流，验证自动执行

### Phase 2: 管理接口（1天）
- [ ] 实现 `SchedulerHandler`
- [ ] 添加HTTP路由（reload, status, pause, resume）
- [ ] 测试管理接口
- [ ] 添加日志记录

### Phase 3: 增强功能（可选）
- [ ] 添加 `NextRunAt` 字段到Workflow表
- [ ] 前端展示下次执行时间
- [ ] 支持修改schedule后自动重载
- [ ] 添加调度历史记录
- [ ] 错误重试机制

### Phase 4: 监控与告警
- [ ] 添加调度执行统计（成功率、平均耗时）
- [ ] 失败告警（如连续失败3次）
- [ ] 前端可视化展示调度状态

---

## ⚠️ 注意事项

### 1. Cron表达式格式

**robfig/cron 支持6位表达式**（秒 分 时 日 月 周）：

```
┌───────────── second (0 - 59)
│ ┌───────────── minute (0 - 59)
│ │ ┌───────────── hour (0 - 23)
│ │ │ ┌───────────── day of month (1 - 31)
│ │ │ │ ┌───────────── month (1 - 12)
│ │ │ │ │ ┌───────────── day of week (0 - 6) (Sunday to Saturday)
│ │ │ │ │ │
* * * * * *
```

**常用示例**：
- `0 2 * * *` - 每天凌晨2点
- `0 */6 * * *` - 每6小时
- `0 9 * * 1-5` - 工作日早上9点
- `0 0 * * 0` - 每周日午夜

### 2. 时区处理

robfig/cron默认使用**系统本地时区**。建议在代码中明确指定：

```go
// 使用时区
location, _ := time.LoadLocation("Asia/Shanghai")
cron.New(cron.WithSeconds(), cron.WithLocation(location))
```

### 3. 进程重启

由于cron任务在内存中，**进程重启会丢失未执行的调度**。解决方案：
- 每次启动时重新加载所有已启用工作流
- 使用容器编排（如Kubernetes CronJob）- 过度设计，不推荐
- 接受重启后丢失本次调度（下次会正常执行）

### 4. 并发执行

- robfig/cron默认**禁止并发执行同一任务**
- 如果任务执行时间超过cron间隔，会跳过本次执行
- 本设计已有**重复执行检查**，双重保险

### 5. 日志管理

调度器日志应包含：
- 调度触发：`⏰ [调度] 触发工作流 [ID=%d]`
- 跳过执行：`⏭️  [调度] 工作流正在运行，跳过本次执行`
- 执行结果：`✅ [调度] 工作流执行完成 [ID=%d, JobID=%d]`

---

## 🎯 总结

**实现目标**：工作流按计划自动执行，无需人工干预

**核心优势**：
- ✅ 架构简单，易于维护
- ✅ 与现有代码无缝集成
- ✅ 支持动态增删工作流
- ✅ 自动防止重复执行
- ✅ 完整的管理接口

**预计工作量**：2-3天

**依赖库**：
```bash
go get github.com/robfig/cron/v3@latest
```

**下一步行动**：
1. 实现Phase 1核心功能
2. 集成测试验证
3. 补充管理接口（Phase 2）
4. 前端适配（显示下次执行时间）
