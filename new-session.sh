#!/bin/bash

# MagicPodcast - 新会话快速启动脚本
# 用法: ./new-session.sh "功能描述"

set -e

echo "🎙️  MagicPodcast - 新会话准备工具"
echo "=================================="
echo ""

# 1. 检查 git 状态
echo "📊 检查当前状态..."
if [ -n "$(git status --porcelain)" ]; then
    echo "⚠️  检测到未提交的改动："
    git status --short
    echo ""
    read -p "是否先提交这些改动? (y/n) " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        echo "📝 请输入 commit 信息: "
        read commit_msg
        git add .
        git commit -m "$commit_msg"
        echo "✅ 改动已提交"
    fi
else
    echo "✅ 工作区干净"
fi
echo ""

# 2. 显示最近 3 条提交
echo "📜 最近的提交："
git log --oneline -3
echo ""

# 3. 显示当前项目阶段
echo "🎯 当前项目阶段："
if grep -q "阶段 1" README.md; then
    head -n 20 README.md | tail -n 10
else
    echo "查看 README.md 了解完整路线图"
fi
echo ""

# 4. 显示待办功能（从 README）
echo "📋 待完成功能（来自 README）："
grep -A 5 "### 🔄 阶段 4" README.md | grep "^\- \[ \]" || echo "无"
echo ""

# 5. 生成会话启动提示词
cat > .claude/next_session_prompt.txt <<EOF
# MagicPodcast 项目会话

## 上下文
- 项目: 个人播库管理与自动化处理工具
- 当前状态: 查看最近 3 条 git commit
- 技术栈: Go (后端) + Next.js (前端) + SQLite

## 快速了解项目
1. 查看 README.md 了解项目目标
2. 查看 CLAUDE.md 了解开发规范
3. 使用 git diff 查看最近改动

## 本会话任务
${1:-"请指定要实现的功能"}

## 工作流程
1. 探索相关代码
2. 制定实现计划
3. 编写代码
4. 本地测试
5. 提交到 git
6. 建议结束会话

---

**重要**: 完成任务后，请运行 ./new-session.sh 提交并准备下一个会话
EOF

echo "✅ 会话准备完成！"
echo ""
echo "📌 下一步："
echo "   1. 结束当前 Claude Code 会话"
echo "   2. 启动新会话"
echo "   3. 使用提示词: '$1'"
echo ""
echo "💡 提示词已保存到 .claude/next_session_prompt.txt"
echo ""
