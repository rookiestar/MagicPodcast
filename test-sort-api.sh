#!/bin/bash

echo "=== 测试播客排序API ==="
echo ""

API="http://localhost:8080/api/v1/podcasts"

echo "1. 最近更新排序:"
curl -s "$API?sort_by=recent_update&page_size=3" | jq -r '.data[] | "\(.id) | \(.title) | \(.newest_episode_date)"'
echo ""
echo ""

echo "2. 单集数量排序:"
curl -s "$API?sort_by=episode_count&page_size=3" | jq -r '.data[] | "\(.id) | \(.title) | \(.episode_count)"'
echo ""
echo ""

echo "3. 名称排序:"
curl -s "$API?sort_by=title&page_size=3" | jq -r '.data[] | "\(.id) | \(.title)"'
echo ""
echo ""

echo "=== 如果看到上面的输出，说明排序功能正常工作 ==="
