# 工作流执行历史详情查看 - 设计方案

## 📋 问题分析

### 当前状态
- ✅ 后端API返回完整的Job数据，包含`executions`数组
- ✅ 前端可以获取Job列表
- ❌ 前端只显示摘要信息，无法查看详细执行报告
- ❌ 点击Job记录没有任何响应

### 用户需求
用户点击"执行历史"中的Job记录，期望看到：
1. 每个播客的同步详情
2. 成功/失败状态
3. 新建/更新的单集数
4. 执行耗时
5. 错误信息（如果有）

---

## 🎯 设计方案

### 方案A：可折叠列表（推荐）

**UI设计**：
```
┌─────────────────────────────────────────────────┐
│ ✓ 已完成  2026/1/17 07:51:25  手动  0s      │
│   处理: 2  发现: 0  创建: 0  错误: 0          │
│                                               │
│   ▼ 展开查看详情                               │
│   ┌─────────────────────────────────────────┐  │
│   │ ✓ 张小珺商业访谈录                        │  │
│   │   状态: 成功                             │  │
│   │   单集: 新建0 更新0                      │  │
│   │   耗时: 207ms                            │  │
│   │   RSS: https://feed.xyzfm.space/...     │  │
│   └─────────────────────────────────────────┘  │
│   ┌─────────────────────────────────────────┐  │
│   │ ✓ 42章经                                 │  │
│   │   状态: 成功                             │  │
│   │   单集: 新建0 更新0                      │  │
│   │   耗时: 156ms                            │  │
│   │   RSS: https://feed.xyzfm.space/...     │  │
│   └─────────────────────────────────────────┘  │
└─────────────────────────────────────────────────┘
```

**实现要点**：
1. 添加`selectedJob`状态跟踪展开的Job
2. 点击Job卡片时展开/折叠
3. 展开后显示`job.executions`列表
4. 每个execution显示完整信息

### 方案B：弹窗详情

**UI设计**：
```
点击Job → 弹出模态框显示完整报告

┌────────────────────────────────────────┐
│  执行报告 - Job #6                     │
│  ─────────────────────────────────────  │
│  状态: ✓ 已完成                         │
│  时间: 2026-01-17 07:51:25              │
│  耗时: 351ms                            │
│                                        │
│  执行统计:                              │
│  • 处理播客: 2 个                        │
│  • 发现单集: 0 个                        │
│  • 新建单集: 0 个                        │
│  • 失败: 0 个                            │
│                                        │
│  详细记录:                              │
│  ┌────────────────────────────────────┐ │
│  │ ✓ 张小珺商业访谈录                 │ │
│  │    耗时: 207ms                     │ │
│  │    单集: 0 新建, 0 更新           │ │
│  └────────────────────────────────────┘ │
│  ┌────────────────────────────────────┐ │
│  │ ✓ 42章经                          │ │
│  │    耗时: 156ms                     │ │
│  │    单集: 0 新建, 0 更新           │ │
│  └────────────────────────────────────┘ │
│                                        │
│  [关闭]                                │
└────────────────────────────────────────┘
```

### 方案C：独立详情页

跳转到新页面：`/workflows/2/jobs/6`

---

## 💡 推荐实现（方案A）

### 前端代码修改

**文件**: `frontend/src/app/workflows/[id]/page.tsx`

#### 1. 添加状态

```typescript
const [selectedJobId, setSelectedJobId] = useState<number | null>(null)
const [jobDetails, setJobDetails] = useState<Record<number, Job>>({})
```

#### 2. 添加获取Job详情的函数

```typescript
const fetchJobDetail = async (jobId: number) => {
  if (jobDetails[jobId]) {
    // 已缓存，直接切换
    setSelectedJobId(jobId === selectedJobId ? null : jobId)
    return
  }

  try {
    const detail = await workflowApi.getJob(jobId)
    setJobDetails(prev => ({ ...prev, [jobId]: detail }))
    setSelectedJobId(jobId)
  } catch (err) {
    console.error('Failed to fetch job detail:', err)
    alert('获取详情失败')
  }
}
```

#### 3. 修改执行历史的渲染

```tsx
{activeTab === 'jobs' && (
  <div>
    <h2 className="text-xl font-semibold text-slate-900 dark:text-slate-50 mb-4">执行历史</h2>
    {jobs.length === 0 ? (
      <div className="text-center py-8 text-slate-500 dark:text-slate-400">
        暂无执行记录
      </div>
    ) : (
      <div className="space-y-3">
        {jobs.map((job) => (
          <div key={job.id} className="border border-slate-200 dark:border-slate-700 rounded-lg overflow-hidden">
            {/* Job摘要卡片 - 可点击 */}
            <div
              onClick={() => fetchJobDetail(job.id)}
              className="p-4 hover:bg-slate-50 dark:hover:bg-slate-900 transition-colors cursor-pointer"
            >
              <div className="flex items-start justify-between mb-2">
                <div className="flex items-center gap-3">
                  {getJobStatusBadge(job.status)}
                  <span className="text-sm text-slate-600 dark:text-slate-400">
                    {new Date(job.created_at).toLocaleString('zh-CN')}
                  </span>
                  <span className="text-xs px-2 py-1 bg-slate-100 dark:bg-slate-700 rounded">
                    {job.triggered_by === 'cron' ? '定时' : '手动'}
                  </span>
                </div>
                <div className="flex items-center gap-2">
                  {job.duration && (
                    <span className="text-sm text-slate-600 dark:text-slate-400">
                      {Math.floor(job.duration / 1000)}s
                    </span>
                  )}
                  <span className="text-slate-400">
                    {selectedJobId === job.id ? '▲ 收起' : '▼ 展开'}
                  </span>
                </div>
              </div>

              <div className="grid grid-cols-4 gap-4 text-sm">
                <div>
                  <span className="text-slate-600 dark:text-slate-400">处理节目:</span>
                  <span className="ml-2 font-medium text-slate-900 dark:text-slate-50">
                    {job.podcasts_processed}
                  </span>
                </div>
                <div>
                  <span className="text-slate-600 dark:text-slate-400">发现单集:</span>
                  <span className="ml-2 font-medium text-slate-900 dark:text-slate-50">
                    {job.episodes_found}
                  </span>
                </div>
                <div>
                  <span className="text-slate-600 dark:text-slate-400">创建单集:</span>
                  <span className="ml-2 font-medium text-slate-900 dark:text-slate-50">
                    {job.episodes_created}
                  </span>
                </div>
                <div>
                  <span className="text-slate-600 dark:text-slate-400">错误数:</span>
                  <span className={`ml-2 font-medium ${
                    job.error_count > 0 ? 'text-red-600' : 'text-slate-900 dark:text-slate-50'
                  }`}>
                    {job.error_count}
                  </span>
                </div>
              </div>
            </div>

            {/* 展开的详细执行记录 */}
            {selectedJobId === job.id && jobDetails[job.id]?.executions && (
              <div className="border-t border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-slate-900/50 p-4">
                <h4 className="text-sm font-medium text-slate-700 dark:text-slate-300 mb-3">
                  详细执行记录
                </h4>
                <div className="space-y-2">
                  {jobDetails[job.id].executions.map((exec) => (
                    <div
                      key={exec.id}
                      className="bg-white dark:bg-slate-800 rounded-lg p-3 border border-slate-200 dark:border-slate-700"
                    >
                      <div className="flex items-start justify-between mb-2">
                        <div className="flex-1">
                          <div className="flex items-center gap-2 mb-1">
                            {exec.status === 'success' && (
                              <span className="text-green-600">✓</span>
                            )}
                            {exec.status === 'failed' && (
                              <span className="text-red-600">✗</span>
                            )}
                            {exec.status === 'skipped' && (
                              <span className="text-yellow-600">○</span>
                            )}
                            <span className="font-medium text-slate-900 dark:text-slate-50">
                              {exec.podcast_title}
                            </span>
                          </div>
                          <a
                            href={exec.podcast_feed_url}
                            target="_blank"
                            rel="noopener noreferrer"
                            className="text-xs text-blue-600 hover:text-blue-700 dark:text-blue-400 dark:hover:text-blue-300"
                          >
                            {exec.podcast_feed_url}
                          </a>
                        </div>
                        <span className="text-xs text-slate-500 dark:text-slate-400">
                          {exec.processing_time}ms
                        </span>
                      </div>

                      <div className="grid grid-cols-3 gap-4 text-xs">
                        <div>
                          <span className="text-slate-600 dark:text-slate-400">状态:</span>
                          <span className="ml-1 font-medium">
                            {exec.status === 'success' && '成功'}
                            {exec.status === 'failed' && '失败'}
                            {exec.status === 'skipped' && '跳过'}
                          </span>
                        </div>
                        <div>
                          <span className="text-slate-600 dark:text-slate-400">发现:</span>
                          <span className="ml-1 font-medium text-slate-900 dark:text-slate-50">
                            {exec.episodes_found}
                          </span>
                        </div>
                        <div>
                          <span className="text-slate-600 dark:text-slate-400">新建:</span>
                          <span className="ml-1 font-medium text-slate-900 dark:text-slate-50">
                            {exec.episodes_created}
                          </span>
                        </div>
                      </div>

                      {exec.error_message && (
                        <div className="mt-2 text-xs text-red-600 bg-red-50 dark:bg-red-900/20 rounded p-2">
                          错误: {exec.error_message}
                        </div>
                      )}
                    </div>
                  ))}
                </div>

                {jobDetails[job.id].executions.length === 0 && (
                  <div className="text-center py-4 text-sm text-slate-500 dark:text-slate-400">
                    暂无详细执行记录
                  </div>
                )}
              </div>
            )}
          </div>
        ))}
      </div>
    )}
  </div>
)}
```

---

## 🔧 API状态

### 现有API（已可用）

```
GET /api/v1/jobs/:id
```

**响应示例**：
```json
{
  "data": {
    "id": 6,
    "status": "completed",
    "podcasts_processed": 2,
    "episodes_found": 0,
    "episodes_created": 0,
    "error_count": 0,
    "duration": 351,
    "executions": [
      {
        "id": 1,
        "podcast_title": "张小珺商业访谈录",
        "status": "success",
        "episodes_found": 0,
        "episodes_created": 0,
        "processing_time": 207,
        "podcast_feed_url": "https://..."
      },
      {
        "id": 2,
        "podcast_title": "42章经",
        "status": "success",
        "episodes_found": 0,
        "episodes_created": 0,
        "processing_time": 156,
        "podcast_feed_url": "https://..."
      }
    ]
  }
}
```

---

## ✅ 实现检查清单

- [ ] 添加`selectedJobId`状态
- [ ] 添加`jobDetails`状态缓存
- [ ] 实现`fetchJobDetail`函数
- [ ] 修改Job卡片为可点击
- [ ] 添加展开/收起图标
- [ ] 实现Execution详情渲染
- [ ] 添加loading状态
- [ ] 添加错误处理
- [ ] 测试点击交互
- [ ] 测试数据展示

---

## 📊 优先级建议

### P0 - 核心功能
1. 实现可折叠的Job详情展示
2. 显示每个播客的执行结果
3. 显示错误信息（如果有）

### P1 - 体验优化
1. 添加loading状态
2. 缓存已获取的详情
3. 添加空状态提示

### P2 - 高级功能
1. 支持重试失败的执行
2. 导出执行报告为CSV
3. 执行详情的可视化图表

---

## 🎯 总结

**推荐方案**: 方案A（可折叠列表）

**优势**：
- ✅ 用户体验好，无需切换页面
- ✅ 实现简单，代码量适中
- ✅ 性能好，按需加载详情
- ✅ 视觉清晰，层次分明

**预计工作量**: 1-2小时

**技术要点**：
- 使用useState管理展开状态
- 缓存已获取的Job详情避免重复请求
- 条件渲染Execution列表
