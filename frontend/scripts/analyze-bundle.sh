#!/bin/bash

# 前端打包分析脚本
# 用于分析前端打包产物，建立性能基线

set -e

echo "=================================="
echo "前端打包分析脚本"
echo "=================================="
echo ""

# 检查是否在正确的目录
if [ ! -f "package.json" ]; then
    echo "错误: 请在 frontend 目录下运行此脚本"
    exit 1
fi

# 清理旧的构建
echo "1. 清理旧的构建产物..."
rm -rf .next
rm -rf out

# 执行生产构建
echo ""
echo "2. 执行生产构建..."
npm run build

# 分析构建产物
echo ""
echo "3. 分析构建产物..."

# 获取 .next 目录大小
NEXT_SIZE=$(du -sh .next 2>/dev/null | cut -f1)
echo "   .next 目录大小: $NEXT_SIZE"

# 查找所有 JS 文件
echo ""
echo "4. 分析 JavaScript 文件..."
echo "   =================================="

# 查找 largest JS chunks
find .next -name "*.js" -type f -exec du -h {} \; | sort -rh | head -20 | while read size file; do
    echo "   $size - $file"
done

# 查找所有 CSS 文件
echo ""
echo "5. 分析 CSS 文件..."
echo "   =================================="

find .next -name "*.css" -type f -exec du -h {} \; | sort -rh | head -10 | while read size file; do
    echo "   $size - $file"
done

# 统计文件数量
echo ""
echo "6. 统计文件数量..."
JS_COUNT=$(find .next -name "*.js" -type f | wc -l | tr -d ' ')
CSS_COUNT=$(find .next -name "*.css" -type f | wc -l | tr -d ' ')
echo "   JavaScript 文件: $JS_COUNT 个"
echo "   CSS 文件: $CSS_COUNT 个"

# 分析主要页面 chunks
echo ""
echo "7. 主要页面 Chunks 分析..."
echo "   =================================="

if [ -d ".next/static/chunks/pages" ]; then
    echo "   页面 chunks:"
    find .next/static/chunks/pages -name "*.js" -type f -exec du -h {} \; | sort -rh | head -10 | while read size file; do
        filename=$(basename "$file")
        echo "     $size - $filename"
    done
fi

if [ -d ".next/static/chunks" ]; then
    echo ""
    echo "   公共 chunks (前10个):"
    find .next/static/chunks -name "*.js" -type f -exec du -h {} \; | sort -rh | head -10 | while read size file; do
        filename=$(basename "$file")
        echo "     $size - $filename"
    done
fi

# 生成 Markdown 报告
echo ""
echo "8. 生成分析报告..."
REPORT_FILE="bundle_analysis_$(date +%Y%m%d_%H%M%S).md"

cat > "$REPORT_FILE" << EOF
# 前端打包分析报告

**分析时间**: $(date '+%Y-%m-%d %H:%M:%S')
**环境**: development

## 构建产物概览

- **.next 目录大小**: $NEXT_SIZE
- **JavaScript 文件数**: $JS_COUNT
- **CSS 文件数**: $CSS_COUNT

## 关键指标

### 总打包大小

\`\`\`
.next 目录: $NEXT_SIZE
\`\`\`

### 文件统计

| 文件类型 | 数量 |
|---------|------|
| JavaScript | $JS_COUNT |
| CSS | $CSS_COUNT |

## 性能基线

### 当前状态
- 构建工具: Next.js 14.2.0
- 打包模式: 生产构建
- 代码分割: 启用

### 重构目标
- **打包大小**: 减少 20%
- **首屏加载时间**: < 2s
- **Time to Interactive**: < 3s
- **Lighthouse 性能分数**: > 90

## 详细分析

### 最大的 JavaScript Chunks

EOF

find .next -name "*.js" -type f -exec du -h {} \; | sort -rh | head -20 >> "$REPORT_FILE"

cat >> "$REPORT_FILE" << EOF


### 最大的 CSS 文件

EOF

find .next -name "*.css" -type f -exec du -h {} \; | sort -rh | head -10 >> "$REPORT_FILE"

cat >> "$REPORT_FILE" << EOF


## 优化建议

1. **代码分割**: 已经启用 Next.js 的自动代码分割
2. **动态导入**: 考虑对大型组件使用 dynamic import
3. **Tree shaking**: 移除未使用的依赖
4. **图片优化**: 已使用 Next.js Image 组件
5. **CSS 优化**: 考虑使用 CSS Modules 或 Tailwind CSS 的 purge 选项

## 后续步骤

- [ ] 运行 Lighthouse 审计
- [ ] 分析 Webpack Bundle Analyzer 报告
- [ ] 优化大型组件
- [ ] 实施路由级代码分割

---

**生成时间**: $(date '+%Y-%m-%d %H:%M:%S')
EOF

echo ""
echo "   报告已保存到: $REPORT_FILE"

# 输出总结
echo ""
echo "=================================="
echo "分析完成！"
echo "=================================="
echo ""
echo "建议下一步操作:"
echo "  1. 运行 Lighthouse 审计: npm run lighthouse"
echo "  2. 安装 bundle analyzer: npm install --save-dev @next/bundle-analyzer"
echo "  3. 查看详细报告: cat $REPORT_FILE"
echo ""
