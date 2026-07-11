# 工作流调度说明

最后更新：2026-05-31

本文记录当前工作流调度器的实际行为。旧设计草案已移入 [archive/planning/WORKFLOW_SCHEDULER_DESIGN.md](archive/planning/WORKFLOW_SCHEDULER_DESIGN.md)，只保留追溯用途。

## 当前行为

- 后端启动时会创建全局调度器，并加载所有已启用且配置了 `schedule` 的工作流。
- 调度器使用本机本地时区解析 cron 表达式。
- 支持 5 位和 6 位 cron 表达式；5 位表达式会自动补秒位。
- 每次触发前会重新读取工作流，已禁用的工作流会跳过。
- 如果上一次任务仍在运行，本次调度会跳过，避免同一个工作流重复执行。
- 启动后会检查 `next_run_at`，发现错过的执行时间时会异步补偿执行。
- 执行完成后会更新 `last_execution_at` 和 `next_run_at`。

## 接口

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| `GET` | `/api/v1/scheduler/status` | 查看调度器状态、注册任务数和工作流下次执行时间 |
| `POST` | `/api/v1/scheduler/reload` | 重新加载已启用工作流的调度配置 |
| `POST` | `/api/v1/scheduler/workflows/:id/pause` | 暂停指定工作流的当前调度注册 |
| `POST` | `/api/v1/scheduler/workflows/:id/resume` | 恢复指定工作流的调度注册 |

创建、更新、删除工作流时，处理器会同步添加或重新加载调度器，避免调度状态和数据库配置长期不一致。

## 运维检查

查看服务是否正常：

```bash
curl http://localhost:8080/health
curl http://localhost:8080/api/v1/scheduler/status
```

修改工作流 cron 后，如果页面没有自动刷新调度状态，可手工重载：

```bash
curl -X POST http://localhost:8080/api/v1/scheduler/reload
```

## 边界和注意事项

- 暂停接口只移除当前进程里的调度注册，不会把工作流写成禁用状态；服务重启或调度器重载后，会按数据库里的 `is_enabled` 和 `schedule` 重新注册。
- 深度调整补偿执行、失败告警或通知策略前，需要先确认真实使用预期，避免改变自动执行行为。
- 当前连续失败只记录告警日志，尚未接入邮件或 webhook 通知。
