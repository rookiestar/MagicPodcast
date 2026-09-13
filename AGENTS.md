# MagicPodcast Agent 合同

本文件是仓库级 **唯一权威** Agent 合同。所有编码代理以本文件为准；`CLAUDE.md` 等入口只转发到这里，不平行维护第二套规则。

本文件仅保存长期稳定的操作方式与安全边界。依赖版本、commit、端口、当前 Issue、一次性统计等易变事实，必须从当前清单、配置、源码、测试、运行结果或跟踪系统核实。

Skills 提供任务方法，遵循当前用户要求与本合同；不把全量预读、全量测试或代理委派设为额外默认步骤，也不扩大操作授权。

## Agent skills

### Issue tracker

本仓库使用 GitHub Issues 管理 Issue 与 Spec，统一使用 `gh` CLI。见 `docs/agents/issue-tracker.md`。

### Triage labels

本仓库使用五个默认 triage 标签：`needs-triage`、`needs-info`、`ready-for-agent`、`ready-for-human`、`wontfix`。见 `docs/agents/triage-labels.md`。

### Domain docs

本仓库采用 single-context 领域文档布局。见 `docs/agents/domain.md`。

## 1. Project Overview

MagicPodcast 是面向个人长期积累与复用的播客知识库，后端为 Go 服务，前端为 Next.js / React 应用，数据主要存储于 SQLite。

- 项目定位、当前功能和本地入口：`README.md`。
- 产品术语与语义边界：`CONTEXT.md`。
- 精确版本：`backend/go.mod`、`frontend/package.json`、lockfile 和当前源码。

## 2. Repository Map

| 路径 | 用途 |
| --- | --- |
| `backend/` | Go API、数据库、工作流和命令；命令风险见 `backend/cmd/README.md` |
| `frontend/src/` | 页面、组件、Hooks、客户端逻辑和测试 |
| `scripts/` | 启停、验证、数据 Profile、备份恢复、发布和性能工具 |
| `docs/` | 当前维护文档；专题入口见 `docs/README.md` |
| `docs/research/`、`docs/archive/`、`archive/` | 研究/方案与历史材料，不是默认生产事实 |
| `.agents/skills/`、`.github/workflows/` | 项目 Agent 工作流与 CI/发布门禁 |

## 3. Context

只使用仓库相对路径；按任务触发读取，不固定全量预读。

| 触发条件 | 先读 |
| --- | --- |
| 本地安装、启动或环境检查 | `README.md` |
| 产品定位、推荐/发现、报告或知识处理语义 | `CONTEXT.md` |
| 定位专题文档或判断当前/研究/归档 | `docs/README.md` |
| 重构范围与优先级 | `docs/REFACTORING_ROADMAP.md` |
| 高风险清理、升级、真实数据、缓存/搜索/通知语义 | `docs/HUMAN_REVIEW_QUEUE.md` |
| 依赖、构建、测试或 CI 门禁 | `.github/workflows/ci.yml` |

文档冲突时，以当前源码、测试和观察到的运行结果为准。跨模块修改须沿入口、调用链和测试建立事实；研究、Spec 和归档内容复用前必须重新验证。人审队列记录不等于操作授权。

## 4. Commands & Local Workflow

- 安装、开发和统一启动命令以 `README.md`、manifest 和当前脚本为准。
- CI 必跑命令以 `.github/workflows/ci.yml` 为准；优先运行相关包或测试，再按风险扩大。
- 数据敏感的启动或验证前先运行 `./scripts/data-profile.sh status`。切换 Fixture / Snapshot 前读 `docs/DATA_PROFILES.md` 或使用 `magicpodcast-data-profile` Skill。
- 性能、发布、迁移和生产检查走 §9 的专项入口，不从普通命令推断。

## 5. Verification Guide

日常改动和 Issue 验收以 `docs/AGENT_VERIFICATION.md` 为准；发布、回退和生产健康以 `docs/RELEASE_CHECKLIST.md` 为准。本地通过不等于生产已验证。

1. 每条验收标准映射到可观察证据。
2. 完成必需检查与最小充分的验收验证；用户可见行为还要通过已授权路径检查真实页面、API 或产物。已有通过结果仍覆盖当前实现和相关环境时可复用；有新改动、失败或未解决风险时再定向重跑或扩大。
3. 文档改动检查相对链接、关键陈述、`git diff --check`，并确认本任务没有产品源码 diff；区分任务外已有改动。
4. 最终复核 `git status` 和 `git diff`；跳过、失败或部分执行的检查必须如实记录。
5. `.agents/skills/code-change-verification/scripts/verify.sh` 是可选辅助；它按整个工作区变更触发整端测试，使用前确认范围适合本任务，不替代验收证据或操作授权。

完成标准：验收证据齐全、验证与风险成比例、未发生未授权写操作、剩余限制已明确。最终用简洁中文报告**改动、验证、状态、风险**，不把推断或未执行操作写成事实。

实施请求应在授权范围内持续完成实现、验证与任务引入的失败修复，不在第一版完成时提前交回。仅评估或规划的请求以交付评估或方案为止。常规实现选择自行决定；缺少必要信息或新增授权时，先完成不依赖该决定的工作，再提出具体问题。任务提示模板见 `docs/AGENT_VERIFICATION.md` §6。

交付耗时与工具执行遵循 `docs/AGENT_VERIFICATION.md` §7；优先减少重复验证、工具往返与环境返工。

## 6. Change Workflow

1. **Preflight**：确认真实仓库、分支/工作树、既有脏文件、具体目标与期望/故障方向。
2. **Route**：按 §3 读取上下文，确认并按用户指定访问路径执行，明确边界、依赖和验收标准；最新纠正立即覆盖旧假设。指定路径不可用或外部目标仍有实质歧义时，先说明并问一个短问题。
3. **Scope**：选择最小必要切片，不顺手重构或清理。若需新增抽象层、配置面、新增文件达到约 5 个以上或推测性选项，实施前分列“最小方案/附加项”，默认最小；用户指出过度设计时直接删减。
4. **Implement**：沿用现有模块，只实现当前明确需求。兼容层、迁移垫片、legacy fallback 和旧数据回填仅在当前需求或已确认合同依据要求时加入；质量词（如“完美/长远/通用”）不扩大授权，未来设想只在汇报中一行说明；这不等于默认授权破坏性变更。文档只写源码、测试或已批准决策支持的事实。
5. **最小充分防御**：复用现有契约、Fixture/Snapshot、测试和门禁。同一信任边界内同一事实只校验一次并保持单一账本。新增校验、指纹、契约副本或 gate 前，在任务记录中说明具体失败场景或可信边界要求、已检查的机制及其缺口、新机制为何最小充分；可由指令或人审处理的判断留在审查层。严格处理真实错误不单独授权新增检查或兼容层。
6. **Verify**：按 §5 验证；排障先查用户点名系统，仅在证据指向相邻工具时延伸。每次用最小变更检验原因；原症状与本任务全部验收通过后停止扩展。
7. **Review**：复读最终 diff；外部写入按 §8 单独授权。

Issue 存在父子或 Blocked-by 时，严格按依赖顺序实施；每条验收标准须有证据。任务要求关闭时先关闭已满足的子票，再复核父票；未要求时只报告可关闭状态。

新增文档引用前确认路径存在。

## 7. Code Style Patterns

- Go：修改文件使用 `gofmt`；可执行入口放在 `backend/cmd/`，共享实现放在 `backend/internal/`。
- 前端：延续 App Router 和相邻代码；ESLint、TypeScript 配置及现有测试结构为准。
- 不在无关任务中重排结构、全局格式化、放宽静态检查或测试门禁。

## 8. Boundaries & Guardrails

### Always do

- 默认允许只读查证、任务范围内的本地可逆编辑和与改动成比例的本地验证。
- 保护用户已有脏文件、未跟踪文件、本地配置、数据和运行产物。
- 区分已验证事实、合理推断、判断和未知项；高风险事项先查或登记 `docs/HUMAN_REVIEW_QUEUE.md`。
- 性能、加载、缓存、分页、超时或重试任务开始前，读取 `docs/optimization/PERFORMANCE_PLAYBOOK.md`；方案与收口使用验收模板和测试指南。

### Ask first

除非用户已在当前任务明确授权，否则执行前必须确认：

同一目标、范围与操作已获明确授权时不重复询问；授权不延伸到表中的其他操作或目标。若 skill 要求额外停工，指出具体条款，并先判断现有授权是否已满足它。

| 类别 | 操作 |
| --- | --- |
| Git | commit、push、force-push、amend 已发布提交、删除分支、修改配置、破坏性重置或清理 |
| 生产 | 部署、回退、改配置、启停服务、切换版本 |
| 真实数据库 | migrate apply、`cmd/maint/*`、写 SQL、覆盖恢复、schema 变更 |
| 行为与依赖 | 未明确要求的产品/API/搜索/缓存/通知/部署语义变化，或跨主版本升级 |
| 访问路径与桌面 UI | 用户指定 SSH / CLI / API 时按该路径；Computer Use、屏幕控制或合成点击/按键仅在当前任务明确要求 UI 操作时使用 |
| 敏感与本地态 | 删除配置、数据库、日志、备份、凭据或运行产物 |
| 代理委派 | 启动 Subagent、并行代理或委派核心实施；“可并行”仅授权对应范围 |

真实生产写通常还要求：备份已验证、服务已停或处于维护窗、schema/发布元数据可配对、回退路径就绪。破坏性操作须先确认精确目标、影响和可恢复路径。获准提交后，提交信息说明目的，范围不得超出授权。

不得以减少首屏有效内容、展示空白/错误态、延后核心交互或降低可用性换取响应更快。凡改动加载态、空态、错误态、超时、缓存、陈旧数据或重试策略，实施前必须列出正常、慢请求、失败、首次访问四种用户可见结果并取得确认；验收同时证明响应时间与有效内容出现时间。

### Never do

- 不清理、覆盖或回退任务外的用户改动。
- 不把人审队列记录当作授权。
- 不让普通 API 启动自动迁移；启动只做只读校验，当前版本从 `backend/internal/database/migrate.go` 的 `CurrentSchemaVersion` 读取。
- 不把归档或未落地 Spec 写成当前生产事实，不编造验证或生产结论。
- 不提交密钥、真实配置、数据库、日志、备份或其他敏感文件。

## 9. Related Documentation

- 文档总索引：`docs/README.md`。
- 日常验证：`docs/AGENT_VERIFICATION.md`。
- 发布与生产：`docs/RELEASE_CHECKLIST.md`、`docs/REMOTE_PRODUCTION_DEPLOYMENT.md`。
- 数据库与备份：`docs/migration/MIGRATION_GUIDE.md`、`docs/BACKUP_RECOVERY.md`。
- 性能：`docs/optimization/PERFORMANCE_PLAYBOOK.md`、`docs/optimization/PERFORMANCE_ACCEPTANCE_TEMPLATE.md`、`docs/PERFORMANCE_TESTING_GUIDE.md`。

其他专题从 `docs/README.md` 进入，不在本文件复制长文。
