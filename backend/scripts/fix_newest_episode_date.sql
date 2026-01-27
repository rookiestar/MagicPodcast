-- 修复脚本：更新所有播客的newest_episode_date字段
-- 该脚本会重新计算每个播客的最新单集发布日期

-- 查看需要修复的播客数量（应与之前查询结果一致）
SELECT 
  COUNT(*) as affected_podcasts
FROM podcasts p
WHERE EXISTS (
  SELECT 1 
  FROM episodes e 
  WHERE e.podcast_id = p.id 
    AND COALESCE(e.updated_date, e.published_date) > p.newest_episode_date
);

-- 显示需要修复的前10个播客
SELECT 
  p.id,
  p.title,
  p.newest_episode_date as current_newest_episode,
  MAX(COALESCE(e.updated_date, e.published_date)) as actual_newest_episode,
  STRFTIME('%Y-%m-%d', MAX(COALESCE(e.updated_date, e.published_date))) as formatted_date,
  p.episode_count
FROM podcasts p
LEFT JOIN episodes e ON e.podcast_id = p.id
GROUP BY p.id
HAVING MAX(COALESCE(e.updated_date, e.published_date)) > p.newest_episode_date
ORDER BY MAX(COALESCE(e.updated_date, e.published_date)) DESC
LIMIT 10;

-- 执行修复（更新所有播客的newest_episode_date）
-- 注意：执行前请先备份数据库！
UPDATE podcasts 
SET newest_episode_date = (
  SELECT COALESCE(MAX(updated_date), MAX(published_date))
  FROM episodes
  WHERE episodes.podcast_id = podcasts.id
)
WHERE id IN (
  SELECT DISTINCT p.id
  FROM podcasts p
  INNER JOIN episodes e ON e.podcast_id = p.id
  WHERE COALESCE(e.updated_date, e.published_date) > p.newest_episode_date
);

-- 验证修复结果
SELECT 
  '修复后的统计' as message,
  COUNT(*) as updated_podcasts
FROM podcasts p
WHERE EXISTS (
  SELECT 1 
  FROM episodes e 
  WHERE e.podcast_id = p.id 
    AND COALESCE(e.updated_date, e.published_date) > p.newest_episode_date
);
-- 如果结果为0，说明所有播客的newest_episode_date都已正确更新
