# MagicPodcast Agent 合同

仓库级 Agent 规则统一维护于本文件，`CLAUDE.md` 等入口仅转发。Skills 提供任务方法，服从当前用户要求与本合同，不额外要求全量预读、全量测试或代理委派，也不扩大授权。

## 1. Project Overview

MagicPodcast 是面向个人长期积累与复用的播客知识库，后端为 Go，前端为 Next.js / React，主要使用 SQLite。版本、端口、提交和当前任务状态从清单、配置、源码或跟踪系统核实。

## 2. Repository Map

| 路径 | 用途 |
| --- | --- |
| `backend/` | Go API、数据库与工作流；可执行入口在 `cmd/`，共享实现在 `internal/` |
| `frontend/src/` | 页面、组件、Hooks、客户端逻辑和测试 |
| `scripts/` | 启停、验证、数据 Profile、备份恢复、发布及性能工具 |
| `docs/` | 专题入口见 `docs/README.md`；`docs/research/`、`docs/archive/` 和 `archive/` 不代表当前生产事实 |
| `.agents/skills/`、`.github/workflows/` | 项目技能与 CI/发布门禁 |

## 3. Context

以下路径相对仓库根目录。按任务读取，复用仍有效的上下文。

| 当前任务 | 读取 |
| --- | --- |
| 本地安装、启动、环境或产品入口 | `README.md`、相关 manifest 和脚本 |
| 产品定位、推荐/发现、报告、知识处理语义或领域命名 | 全仓共享 `CONTEXT.md`；领域文档规则见 `docs/agents/domain.md` |
| 定位专题或核对当前/研究/归档资料 | `docs/README.md` |
| 重构范围与优先级 | `docs/REFACTORING_ROADMAP.md` |
| 高风险清理、升级、真实数据或待决行为取舍 | `docs/HUMAN_REVIEW_QUEUE.md`；记录不等于授权 |
| 日常验证、Issue 验收或交付 | `docs/AGENT_VERIFICATION.md`；CI 必需检查查 `.github/workflows/ci.yml` |
| Issue/Spec 操作 | 使用 `gh`，规则见 `docs/agents/issue-tracker.md`；triage 标签见 `docs/agents/triage-labels.md` |
| 性能、加载、缓存、分页、超时或重试行为 | `docs/optimization/PERFORMANCE_PLAYBOOK.md`；按需使用其中的验收模板与测试指南 |
| 发布、回退或生产健康 | `docs/RELEASE_CHECKLIST.md`、`docs/REMOTE_PRODUCTION_DEPLOYMENT.md` |
| 数据 Profile 切换或 Snapshot 刷新 | `docs/DATA_PROFILES.md` 或 `magicpodcast-data-profile` Skill |
| 数据库命令、迁移或备份恢复 | `backend/cmd/README.md`、`docs/migration/MIGRATION_GUIDE.md`、`docs/BACKUP_RECOVERY.md` |

当前实现以源码、测试及运行证据核实；已批准 Spec 说明目标，不冒充已实现行为。发现冲突先区分目标与现状，跨模块沿入口、调用链和测试查证。

## 4. Commands & Local Workflow

- 安装、启动和检查命令以 `README.md`、manifest 及当前脚本为准，不从普通命令推断生产或真实数据操作授权。
- 数据敏感的启动或验证前运行 `./scripts/data-profile.sh status`，确认使用的 Profile 和数据目标。
- 先核对仓库、分支/工作树、既有改动、任务范围和验收条件；保留已有工作、配置、数据及运行产物。

## 5. Verification Guide

- 按 `docs/AGENT_VERIFICATION.md` 选择必需检查和最小充分证据，每条验收要求对应观察结果；用户可见行为须检查已授权路径上的实际页面、API 或产物。
- 文档改动核对链接、关键陈述和 `git diff --check`，确认本任务差异不含产品源码；任务外已有改动单独辨认。验证脚本可选，不因整个工作区有其他改动而扩大本任务检查。
- 已有证据仍覆盖当前实现和环境时复用；仅因新改动、失败、证据失效或具体未解决风险重跑。跳过、失败、部分执行及本地/CI/生产证据分别如实报告。
- 交付时按验证指南 §7 复用环境、管理长任务与检查合并门禁；保留严格 base 保护、必需检查及已核对 head 的绑定要求。生产验收仍走发布清单。

## 6. Change Workflow

- 实施只覆盖当前需求及必要回归；完成已授权的实现、验证和本任务引入的失败修复后交付。评估或规划请求以相应结论为止。
- 复用现有模块，采用最小充分方案。文件数量本身不触发确认；超出已确认范围的抽象、配置或选项先说明必要性和最小替代方案，再对齐。质量目标不自动纳入未来扩展。
- 兼容层、迁移垫片、legacy fallback 和旧数据回填只在当前需求或已确认合同依据要求时加入；不默认授权破坏性变更。
- 新增 hash/指纹、validator、契约副本或 gate 须有具体失败场景或真实信任边界依据，以及现有机制的缺口；同一信任边界内复用校验并保持单一事实来源。不把需求判断机械化为规则引擎，也不额外要求论证文档或审批流程。
- 用户目标、偏好和已知取舍指导范围；事实归因与方案建议仍结合证据审查，有实质异议直接说明。常规实现自行决定；仅在必要信息或新增授权缺失时请求决策，先完成独立工作。
- 排障先查指定系统和原访问路径，证据指向相邻工具时再延伸；用最小变更复验原因，验收和必要回归完成后收口。
- 按 Issue 的实际依赖实施；要求关闭时先核对并关闭已满足的子票，再复核父票，否则报告可关闭状态。交付前复读本任务最终 diff 和工作区状态。

## 7. Code Style Patterns

- Go 修改使用 `gofmt`；前端延续 App Router、相邻代码及现有测试结构。
- ESLint、TypeScript 和当前测试配置为准；不夹带无关重构、全局格式化或放宽门禁。

## 8. Boundaries & Guardrails

默认允许任务范围内的只读查证、本地可逆编辑及相称验证。以下操作须有对应目标和范围的明确授权；同一授权不重复询问，队列记录和检查通过不代替授权。

| 类别 | 需授权的操作 |
| --- | --- |
| Git | commit、push、force-push、amend 已发布提交、删除分支、修改配置、破坏性重置或清理 |
| 生产 | 部署、回退、改配置、启停服务、切换版本 |
| 真实数据库 | migrate apply、`cmd/maint/*`、写 SQL、覆盖恢复、schema 变更 |
| 行为与依赖 | 未明确要求的产品/API/搜索/缓存/通知/部署语义变化或跨主版本升级 |
| 敏感与本地态 | 删除配置、数据库、日志、备份、凭据或运行产物 |
| 代理委派 | Subagent、并行代理或委派核心实施；“可并行”仅授权对应范围 |

- 遵循用户指定的 SSH/CLI/API；Computer Use、屏幕控制或合成点击/按键仅在当前任务明确要求 UI 操作时使用。指定路径不可用先报告，不通过 UI 旁路。
- 普通 API 启动仅做只读 schema 校验，不自动迁移；当前版本查 `backend/internal/database/migrate.go` 的 `CurrentSchemaVersion`。
- 生产写入按专项流程核实备份、停服或维护窗口、版本/schema 配对及回退路径；破坏性操作先确认精确目标、影响和可恢复性。
- 保护任务外改动；不提交密钥、真实配置、数据库、日志、备份及其他敏感文件。
- 性能优化保留有效内容和核心交互，不以空白、错误态或可用性退化换取更快响应。正常、慢、失败、首次访问是相关行为的验收依据；新增体验取舍或超出已授权范围才再次确认。性能结论同时说明响应与有效内容出现时间。

## 9. Related Documentation

专题从 `docs/README.md` 进入，按 §3 路由读取。简洁中文报告结果、验证及剩余限制；区分事实、推断和未知，不将本地通过写成生产已验证。
