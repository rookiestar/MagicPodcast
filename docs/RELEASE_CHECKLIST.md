# MagicPodcast 发布前检查清单

最后更新：2026-06-01

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
