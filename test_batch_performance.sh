#!/bin/bash

echo "=== 批量查询性能测试 ==="
echo ""

# 测试1: 批量查询22个播客
echo "测试1: 批量查询工作流#5的22个播客"
time curl -s -X POST http://localhost:8080/api/v1/podcasts/batch \
  -H "Content-Type: application/json" \
  -d '{"ids": [317,199,171,61,120,191,269,342,170,280,153,87,475,310,224,448,464,272,318,281,466,77]}' \
  > /dev/null

echo ""
echo "✅ 批量查询完成"
echo ""

# 测试2: 分页查询（原始方法）
echo "测试2: 原始分页查询方法（获取所有489个播客）"
time curl -s "http://localhost:8080/api/v1/podcasts?page=1&page_size=100" > /dev/null

echo ""
echo "✅ 分页查询完成"
echo ""

# 验证返回数据
echo "验证批量查询返回数据:"
curl -s -X POST http://localhost:8080/api/v1/podcasts/batch \
  -H "Content-Type: application/json" \
  -d '{"ids": [317,199,171]}' | python3 -c "
import sys, json
data = json.load(sys.stdin)
print(f\"返回播客数: {len(data['data'])} 个\")
print(f\"播客名称: {', '.join([p['title'] for p in data['data']])}\")
"

echo ""
echo "=== 测试完成 ==="
