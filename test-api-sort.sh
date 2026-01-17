#!/bin/bash

echo "=== 测试播客排序API ==="
echo ""

API="http://localhost:8080/api/v1/podcasts"

echo "1. 最近更新排序 (recent_update):"
curl -s "$API?sort_by=recent_update&page_size=3" | jq '.data[] | {id, title: .title, newest: .newest_episode_date}'
echo ""
echo "2. 最新添加排序 (newest_added):"
curl -s "$API?sort_by=newest_added&page_size=3" | jq '.data[] | {id, title: .title, added: .added_date}'
echo ""
echo "3. 单集数量排序 (episode_count):"
curl -s "$API?sort_by=episode_count&page_size=3" | jq '.data[] | {id, title: .title, count: .episode_count}'
echo ""
echo "4. 名称排序 (title):"
curl -s "$API?sort_by=title&page_size=3" | jq '.data[] | {id, title: .title}'
echo ""
echo "=== 测试完成 ==="
