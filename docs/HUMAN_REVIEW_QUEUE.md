# 需要人工确认的事项

最后更新：2026-07-12

这些事项不影响当前功能验证，但需要使用者按真实使用习惯确认后再继续清理或改动。

## 暂缓处理

| 事项 | 原因 | 建议 |
| --- | --- | --- |
| `backend/cmd/maint/` 下的一次性维护命令 | 多数脚本面向历史数据修复，无法仅凭代码判断是否仍会被使用 | 后续按真实维护需求保留、合并或归档 |
| `backend/cmd/migrate` 版本化迁移命令 | `--apply` 会改写真实数据库，属于高风险数据库写操作 | 仅按 `docs/migration/MIGRATION_GUIDE.md` 先备份、验证、停服务，再用确认字符串和备份路径运行；普通启动不再自动迁移 |
| `backend/scripts/fix_newest_episode_date.sql` 和 `backend/scripts/init_tags.sql` | 属于会改写真实数据的一次性 SQL，不能仅凭未被代码引用就自动删除 | 后续确认是否仍需手工维护入口；如不需要可迁入归档或删除 |
| `archive/` 下的旧 Docker / Nginx 配置 | 当前启动方式已改为脚本，但旧部署记录可能仍有参考价值 | 确认不再需要 Docker 部署后再删除 |
| 工作流详情页和 `WorkflowFormModal` 大组件 | 文件仍偏大，继续拆分需要页面级交互验证 | 单独安排工作流专项，先截图和点击验证后再拆 |
| 搜索深度优化 | 热态搜索已降到约 146-184ms，并发 P95 约 187ms，但重启后第一跳仍可能超过 500ms；进一步优化排序、缓存或查询策略可能改变搜索结果顺序 | 单独确认是否接受搜索排序或缓存策略调整，再继续压榨搜索上限 |
| 前端开发依赖审计剩余提示 | `npm audit --omit=dev` 已为 0；完整审计仍有 4 项开发/测试工具风险（2 low、1 high、1 critical），不随生产构建部署 | 后续单独评估 `@babel/core`、`esbuild`、`vite` 和 `vitest` 的升级，先保持当前已验证测试入口 |
| Go 依赖漏洞扫描 | 2026-06-01 重试仍失败：访问 `proxy.golang.org` 超时；改用 `GOPROXY=direct` 后访问 `golang.org` 仍超时，无法给出有效结论 | 网络稳定后重跑 `go run golang.org/x/vuln/cmd/govulncheck@latest ./...` |
| Go 静态诊断扩展扫描 | 2026-06-01 重试仍失败：访问 `proxy.golang.org` 超时；改用 `GOPROXY=direct` 后访问 GitHub 仍失败，当前仅以 `go vet` 覆盖基础静态检查 | 网络稳定后重跑 `go run honnef.co/go/tools/cmd/staticcheck@latest ./...` |
| 前端生产清理扫描剩余提示 | `knip --production` 仍提示 `vitest.setup.ts`、Tailwind 插件和 20 个测试覆盖的辅助函数；这些要么由配置使用，要么专门用于单元测试验证边界 | 若希望继续压缩公开测试入口，需要先确认测试策略：保留白盒辅助函数，或改为只测用户可见行为 |
| 调度器连续失败通知策略 | 当前连续失败只记录日志；自动邮件、webhook 或其他通知会改变使用体验和打扰频率 | 单独确认通知渠道、阈值和是否需要静默时段后再实现 |
| 本地 `backend/api`、`backend/configs/config.yaml*`、`.env*`、日志、数据库、`.bak` 和备份 | 属于本机运行产物、配置或手工留存文件，可能含敏感配置或正在被服务使用 | 不纳入版本库，不做自动删除；确认不需要后可手动清理 |

## 已自动处理

| 事项 | 处理 |
| --- | --- |
| 跟踪中的 Go 编译产物 | 已从版本库删除，并补充忽略规则 |
| 损坏的 `backend/internal/router/router.go.new` | 已删除 |
| 一次性测试二维码和工作流备份 SQL | 已删除 |
| 被跟踪进版本库的调度器测试 SQLite 产物 | 已删除 `backend/internal/scheduler/test_scheduler_*`，并补充忽略规则 |
| 旧调度器手工回归脚本 | `backend/test_scheduler_reload.sh` 会写真实数据库且调用不存在的测试二进制，已删除 |
| 工作流服务中的未完成触发方法 | `WorkflowService.TriggerWorkflow` 无任何调用方，且真实手动触发已由当前接口处理层执行，已删除 |
| 工作流服务直接查询任务数量 | 已把任务计数收回到工作流仓储入口，接口返回保持不变 |
| 性能巡检脚本默认前端端口 | 已从旧的 `8088` 改为当前生产启动使用的 `3000` |
| 性能巡检冷态波动 | 搜索单次冷态采样曾到 913ms，连续热态复测回到 130-156ms；脚本已增加默认 1 次预热，并保留 `--warmup-runs 0` 用于冷态复查 |
| 前端 Vitest 初始样例测试 | `frontend/src/__tests__/sample.test.tsx` 只验证 Hello World 和基础断言，真实测试已覆盖配置，已删除 |
| `.gitignore` 数据库规则含糊 | 已去掉“保留本地数据库”的误导性例外，明确忽略 `backend/data/` 下的本地数据文件 |
| 后端示例响应测试空跑 | 原 `example_test.go` 中有测试没有断言，已改为真实响应校验并重命名为 `error_response_test.go` |
| 前端生产依赖风险 | 已在取得单独授权后将 Next 14 升至 Next 16.2.10，并更新 axios、DOMPurify、PostCSS、ESLint 和 form-data；`npm audit --omit=dev` 为 0，生产构建和回退发布验证通过 |
| 前端未使用依赖 | 已删除未引用的 `@testing-library/user-event`、`jsdom` 和 `@types/dompurify`，减少依赖面 |
| 前端未引用辅助文件 | 已删除未被运行代码或测试引用的 `ErrorBoundary`、`useApi`、`useViewportDetection`、`test-utils` 和重复的 `global.d.ts` |
| 前端严格未使用扫描 | 已清理未使用的导入、闲置状态、空函数和未使用参数，保留现有页面行为 |
| 前端遗留导出扫描 | 已删除未引用的旧搜索 Hook、日志工具、默认导出、旧分页 Hook、单集骨架和未使用 Cron 描述函数；测试入口和 Tailwind 插件保留 |
| 前端 API 和 Hook 导出面收口 | 已删除未使用的健康检查封装、旧列表 Hook、旧标签详情 Hook、多参数 URL Hook、分页 fetcher、图片预加载入口和多余提示函数；仅内部使用的 API 函数和类型已收回 |
| 被错误忽略但实际被后端引用的 `backend/internal/scraper` | 已恢复为可纳入版本库的源码 |
| 旧移动端说明里的固定 IP 和已失效脚本引用 | 已改为通用局域网 IP 流程 |
| 与产品无关的 `skills/`、`tools/` 脚手架 | 无代码引用，且打包脚本依赖缺失，已删除 |
| 大量 `PHASE*`、`FINAL*`、`*_REPORT.md` 历史文档 | 已迁入 `docs/archive/`，保留追溯用途，不再混在当前文档入口 |
| `docs/reports/` 和 `docs/planning/` 历史材料 | 已迁入 `docs/archive/reports/` 和 `docs/archive/planning/`，当前入口只保留维护文档和专题文档 |
| 过时的清理、部署、环境和索引说明 | 已按当前 `scripts/` 入口、生产启动方式和真实索引脚本重写 |
| 搜索服务旧微基准 | 原基准依赖未初始化的全局配置和数据库，会直接崩溃；已改成独立内存基准 |
| 搜索 `type=all` 顺序等待 | 播客和单集搜索互相独立，已改为并行查询；测试确认结果与分别搜索两次一致 |
| 生产模式启动配置 | 后端已可通过环境变量覆盖为 release 模式，并默认关闭数据库 SQL 调试日志；启动脚本已改为后台会话方式并完成重启验证 |
| 搜索服务测试过渡残留 | `search_service_refactored_test.go` 已改名为 `search_service_test.go`；文件内永远跳过的旧集成测试已删除，保留当前内存库测试和基准 |
| 标签关联处理器过渡命名 | 当前处理器已经是正式实现，`Refactored` 类型和构造函数命名已改回普通业务名，路由不变 |
| 标签计数旧字段入口 | `UpdatePodcastCount` 仍尝试写已不存在的 `podcast_count` 字段，且没有调用方；已删除旧接口和跳过测试，保留当前批量计数查询 |
| 标签关联服务跳过测试 | 已改为内存数据库真实测试，并修正通用移除方法不实际删除关联的问题；当前路由仍使用原公开移除入口 |
| 未接入的标签服务层 | `backend/internal/services/tag_service.go` 没有创建或调用入口，且与当前标签处理器和标签关联服务重复；已删除，当前接口路径不变 |
| 后端命令入口风险不清晰 | 已新增 `backend/cmd/README.md` 和 `backend/cmd/maint/README.md`，把常用入口、写库命令和人审边界分开说明 |
| 后端缓存死代码 | 已删除未被调用的缓存构造器、GetOrSet、搜索缓存失效入口和未使用的缓存键构建方法，避免误导后续维护 |
| 工作流处理器历史类型别名 | 已删除工作流响应相关别名，改为直接引用当前响应定义，接口返回保持不变 |
| 工作流报告字段重复维护 | 已把报告返回字段收口到单一内部函数，并新增测试锁定字段清单 |
| 工作流 LLM 报告数据转换重复维护 | 已把报告生成和手动重生成摘要共用的数据转换收口到同一个函数，并新增字段映射测试 |
| 工作流调试日志残留 | 已删除工作流表单里的调试输出，并把后端手动触发路径的 DEBUG 标记改为普通日志 |
| LLM 摘要提示词构造重复逻辑 | 已把默认模板和自定义模板的公共数据构造收口，并新增测试覆盖自定义提示词渲染和模板错误 |
| 前端生产调试日志 | 已把 SSE、图片重试和预取失败里的调试输出限制在开发环境，生产环境只保留错误和超时提示 |
| 启动脚本后台会话不稳定 | 已改为后台会话启动后记录真实监听进程，并在启动失败时主动清理后台会话，避免留下未记录的半启动进程 |
| 过时的工作流调度设计草案 | 原文仍按“自动调度尚未实现”的旧状态描述，并包含未完成待办标记，已移入归档；当前调度行为改由 `docs/WORKFLOW_SCHEDULER.md` 说明 |
| 分散的旧清理脚本 | 已删除 `backend/clean.sh` 和 `frontend/clean.sh`，清理入口收口到 `scripts/clean-cache.sh`；前端 `npm run clean` 继续指向新入口 |
| 过时的布局迁移和执行历史设计草案 | 布局迁移草案仍列出已完成页面为待迁移；执行历史草案仍写着无法查看详情。两者已移入归档，当前执行历史行为改由 `docs/WORKFLOW_EXECUTION_HISTORY.md` 说明 |
| 过时的项目搬家迁移指南 | 旧文档仍引用本机 iCloud 路径、旧启动命令和已失效脚本，已移入归档；当前数据库迁移入口改由 `docs/migration/MIGRATION_GUIDE.md` 说明 |
| 过时的性能优化和播客列表优化记录 | 旧性能记录仍引用 `8088`、公开域名抽查和历史轮次数据；旧播客列表计划仍列出已完成页面为待优化。两者已移入归档，当前性能入口改由 `docs/optimization/README.md` 和最新基线说明 |
| 后端高频运行日志 | 工作流列表查询、工作流保存调试信息、SSE 心跳和逐条消息、时间窗口计算、LLM 配置细节已降为调试日志；异常仍保留为警告或错误 |
| 原始基线长文混入当前入口 | `docs/BASELINE.md` 原本包含大量 2026-02-01 的失败记录和旧待办，已移入归档；当前文件只保留基线索引和最新摘要 |
| PodcastIndex 去重视图文档不一致 | 旧文档引用旧数据库名、未跟踪分析脚本和错误字段名 `lasthttpstatus`；已改为当前脚本、配置路径和 `lastHttpStatus` 字段，重复 Schema 长文已归档 |
| Cloudflare Tunnel 脚本旧端口和旧命令 | 脚本仍指向旧 `8088`，部署文档还列出脚本不支持的 `status` / `stop`；已改为当前前端 `3000`、后端健康检查 `8080/health`，并同步文档 |
| 开机自启脚本重复维护启动流程 | `scripts/production-startup.sh` 原本手动编译和启动前后端，和 `scripts/start.sh` 分叉；已改为只做健康判断并委托当前标准启动/重启入口 |
| 根目录误装 `node_modules/` | 根目录没有 `package.json`，本地存在约 40MB 被忽略依赖目录；已把它纳入 `clean-cache.sh --deep` 的 dry-run/清理范围，未在运行中直接删除 |
| 旧前端打包分析脚本 | `frontend/scripts/analyze-bundle.sh` 没有调用入口，且写死旧 Next 版本、生成零散报告并提示不存在的 Lighthouse 命令；当前性能口径已由 `scripts/performance-audit.mjs` 和基线文档承接，已删除 |
| 根目录旧 Nginx 配置 | `nginx.conf` 只被归档 Docker Compose 使用，却留在当前项目根目录；已移入 `archive/nginx.conf`，当前根目录不再暴露旧代理配置 |
| 根目录旧运行产物 | `api_server.log` 和 `api_server.pid` 是本地残留，已纳入 `clean-cache.sh --workspace` 清理范围并完成一次清理 |
| 项目内 `.DS_Store` | macOS 本地文件已纳入 `clean-cache.sh --workspace` 清理范围，并跳过依赖目录、构建目录和数据库目录 |
| 未引用源码备份文件 | `frontend/next.config.js.bak` 是旧外网代理配置副本，`backend/internal/router/router.go.bak` 是旧路由副本；二者未被跟踪、无引用，已删除 |
| 未引用的历史 Go 迁移入口 | `backend/scripts/migrations/002-004` 和 `backend/scripts/standalone/001_add_podcastindex_fields.go` 没有当前调用方，且对应字段已由当前模型和索引入口承接；已删除，保留仍在使用的 SQL 索引脚本 |
| 备份恢复演练和发布检查入口 | 已用最新备份恢复到临时库并通过验证；新增 `docs/RELEASE_CHECKLIST.md` 收口发布前固定检查步骤 |
| 自动化重构专项收尾 | 已新增 `docs/AUTOMATED_REFACTORING_CLOSEOUT.md`，汇总本轮完成范围、验证结果、剩余人审事项和下一步 |
