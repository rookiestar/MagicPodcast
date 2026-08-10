# MagicPodcast 字体与排版规范

状态：现行规范
基准：B「平衡建议版」

本规范是字体角色、语义字号和富文本排版的权威入口。页面视觉仍以
[DESIGN_SYSTEM.md](DESIGN_SYSTEM.md) 为总入口。

## 1. 原则

1. 中文标题使用霞鹜文楷 Screen；纯英文标题使用 Newsreader。
2. 英文 UI 正文使用 IBM Plex Sans，元数据与代码使用 IBM Plex Mono。
3. 中文层级优先依靠字号、颜色、间距和结构；文楷只使用真实 400，不合成粗体。
4. 中文正文保留系统黑体，避免为连续阅读增加额外 CJK 字体负担。
5. 业务组件使用语义排版角色；不得继续新增无归属的字号、行高或字体栈。
6. 富文本只使用统一渲染规则；不同场景通过阅读版、报告版或紧凑版表达密度。
7. 字体资源随前端构建自托管；生产运行时不得依赖第三方字体 CDN。

## 2. 字体角色

| 角色 | CSS 变量 | 字体栈 | 用途 |
| --- | --- | --- | --- |
| 中文标题 | `--font-cjk-display` | LXGW WenKai Screen、Songti SC、STKaiti | 页面、区块、卡片和内容标题中的中文 |
| 英文标题 | `--font-latin-display` | Newsreader Variable、Georgia | 纯英文标题及标题中的拉丁字符 |
| Sans | `--font-sans` | IBM Plex Sans Variable、系统字体、PingFang SC、Microsoft YaHei | 正文、导航、按钮、表单 |
| Mono | `--font-mono` | IBM Plex Mono、系统等宽、SF Mono、Menlo | 日期、编号、统计、代码 |

`--font-serif` 是英文 Newsreader + 中文文楷组成的复合标题栈。浏览器按字形覆盖自动选择，
无需为了中英文混排拆分文本节点。

允许字重：

| 角色 | 允许值 | 说明 |
| --- | --- | --- |
| 中文标题 | 400 | 文楷无额外真实字重；全局禁止合成粗体 |
| 英文标题 | 650 | Newsreader Variable 的真实可变字重；保持预览版的克制对比 |
| Sans | 400 / 500 / 600 | IBM Plex Sans 负责拉丁字符；中文回退系统黑体 |
| Mono | 500 / 600 | IBM Plex Mono 负责拉丁字符、数字与代码 |

## 3. 语义排版角色

| 角色 | 桌面 | 移动 | 字体 / 字重 | CSS 类 |
| --- | --- | --- | --- | --- |
| 页面标题 | 22 / 1.1 | 20 / 1.1 | 文楷 400 / Newsreader 650 | `.type-page-title` |
| 区块标题 | 24 / 1.2 | 22 / 1.2 | 文楷 400 / Newsreader 650 | `.type-section-title` |
| 卡片标题 | 20 / 1.25 | 18 / 1.25 | 文楷 400 / Newsreader 650 | `.type-card-title` |
| 阅读正文 | 16 / 1.75 | 16 / 1.75 | 系统黑体 / IBM Plex Sans 400 | `.type-reading` |
| 普通正文 | 14 / 1.6 | 14 / 1.6 | 系统黑体 / IBM Plex Sans 400 | `.type-body` |
| 辅助正文 | 13 / 1.55 | 13 / 1.55 | 系统黑体 / IBM Plex Sans 400 | `.type-secondary` |
| 主导航 | 14 / 1.4 | 13 / 1.35 | 系统黑体 / IBM Plex Sans 600 | `.type-nav` |
| 标签 / 按钮 | 12 / 1.4 | 12 / 1.4 | 系统黑体 / IBM Plex Sans 600 | `.type-label` |
| 元数据 | 11 / 1.4 | 11 / 1.4 | IBM Plex Mono 600 | `.type-meta` |

表格中的“字号 / 行高”单位分别为 `px` 和无单位倍数。

字距规则：

- 中文标题使用正常字距；不得使用负字距压缩文楷。
- 纯英文编辑标题可使用 `-0.01em`–`-0.025em`。
- 大写英文 kicker 可使用 `0.05em`–`0.08em`。
- 品牌字标可保留独立字距。

## 4. 富文本

所有 HTML 与 Markdown 富文本使用 `.editorial-rich-text`，不得再使用
Tailwind Typography 的 `prose` 比例。

### 阅读版

类名：`.editorial-rich-text--reading`

- 正文：16 / 1.75
- H1：24 / 1.3 / 文楷 400、Newsreader 650
- H2：22 / 1.3 / 文楷 400、Newsreader 650
- H3：20 / 1.35 / 文楷 400、Newsreader 650
- H4：18 / 1.4 / 文楷 400、Newsreader 650
- H5–H6：16 / 1.45 / 文楷 400、Newsreader 650

适用：节目简介及其他连续阅读内容。

### 报告版

类名：`.editorial-rich-text--report`

- 正文：15 / 1.7
- H1：24 / 1.3；移动端 22 / 1.3
- H2：20 / 1.35；移动端 19 / 1.35
- H3：18 / 1.4；移动端 17 / 1.4
- H4–H6：16 / 1.45

适用：首页精选报告等兼具阅读与快速扫描的内容。报告标题保持阅读版 H1
层级，仅压缩正文及正文内二级以下标题；节目简介和其他连续长文仍使用阅读版。

### 紧凑版

类名：`.editorial-rich-text--compact`

- 正文：14 / 1.7
- H1：18 / 1.4
- H2：17 / 1.4
- H3：16 / 1.45
- H4–H6：14 / 1.5

适用：单集列表展开内容、受高度约束的预览。

三档共用链接、列表、引用、行内代码、代码块、表格、图片和深色模式规则。

## 5. 使用约束

- 新组件优先使用语义 CSS 类，不直接复制字体栈。
- 无对应语义角色时，先更新本规范和 Token，再在组件中使用。
- 文楷不允许浏览器合成粗体；标题中的拉丁字符仍可使用 Newsreader 真实字重。
- `font-bold`、`font-extrabold`、`font-black` 不得用于区分中文正文状态。
- 9px、10px 中文仅允许装饰性、非必要信息；交互与必要信息下限为 11px。
- 富文本组件允许传入布局类，但字号和标题比例由 `density` 控制。

## 6. 字体加载

- 文楷只加载简体中文 Screen 分片，使用 `unicode-range` 按页面字符请求。
- Newsreader 与 IBM Plex Sans 使用可变 WOFF2；IBM Plex Mono 只加载 500 / 600。
- 所有字体使用 `font-display: swap`，缺失字形回退到系统字体。
- 字体 CSS 在根布局静态导入，路径可分析；不在组件渲染期间动态加载。
- 字体包由前端依赖锁定并随构建产物发布。

## 7. 验证

排版变更至少核对：

1. 1440px 桌面与 390px 移动视口；
2. 中文、英文、数字混排；
3. H1–H6、段落、嵌套列表、链接、引用、代码、表格和图片；
4. 阅读版与紧凑版；
5. 深色模式；
6. 无横向溢出、标题截断或行盒裁切。
