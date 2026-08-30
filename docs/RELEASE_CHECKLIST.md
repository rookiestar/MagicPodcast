# MagicPodcast 发布前检查清单

最后更新：2026-08-30

这份清单用于重构、依赖升级、部署前复查。目标是确认改动没有破坏现有功能、数据库可恢复、文档仍指向当前入口。

## 必跑检查

从项目根目录执行：

```bash
(cd backend && go test ./...)
(cd backend && go vet ./...)
(cd frontend && npm run type-check)
(cd frontend && npm run lint)
(cd frontend && npm run test:run)
./scripts/health-check.sh
node scripts/performance-audit.mjs --base-url http://localhost:3000 --api-url http://localhost:8080 --runs 3 --strict
```

## 可回退发布检查

生产入口必须使用：

```bash
./scripts/release.sh --dry-run
./scripts/restart.sh --prod
./scripts/release.sh --rollback
```

`restart.sh --prod` 会在停止当前服务前完成前后端构建和配对校验，并显式锁定 `production` 数据 Profile；切换后的 `/health` 必须同时返回新 `release_id`、`frontend_build_id`、`build_mode=release` 和 `data_profile=production`。发布失败时应确认旧 PID、旧 `.next` 和旧 `backend/api` 仍可恢复，并检查 `logs/release.log` 没有环境变量或凭据内容。

生产发布、回退、migration apply 和 `restore-db.sh` recovery 必须持有同一个维护窗口 `/tmp/magicpodcast-production-deploy.lock`（可由 `MAGICPODCAST_DEPLOY_LOCK_DIR` 覆盖），四种操作互斥。窗口内 `logs/supervisor.status` 应为 `state=maintenance` 并记录 owner PID、开始时间和 operation；supervisor 不得执行停启恢复。数据库写入阶段使用 `critical` 状态：即使 owner 被强杀也不自动 stale reclaim，只能由显式 recovery 在原锁目录内原子接管。迁移先消费 `release.sh --prepare` 的目标 stage，恢复则在文件验证后继续保留 `recovery_required`；只有配对 stage 启动并完成健康校验才释放锁。普通非关键 stale 窗口仍由 heartbeat、owner PID、PID 启动时间和超时兜底。不以手动卸载 supervisor 或直接删锁作为发布/恢复步骤。

远程生产发布使用 [REMOTE_PRODUCTION_DEPLOYMENT.md](REMOTE_PRODUCTION_DEPLOYMENT.md)：先由 GitHub CI 验证固定 `main` SHA，再经 `production` Environment 审批，最后由 Mac mini Runner 在固定生产目录执行 `scripts/production-deploy.sh`。Runner 注册、Environment 审批人和分支保护属于仓库外的一次性人工配置，未配置完成前不得把 workflow 视为可发布。

代码回退前还必须确认发布元数据中的 schema 版本与当前数据库一致。涉及数据库迁移时，先准备并验证迁移前备份；`release.sh --rollback` 会在停止服务前拒绝缺少配对信息或 schema 不一致的回退，不能把它当作数据库恢复命令。

非生产验证至少覆盖：

1. 临时发布目录中的后端或前端构建失败，确认当前服务仍运行。
2. PID 文件或端口对应未知工作目录/命令，确认启停脚本拒绝停止或接管。
3. 用临时发布根目录注入启动失败，确认单一步骤回退恢复上一版本并完成健康校验。

如果改动涉及页面结构、组件或样式，再补跑：

```bash
(cd frontend && npm run build)
```

## 数据相关检查

涉及真实数据库、导入、同步、迁移或维护命令前：

```bash
./scripts/backup-db.sh
./scripts/verify-db.sh backend/data/magicpodcast.db
```

恢复演练使用临时库，不覆盖当前数据库：

```bash
backup="$(ls -t backend/data/backups/*.db.gz | head -n 1)"
tmp_db="$(mktemp -t magicpodcast-restore-check)"
rm -f "$tmp_db"
DB_PATH="$tmp_db" ./scripts/restore-db.sh "$backup" --force --no-safety-backup
./scripts/verify-db.sh "$tmp_db"
rm -f "$tmp_db" "$tmp_db-wal" "$tmp_db-shm"
```

## 文档与清理检查

```bash
./scripts/clean-cache.sh --workspace --dry-run
git diff --check
```

文档链接可用下面的本地检查脚本复查：

```bash
node - <<'NODE'
const fs = require('fs');
const path = require('path');
const root = process.cwd();
const ignored = new Set(['.git', 'node_modules', '.next']);
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
for (const file of walk(root)) {
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
console.log(`checked ${walk(root).length} markdown files; local links OK`);
NODE
```

## 人审边界

这些事项不在无人值守发布检查里自动处理：

- 会改写真数据的一次性维护命令。
- 本机配置、环境变量、日志、备份和数据库文件。
- 可能改变搜索排序、通知频率或页面交互的优化。
- 需要真实使用习惯确认的旧 Docker、Nginx 或外部部署入口。
