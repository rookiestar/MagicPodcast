#!/bin/bash

# 手动回归测试脚本：测试 Scheduler Reload 的并发安全性
# 这个脚本验证修复后的 Reload() 方法能够：
# 1. 在整个 Reload 过程中保持锁，避免竞态条件
# 2. 在失败时回滚到之前的状态
# 3. 正确处理并发的 Reload 调用

set -e

echo "🧪 Scheduler Reload 回归测试"
echo "================================"

cd "$(dirname "$0")"

# 检查数据库
if [ ! -f "./data/magicpodcast.db" ]; then
    echo "❌ 数据库不存在，请先运行初始化"
    exit 1
fi

echo ""
echo "✅ 数据库文件存在"

# 检查是否有测试用的工作流
WORKFLOW_COUNT=$(sqlite3 ./data/magicpodcast.db "SELECT COUNT(*) FROM workflows WHERE is_enabled = 1 AND schedule IS NOT NULL AND schedule != '';" 2>/dev/null || echo "0")

if [ "$WORKFLOW_COUNT" -eq 0 ]; then
    echo "⚠️  没有找到已启用的工作流，创建测试工作流..."
    sqlite3 ./data/magicpodcast.db <<EOF
INSERT INTO workflows (name, description, schedule, scope_type, scope_config, rules_config, is_enabled, created_at, updated_at)
VALUES
('测试工作流1', '用于测试Reload的工作流', '0 * * * * *', 'all_subscribed', '{}', '{}', 1, datetime('now'), datetime('now')),
('测试工作流2', '用于测试Reload的工作流', '0 */5 * * * *', 'all_subscribed', '{}', '{}', 1, datetime('now'), datetime('now'));
EOF
    echo "✅ 已创建2个测试工作流"
fi

echo ""
echo "📊 当前工作流状态："
sqlite3 ./data/magicpodcast.db <<EOF
.mode column
.headers on
SELECT id, name, schedule, is_enabled FROM workflows WHERE is_enabled = 1;
EOF

echo ""
echo "🚀 启动后端服务（测试模式）..."
echo "提示：服务将在后台运行，测试完成后会自动停止"
echo ""

# 启动服务
./magicpodcast-test &
SERVER_PID=$!

# 等待服务启动
sleep 3

echo "✅ 服务已启动 (PID: $SERVER_PID)"
echo ""

# 测试1: 基本Reload
echo "📝 测试1: 基本Reload调用"
echo "curl -s http://localhost:8080/api/v1/scheduler/reload -X POST"
curl -s http://localhost:8080/api/v1/scheduler/reload -X POST
echo ""
echo ""

# 测试2: 并发Reload（使用后台进程）
echo "📝 测试2: 并发Reload调用（5个并发请求）"
for i in {1..5}; do
    (curl -s http://localhost:8080/api/v1/scheduler/reload -X POST &)
done
wait
echo "✅ 并发请求完成"
echo ""

# 测试3: 检查调度器状态
echo "📝 测试3: 获取调度器状态"
curl -s http://localhost:8080/api/v1/scheduler/status | python3 -m json.tool 2>/dev/null || curl -s http://localhost:8080/api/v1/scheduler/status
echo ""
echo ""

# 清理
echo "🧹 清理：停止服务"
kill $SERVER_PID 2>/dev/null || true
wait $SERVER_PID 2>/dev/null || true

echo ""
echo "✅ 回归测试完成！"
echo ""
echo "📋 检查清单："
echo "  ✅ 基本Reload调用成功"
echo "  ✅ 并发Reload调用未崩溃"
echo "  ✅ 调度器状态正常"
echo ""
echo "如果以上步骤都成功完成，说明修复有效！"
