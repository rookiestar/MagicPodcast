# Issue tracker：GitHub

MagicPodcast 的 PRD 和实施任务统一存放在当前仓库的 GitHub Issues 中。涉及问题跟踪的技能使用 `gh` CLI 读取和写入事项，并根据当前仓库的 Git 远程地址识别目标仓库。

## 约定

- 创建 PRD 或实施任务时，创建新的 GitHub Issue。
- 读取事项时，同时读取正文、评论和标签，避免遗漏后续形成的结论。
- 更新事项时，使用评论补充进展；除非用户明确要求，不关闭或改写作为上级来源的事项。
- 技能要求“发布到问题跟踪器”时，含义是创建 GitHub Issue。
- 外部 Pull Request 不作为需求入口，也不进入 Issue 的分类和状态流转。
- GitHub 的 Issue 和 Pull Request 共用编号空间；遇到编号歧义时，应先确认对象类型。

## 常用操作

- 创建：`gh issue create`
- 查看正文和评论：`gh issue view <number> --comments`
- 列出：`gh issue list`
- 评论：`gh issue comment <number>`
- 增减标签：`gh issue edit <number> --add-label/--remove-label`
- 关闭：`gh issue close <number>`

所有外部写入仍受 `AGENTS.md` 中的授权和范围约束。
