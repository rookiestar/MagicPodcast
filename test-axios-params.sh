#!/bin/bash

echo "=== 测试 axios params 序列化 ==="
echo ""
echo "测试 1: sort_by=episode_count"
curl -s "http://localhost:8080/api/v1/podcasts?page=1&page_size=3&sort_by=episode_count" | jq '.data[] | {id, title, episode_count}' | head -10

echo ""
echo "测试 2: sort_by=title"
curl -s "http://localhost:8080/api/v1/podcasts?page=1&page_size=3&sort_by=title" | jq '.data[] | {id, title}' | head -10

echo ""
echo "测试 3: 默认排序 (recent_update)"
curl -s "http://localhost:8080/api/v1/podcasts?page=1&page_size=3" | jq '.data[] | {id, title, newest_episode_date}' | head -10
