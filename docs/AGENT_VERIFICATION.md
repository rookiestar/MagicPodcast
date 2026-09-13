# Agent 验证指南

最后更新：2026-09-07

本指南是 Agent **日常验证与 Issue 验收**的权威入口，由根目录 [AGENTS.md](../AGENTS.md) 引用。

- **本文件负责**：按风险做定向检查；把完成标准落到可观察证据；区分源码、自动化测试、运行态加载与用户可见产物。
- **本文件不负责**：生产部署、回退、发布配对与生产健康门禁 → 见 [RELEASE_CHECKLIST.md](RELEASE_CHECKLIST.md)。

普通改动不要把发布清单里的部署/回退步骤当作默认验证入口。

## 1. 风险与检查范围

按改动触及的路径选择最小充分集合；能定向则不必默认全量。

| 改动类型 | 最低建议检查 |
| --- | --- |
| 仅治理/文档（`AGENTS.md`、`docs/**`、转发用 `CLAUDE.md` 等） | 本地 Markdown 链接；关键陈述与源码/已批准决策一致；`git diff --check`；确认无产品源码 diff |
| 后端 Go | `(cd backend && go test ./...)` 中与包相关的测试；必要时 `go vet`；涉及行为时补相关包测试 |
| 前端 | `(cd frontend && npm run type-check)`；`(cd frontend && npm run test:run)` 或定向测试 |
| 脚本（`scripts/*.sh` 等） | `bash -n` 语法检查；按脚本用途做 dry-run（若支持） |
| 性能、加载、缓存、分页、超时或重试行为 | 先读 [性能专项工作手册](optimization/PERFORMANCE_PLAYBOOK.md)；用 [验收模板](optimization/PERFORMANCE_ACCEPTANCE_TEMPLATE.md) 证明体验不变量、正常/慢/失败/首次访问和有效内容指标；命令见 [性能测试指南](PERFORMANCE_TESTING_GUIDE.md) |
| 性能脚本 / 启动路径 | 仅在改动触及启动、健康检查或性能脚本时：健康检查与 [performance/](performance/) 基线中的复跑命令 |
| 数据库迁移相关文档或迁移代码 | 对照 `CurrentSchemaVersion` 与注册表；真实 `--apply` **不在**日常验证范围，需单独授权并走迁移指南 |

可选：[verify.sh](../.agents/skills/code-change-verification/scripts/verify.sh) 会收集整个工作区已暂存、未暂存和未跟踪的路径；触及后端时运行整端 Go 测试与 vet，触及前端时运行类型检查与整端测试。先区分本任务与既有改动，再决定是否使用。脚本存在不代表每次修改都必须运行；定向检查足够时直接执行定向检查。

必需检查不能以定向通过替代。已有结果仍覆盖当前实现、依赖与相关环境时可复用；新改动后重跑受影响检查，存在失败或未解决风险时再扩大。环境缺失、既有失败与本任务引入的失败分别记录，避免无条件重复全量验证。

## 2. Issue 验收的证据分层

每条验收标准至少落在一层可观察证据上；高层声称不能替代底层证明。

| 层级 | 含义 | 示例证据 |
| --- | --- | --- |
| A. 源码 / 文档源 | 仓库内文件内容支持该条 | 文件路径 + 关键片段；`grep`/`git diff` 摘录 |
| B. 自动化测试 | 测试命令针对**已交付**实现 | 测试命令与通过输出；禁止硬编码假通过或绕过被测入口 |
| C. 运行态加载 | 进程/配置/schema 实际加载了预期版本或路径 | 健康检查输出、只读 schema 状态、启动日志中的非敏感字段 |
| D. 用户可见产物 | 界面、API 响应、生成物对用户可见 | 截图说明、HTTP 响应摘要、导出文件 |

规则：

1. 文档-only 任务通常 A + 链接/格式检查即可；不得假装跑了应用测试。
2. 行为改动至少要有 B，或在无法自动测时明确写出缺口与替代的 A/C 证据。
3. 「本地测试通过」不等于「生产已验证」；生产结论必须来自发布清单下的独立授权操作。
4. 每条 AC 在 Issue 评论或收口记录中应能指到观察结果；纯散文总结不够。

## 3. 文档与治理改动的固定检查

```text
1. 从 CLAUDE.md（若存在）应能到达 AGENTS.md，且 CLAUDE.md 不再平行复述规则。
2. AGENTS.md 引用的路径在主线存在（含 docs/ 与 .agents/ 下被点名的文件）。
3. 本地 Markdown 相对链接可解析（可用下方脚本或发布清单中的等价脚本）。
4. 主线入口无第二套并列 Agent 合同。
5. 本任务新增的 diff 仅含预期文档或治理路径；git status / git diff 中已有的无关改动保持不动。
6. git diff --check 无空白错误。
```

链接检查示例（在仓库根目录执行）：

```bash
node - <<'NODE'
const fs = require('fs');
const path = require('path');
const root = process.cwd();
const ignored = new Set(['.git', 'node_modules', '.next', 'archive', 'docs/archive']);
function walk(dir, files = []) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    if (ignored.has(entry.name)) continue;
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) walk(full, files);
    else if (entry.isFile() && entry.name.endsWith('.md')) files.push(full);
  }
  return files;
}
const missing = [];
const files = walk(root);
for (const file of files) {
  const text = fs.readFileSync(file, 'utf8');
  const linkRe = /\[[^\]\n]+\]\((?!https?:\/\/|mailto:|#)([^)]+)\)/g;
  for (const match of text.matchAll(linkRe)) {
    const target = match[1].trim().split('#')[0];
    if (!target || target.startsWith('<') || target.startsWith('app://')) continue;
    const resolved = path.resolve(path.dirname(file), decodeURI(target));
    if (!fs.existsSync(resolved)) missing.push(`${path.relative(root, file)} -> ${target}`);
  }
}
if (missing.length) {
  console.error(missing.join('\n'));
  process.exit(1);
}
console.log(`checked ${files.length} markdown files; local links OK`);
NODE
```

说明：完整发布前的全库链接检查仍以 [RELEASE_CHECKLIST.md](RELEASE_CHECKLIST.md) 为准；上例可对主线文档做较快复查。按需缩小 `walk` 根目录到本次改动涉及的路径。

## 4. 与发布清单的边界

| 场景 | 用哪份文档 |
| --- | --- |
| 功能/文档改动是否可合并或可关闭 Issue | 本指南 + [AGENTS.md](../AGENTS.md) |
| 是否可部署、回退、确认生产健康与发布元数据 | [RELEASE_CHECKLIST.md](RELEASE_CHECKLIST.md) |
| 是否可对真实数据库 migrate/restore | [migration/MIGRATION_GUIDE.md](migration/MIGRATION_GUIDE.md)、[BACKUP_RECOVERY.md](BACKUP_RECOVERY.md)，且需明确授权 |

## 5. 汇报时如何写验证

简洁中文列出：

1. 执行了哪些命令或检查；
2. 每条 AC 对应的证据层级（A/B/C/D）与结果；
3. 明确跳过的项及原因；
4. 剩余风险（未跑的生产步骤、人审项、环境限制）。

## 6. 任务提示与交接

任务提示只补充本次目标、范围、验收和授权，不复制整份 Agent 合同。跨模型共用相同的验收与安全边界；复杂任务或已出现遗漏时，补充必要入口、依赖顺序和一个能区分正确/错误结果的例子，而非固定增加流程。

```text
目标：……
范围与依赖：……
完成标准：……（可观察的行为与结果）
授权范围：……（未列出的操作遵循 AGENTS.md）

按项目合同完成本次请求。实施任务包括实现、相关验证及任务引入的失败修复；
评估或规划任务以方案为止。常规实现选择自行决定，验收完成后停止。
需要必要信息或新增授权时，先完成不依赖该决定的工作，再提出具体问题。
最终简短报告结果、验证和剩余限制。
```

交接时补充已完成事项及证据、尚未解决的问题、当前工作树与用户改动、下一步和授权边界。易变状态注明观察时点，接手后只刷新下一步依赖的事实，不重复已经有效完成的工作。


## 7. 交付耗时

以用户请求到验收结束的墙钟时间衡量效率；CI 用时只是其中一段。沿用当前任务输出记录检查结果、对应提交或未提交差异及环境，不新增证据账本。

- **短命令一次完成**：状态、diff、GitHub 查询默认等待 10 秒；预计稍长的命令等待 30 秒。仅真正长任务返回后台会话；完整保留任务名、session_id、exit_code 和所需输出。批量调用逐项检查结果，不仅提取 output。仅轮询已返回的有效 ID，完成后立即进入下一步。
- **合并独立读取**：仓库状态、必要文档及 GitHub 检查/审查信息批量读取。可并行的轻量检查一起执行；构建、整端测试及进程探测争用资源时串行。工具并行不等于代理委派。
- **默认非登录 shell**：已能解析命令时使用 login=false；需要 NVM 等环境时显式加载一次所需环境，避免每次命令运行交互初始化。
- **就地复用环境**：同一任务沿用干净隔离工作树。安装前检查依赖是否存在、锁文件是否变化和 node_modules 是否为软链接；新工作树按锁文件安装，复用包管理器下载缓存。跨工作树不共享可写 node_modules。开发与生产构建不同时写同一 .next；只停止本任务拥有的临时服务。
- **验证一次并复用**：小改动先按 §1 定向验证；用户可见改动同时检查实际页面。已通过的结果仍覆盖当前差异及环境时，不因用户接着说 commit、push 或部署就重跑。修改后只重跑受影响检查；完整 CI 继续由现有工作流执行。发布证据复用见发布清单。
- **门禁一次读全**：PR 检查与 review threads 同时读取；BLOCKED 先查明确的未满足条件。任务仍运行时使用一个 watch，避免同时再开重复轮询；watch 结束后、合并或收口前批量刷新 PR 的 head/base、对应检查和 review threads，覆盖等待期间的提交、基线及审查变化；发生变化时重新判断受影响证据。合并请求绑定已核对的 head SHA（gh pr merge --match-head-commit 或 merge API 的 sha），拒绝把核对后的新提交一并合入。API 超时先查询操作结果，只有确认未完成后才决定重试；重跑仅针对已确认的基础设施失败 job。
- **收口即停止**：授权范围内的交付与验收齐全即结束。失败先定位到环境、实现或外部服务，不通过连续全量重跑求绿。Git、发布与真实数据操作继续遵循已有授权边界。

阶段性 PR 仅使用 Refs 关联尚未验收完的工单；全部验收满足后再关闭，避免规则合并自动关闭仍待真实样本的专项。

在交付摘要中按需区分本地实现/检查、CI 排队/执行、审查处理、发布审批/构建/切换与验收。比较同类任务；没有前后对照时只报告已消除的步骤与实际用时，不承诺提速比例。
