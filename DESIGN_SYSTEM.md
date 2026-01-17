# MagicPodcast 设计系统规范

## 🎨 核心设计原则

### **极简主义**
- 干净的布局，充足的留白
- 无装饰性动画，只保留功能性过渡
- 扁平化设计风格

### **色彩系统**

#### 主色调
- **背景色**: `bg-slate-50` (浅灰)
- **主文字**: `text-slate-800` (深灰)
- **次要文字**: `text-slate-600` (中灰)
- **辅助文字**: `text-slate-400` (浅灰)

#### 强调色
- **主渐变**: `from-violet-600 to-indigo-600` (紫蓝渐变)
- 用于：标题、重点标识

#### 中性色
- **卡片背景**: `bg-white`
- **边框**: `border-slate-200`
- **hover背景**: `bg-slate-50`
- **hover边框**: `border-slate-400`

### **排版系统**

#### 标题层级
- **H1**: `text-6xl md:text-7xl font-bold`
- **H2**: `text-4xl md:text-5xl font-bold`
- **H3**: `text-xl font-semibold`

#### 字间距
- **标题**: `letter-spacing: -0.02em`
- **正文**: 默认

### **组件规范**

#### 按钮（标准）
```
className="px-6 py-3 bg-white text-slate-800 font-medium rounded-lg
           border border-slate-300 hover:bg-slate-50 hover:border-slate-400
           transition-colors"
```

#### 卡片（标准）
```
className="bg-white rounded-xl shadow-sm hover:shadow-md transition-shadow
           p-8 border border-slate-200"
```

#### 输入框
```
className="w-full px-4 py-3 bg-white border border-slate-300 rounded-lg
           focus:outline-none focus:ring-2 focus:ring-violet-500 focus:border-transparent"
```

### **间距系统**

- **容器内边距**: `py-20`
- **section间距**: `mb-16`
- **卡片间距**: `gap-6`

### **阴影层级**

- **卡片默认**: `shadow-sm`
- **卡片 hover**: `shadow-md`
- **模态框**: `shadow-lg`

### **过渡动画**

只保留功能性过渡，移除所有装饰性动画：
- `transition-colors` - 颜色过渡
- `transition-shadow` - 阴影过渡
- ❌ 禁用：`animate-bounce`, `animate-pulse`, `animate-spin` 等

---

## 📋 页面优化清单

- [x] 首页（已完成）
- [ ] 播客列表页 (`/podcasts`)
- [ ] 播客详情页 (`/podcasts/[id]`)
- [ ] 标签管理页 (`/tags`)
- [ ] 工作流列表页 (`/workflows`)
- [ ] 工作流详情页 (`/workflows/[id]`)
- [ ] 导入/同步页 (`/import`)

---

## 🎯 实施顺序

按照优先级逐个优化，每完成一个页面就提交一次 git。
