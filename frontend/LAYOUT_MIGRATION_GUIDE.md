# 布局系统迁移指南

## 已完成的工作

✅ **核心组件已创建**：
- `Logo.tsx` - Logo组件（含多种变体）
- `AppNavbar.tsx` - 桌面端顶部导航栏
- `MobileBottomNav.tsx` - 移动端底部导航栏
- `PageToolbar.tsx` - 页面工具栏
- `PageLayout.tsx` - 页面布局容器

✅ **已迁移的页面**：
- `/podcasts` - 播客列表页
- `/podcasts/[id]` - 播客详情页

✅ **全局配置**：
- `favicon.svg` - 浏览器图标
- `layout.tsx` - 根布局已更新
- `globals.css` - 安全区域支持

---

## 剩余待迁移的页面

- `/tags` - 标签管理页
- `/workflows` - 工作流列表页
- `/workflows/[id]` - 工作流详情页
- `/import` - OPML导入页
- `/` - 首页（可选）

---

## 迁移步骤

### 1. 导入PageLayout组件

在页面顶部添加导入：

```tsx
import PageLayout from "@/components/layout/PageLayout";
```

### 2. 识别并移除旧的导航元素

查找并删除以下内容：
- 顶部的返回首页按钮
- 搜索按钮（已移至全局导航栏）
- 标题区域（将移至工具栏）

### 3. 使用PageLayout包裹内容

**原有结构**：
```tsx
export default function MyPage() {
  return (
    <main className="min-h-screen bg-slate-50">
      <div className="container mx-auto px-4 py-8">
        {/* 旧的导航按钮 */}
        <Link href="/" className="...">
          ← 返回首页
        </Link>

        {/* 页面内容 */}
        <div>{/* ... */}</div>
      </div>
    </main>
  );
}
```

**新结构**：
```tsx
export default function MyPage() {
  return (
    <PageLayout
      toolbar={{
        breadcrumbs: [{ label: "返回首页", href: "/" }],
        title: "页面标题",
        description: "页面描述（可选）",
        // 或者使用自定义内容
        leftContent: <div>自定义左侧内容</div>,
        rightContent: <div>自定义右侧内容</div>,
      }}
    >
      {/* 页面内容 */}
      <div>{/* ... */}</div>
    </PageLayout>
  );
}
```

### 4. 工具栏配置选项

#### 选项1：简单配置（推荐用于列表页）

```tsx
<PageLayout
  toolbar={{
    breadcrumbs: [{ label: "首页", href: "/" }],
    title: "我的标签",
    description: `${totalCount} 个标签`,
    rightContent: (
      <button onClick={handleCreate}>
        创建标签
      </button>
    ),
  }}
>
  {/* 内容 */}
</PageLayout>
```

#### 选项2：无工具栏（首页）

```tsx
import { SimplePageLayout } from "@/components/layout/PageLayout";

<SimplePageLayout>
  {/* 内容 */}
</SimplePageLayout>
```

#### 选项3：完全自定义

```tsx
<PageLayout
  toolbar={{
    leftContent: (
      <div className="flex items-center gap-4">
        <h1>自定义标题</h1>
        <span>描述</span>
      </div>
    ),
    rightContent: (
      <div className="flex gap-2">
        <button>操作1</button>
        <button>操作2</button>
      </div>
    ),
  }}
>
  {/* 内容 */}
</PageLayout>
```

---

## 页面特定迁移指南

### 标签页 (`/tags`)

```tsx
// 在页面顶部添加
import PageLayout from "@/components/layout/PageLayout";

// 修改返回语句
return (
  <PageLayout
    toolbar={{
      breadcrumbs: [{ label: "首页", href: "/" }],
      title: "标签管理",
      description: `${tags.length} 个标签`,
      rightContent: (
        <button
          onClick={() => setShowCreateModal(true)}
          className="px-4 py-2 bg-gradient-to-r from-violet-600 to-indigo-600 text-white rounded-lg"
        >
          创建标签
        </button>
      ),
    }}
  >
    {/* 移除旧的导航按钮 */}
    {/* 保留标签列表内容 */}
  </PageLayout>
);
```

### 工作流列表页 (`/workflows`)

```tsx
return (
  <PageLayout
    toolbar={{
      breadcrumbs: [{ label: "首页", href: "/" }],
      title: "工作流",
      description: `${workflows.length} 个工作流`,
      actions: [
        {
          label: "创建工作流",
          icon: "+",
          onClick: () => setShowCreateModal(true),
          variant: "primary",
        },
      ],
    }}
  >
    {/* 工作流列表内容 */}
  </PageLayout>
);
```

### 工作流详情页 (`/workflows/[id]`)

```tsx
// 构建返回URL
const buildBackUrl = () => {
  return "/workflows";
};

return (
  <PageLayout
    toolbar={{
      breadcrumbs: [
        { label: "工作流", href: "/workflows" },
        { label: workflow?.title || "详情" },
      ],
    }}
  >
    {/* 详情内容 */}
  </PageLayout>
);
```

### 导入页 (`/import`)

```tsx
return (
  <PageLayout
    toolbar={{
      breadcrumbs: [{ label: "首页", href: "/" }],
      title: "导入订阅",
      description: "支持OPML格式文件",
    }}
  >
    {/* 导入表单 */}
  </PageLayout>
);
```

---

## 响应式设计说明

新布局系统已包含响应式支持：

- **桌面端 (≥768px)**：
  - 顶部导航栏：64px高度，固定在顶部
  - 页面工具栏：56px高度，可选固定

- **移动端 (<768px)**：
  - 底部导航栏：60px高度，固定在底部
  - 内容区域底部自动添加padding，避免内容被遮挡

### 安全区域支持

移动端会自动处理iPhone等设备的安全区域：

```css
/* 已在 globals.css 中添加 */
.safe-area-inset-bottom {
  padding-bottom: env(safe-area-inset-bottom);
}
```

---

## 常见问题

### Q: 搜索按钮去哪了？

A: 搜索按钮已移至全局导航栏（桌面端右上角、移动端底部），通过`onSearchClick`回调处理。

### Q: 如何禁用全局导航栏？

A: 使用`showNavbar={false}`：

```tsx
<PageLayout showNavbar={false}>
  {/* 内容 */}
</PageLayout>
```

### Q: 如何自定义工具栏样式？

A: 使用`className`和`sticky`属性：

```tsx
<PageLayout
  toolbar={{
    sticky: false, // 不固定工具栏
    className: "bg-slate-100", // 自定义样式
  }}
>
  {/* 内容 */}
</PageLayout>
```

### Q: 移动端底部导航栏遮挡内容怎么办？

A: PageLayout会自动处理底部padding。如果仍有问题，检查：

1. 是否使用了`fixed`定位的元素
2. 是否使用了`vh`单位（建议使用`dvh`或`h-screen`）
3. 检查z-index是否正确

---

## 测试清单

迁移完成后，请测试以下内容：

- [ ] 桌面端：顶部导航栏显示正常
- [ ] 移动端：底部导航栏显示正常
- [ ] 返回按钮跳转正确
- [ ] 搜索功能正常
- [ ] 工具栏固定效果正常
- [ ] 页面滚动时不遮挡内容
- [ ] 响应式断点切换正常
- [ ] URL参数正确传递（如筛选条件）

---

## 样式参考

### 颜色系统
- 主色：`violet-600` → `indigo-600`（渐变）
- 文字：`slate-800`（标题）、`slate-600`（正文）、`slate-400`（次要）
- 边框：`slate-200`（默认）、`slate-300`（悬停）

### 间距系统
- 导航栏：`h-16` (64px)
- 工具栏：`h-14` (56px)
- 移动端底部导航：`h-15` (60px)
- 页面padding：`px-4 py-6` 或 `py-8`

### 圆角
- 按钮/卡片：`rounded-xl` (12px)
- 小元素：`rounded-lg` (8px)

---

## 需要帮助？

如遇到问题，请检查：
1. 是否正确导入PageLayout
2. 工具栏配置是否符合格式
3. 是否有CSS冲突（使用浏览器开发者工具检查）
4. 终端是否有报错信息

---

**文档版本**: v1.0
**最后更新**: 2026-02-01
**状态**: 核心功能已完成，剩余页面待迁移
