# Agent 验证指南

最后更新：2026-07-25

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

可选：工作区存在变更时，可运行 [.agents/skills/code-change-verification/scripts/verify.sh](../.agents/skills/code-change-verification/scripts/verify.sh) 做与变更路径绑定的基础检查。它辅助本指南，不取代 Issue 验收证据要求。

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
5. git status / git diff 仅含任务意图内的文档或治理路径；未触碰无关脏文件。
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
