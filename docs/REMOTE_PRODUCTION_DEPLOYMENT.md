# 远程生产发布

最后更新：2026-08-15

本方案解决“不在 Mac mini 局域网内也能安全发布”的问题。仓库负责发布逻辑；Mac mini 上的 Runner、GitHub Environment 审批人和生产目录仍需一次人工配置。本方案不会自动切换数据 Profile、执行数据库迁移或保存生产凭据。

## 发布链路

```text
main 上的固定 SHA
    ↓
GitHub CI 全部通过
    ↓
production Environment 人工批准
    ↓
Mac mini self-hosted Runner
    ↓
固定生产目录校验干净 → fetch main → checkout 固定 SHA
    ↓
release.sh --dry-run → restart.sh --prod
    ↓
/health 校验 release_id / frontend_build_id / build_mode / data_profile
```

Actions 的临时工作目录只用于拿到工作流脚本，不能承载生产数据库或运行服务。真正的发布由 `scripts/production-deploy.sh` 在固定生产目录中执行。

## 已落地的仓库能力

- `.github/workflows/ci.yml`：PR 和 `main` 合并后的 Go/前端检查。
- `.github/workflows/production-deploy.yml`：手动选择 `main` 的完整 40 位 SHA，先确认该 SHA 的 CI 已成功，再进入生产审批。
- `.github/workflows/production-rollback.yml`：单独的人工审批回退入口。
- `scripts/production-deploy.sh`：校验生产目录、拒绝脏工作树、串行锁、固定 SHA、强制 `production` Profile，并在失败时恢复代码目录。
- `scripts/release.sh`：发布和回退健康门禁同时要求 `build_mode=release`。

## Mac mini 一次性配置

### 1. 注册 Runner

在 GitHub 仓库 `Settings → Actions → Runners → New self-hosted runner` 中，按页面针对 macOS 给出的命令注册 Runner。建议使用专用、低权限的本机用户，不要让生产 Runner 接收 `pull_request` 工作流。

为 Runner 增加自定义标签：

```text
magicpodcast-production
```

工作流还会匹配 GitHub 默认的 `self-hosted` 和 `macos` 标签。

### 2. 准备固定生产目录

当前生产目录约定为：

```text
/Users/rookiestar/VSCode/Projects/MagicPodcast
```

如果实际路径不同，只修改 GitHub Environment 变量，不要把路径写进工作流。Runner 服务用户需要：

- 能读取该目录并执行 `scripts/*.sh`；
- 能通过 `origin` 只读拉取 `main` 的 Git 对象；
- 对数据库、`.env`、备份、日志和发布目录保持现有权限；
- 生产目录没有未提交的代码修改或非忽略的临时文件。

首次接入前在 Mac mini 只做检查：

```bash
cd /Users/rookiestar/VSCode/Projects/MagicPodcast
git status --short --untracked-files=all
git remote -v
git fetch --no-tags origin main
test -x scripts/release.sh
test -x scripts/restart.sh
```

若 `git status` 有输出，先人工确认，不要用 `reset --hard` 或清理命令覆盖生产文件。

### 3. 配置 GitHub Environment

创建 Environment：`production`。

Required reviewers 至少配置一名负责人，并开启部署分支限制，仅允许 `main`。添加非敏感变量：

```text
MAGICPODCAST_PRODUCTION_DIR=/Users/rookiestar/VSCode/Projects/MagicPodcast
```

不要把 SSH 私钥、`.env`、数据库路径中的凭据、Cloudflare 凭据或恢复材料写入仓库、Issue、Workflow 或日志。GitHub Actions 只申请 `contents: read` / `actions: read`；生产目录的 Git 读取权限由 Runner 用户自己的 SSH/凭据配置提供。

### 4. 加固 main

建议在分支保护中要求 `CI / Backend tests` 和 `CI / Frontend checks`，禁止直接推送，并要求生产发布必须走 `production` Environment 审批。Self-hosted Runner 不应被 PR 工作流复用。

## 日常发布

1. 等待目标 SHA 的 `CI` 成功。
2. GitHub `Actions → Production deploy → Run workflow`，选择 `main`，填写完整 40 位 SHA。
3. 审批 `production` Environment。
4. Runner 自动执行：固定 SHA 校验、`release.sh --dry-run`、`restart.sh --prod` 和本机前后端健康校验。
5. 以 `/health` 为发布证据，必须同时看到：

```text
status=ok
release_id=目标发布
frontend_build_id=当前前端构建
build_mode=release
data_profile=production
```

发布脚本不会执行数据库迁移，也不会把 fixture/snapshot 自动切成 production 以外的 Profile。迁移仍按 [迁移指南](migration/MIGRATION_GUIDE.md) 单独审批和执行。

## 回退

发现发布异常时，先停止重复发布，进入 `Actions → Production rollback`，审批后由 Runner 执行：

```text
release.sh --dry-run
release.sh --rollback
/health 配对校验
恢复生产目录到上一发布 SHA
```

`source-state.env` 从第一次受控发布开始记录代码 SHA。此前已有的生产版本没有这份代码状态，首次接入前如需回退，仍使用 Mac mini 上既有的 `./scripts/release.sh --rollback`，并按 [发布清单](RELEASE_CHECKLIST.md) 人工核对代码和 schema。

发布失败时，现有 `release.sh` 会优先自动恢复上一对前后端产物；包装脚本随后恢复生产代码目录。若代码目录恢复失败，下一次发布前必须先人工检查 `git status` 和当前服务健康状态。

## 安全边界

- 只有手动 workflow 能触发生产发布；没有 PR 自动部署。
- 固定到用户明确输入的 `main` SHA，不跟随“最新 main”漂移。
- Environment 审批和并发锁防止未经确认的重叠发布。
- CI、发布和回退都不写真实数据库；启动时强制 `data_profile=production`，避免误用本地 Profile。
- 所有发布动作最终以本机 `/health` 和前端 HTTP 响应为准，不把“Runner 命令成功”当作生产已发布。

## 尚需人工完成

本仓库无法替代以下一次性外部配置：Runner 注册与开机自启、生产 Environment 审批人、main 分支保护、Runner 用户的 Git 只读凭据，以及 Mac mini 上现有服务与备份配置的复核。当前环境也未执行生产发布。
