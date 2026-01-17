#!/bin/bash

# 测试不同的排序方式
API_URL="http://localhost:8080/api/v1/podcasts"

echo "=== 测试排序功能 ==="
echo ""
echo "1. 测试默认排序 (recent_update):"
curl -s "$API_URL?page=1&page_size=3" | jq '.data[].title' | head -3
echo ""
echo "2. 测试按最新添加排序 (newest_added):"
curl -s "$API_URL?page=1&page_size=3&sort_by=newest_added" | jq '.data[].title' | head -3
echo ""
echo "3. 测试按单集数量排序 (episode_count):"
curl -s "$API_URL?page=1&page_size=3&sort_by=episode_count" | jq '.data[].title' | head -3
echo ""
echo "4. 测试按名称排序 (title):"
curl -s "$API_URL?page=1&page_size=3&sort_by=title" | jq '.data[].title' | head -3
echo ""
echo "=== 测试完成 ==="
