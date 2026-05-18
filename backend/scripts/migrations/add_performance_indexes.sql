-- ============================================
-- MagicPodcast 性能优化索引
-- 版本: 1.0
-- 创建时间: 2026-01-30
-- ============================================

-- 1. podcasts 表优化
-- ===================

-- 订阅状态查询
CREATE INDEX IF NOT EXISTS idx_podcasts_is_subscribed
ON podcasts(is_subscribed)
WHERE is_subscribed = true;

-- 最新单集日期排序
CREATE INDEX IF NOT EXISTS idx_podcasts_newest_episode_date_desc
ON podcasts(newest_episode_date DESC);

-- 列表默认排序：最近更新
CREATE INDEX IF NOT EXISTS idx_podcasts_recent_update_desc
ON podcasts(COALESCE(newest_episode_date, created_at) DESC, id DESC);

-- 最新添加排序
CREATE INDEX IF NOT EXISTS idx_podcasts_added_date_desc
ON podcasts(added_date DESC, id DESC);

-- 单集数量排序
CREATE INDEX IF NOT EXISTS idx_podcasts_episode_count_desc
ON podcasts(episode_count DESC, id DESC);

-- 名称排序
CREATE INDEX IF NOT EXISTS idx_podcasts_title_nocase
ON podcasts(title COLLATE NOCASE ASC, id ASC);

-- Feed抓取优先级
CREATE INDEX IF NOT EXISTS idx_podcasts_priority_dead
ON podcasts(priority, is_dead)
WHERE is_dead = false;

-- 最后抓取时间
CREATE INDEX IF NOT EXISTS idx_podcasts_last_fetched_at
ON podcasts(last_fetched_at DESC)
WHERE last_fetched_at IS NOT NULL;

-- 数据源过滤
CREATE INDEX IF NOT EXISTS idx_podcasts_data_source
ON podcasts(data_source);

-- 失效标记
CREATE INDEX IF NOT EXISTS idx_podcasts_is_dead
ON podcasts(is_dead)
WHERE is_dead = true;

-- 复合索引：订阅 + 最新日期
CREATE INDEX IF NOT EXISTS idx_podcasts_subscribed_newest_date
ON podcasts(is_subscribed, newest_episode_date DESC)
WHERE is_subscribed = true;

-- 复合索引：有效 + 优先级
CREATE INDEX IF NOT EXISTS idx_podcasts_valid_priority
ON podcasts(is_dead, priority DESC)
WHERE is_dead = false;

-- 抓取错误次数
CREATE INDEX IF NOT EXISTS idx_podcasts_fetch_error_count
ON podcasts(fetch_error_count DESC)
WHERE fetch_error_count > 0;

-- 2. episodes 表优化
-- ===================

-- 发布日期查询
CREATE INDEX IF NOT EXISTS idx_episodes_published_date
ON episodes(podcast_id, published_date DESC);

-- 单集列表稳定排序
CREATE INDEX IF NOT EXISTS idx_episodes_podcast_published_id_desc
ON episodes(podcast_id, published_date DESC, id DESC);

-- 更新时间查询
CREATE INDEX IF NOT EXISTS idx_episodes_updated_date
ON episodes(podcast_id, updated_date DESC)
WHERE updated_date IS NOT NULL;

-- 音频时长
CREATE INDEX IF NOT EXISTS idx_episodes_duration
ON episodes(duration);

-- 抓取时间
CREATE INDEX IF NOT EXISTS idx_episodes_fetched_at
ON episodes(fetched_at DESC)
WHERE fetched_at IS NOT NULL;

-- 3. jobs 表优化
-- ==============

-- 工作流 + 创建时间
CREATE INDEX IF NOT EXISTS idx_jobs_workflow_created
ON jobs(workflow_id, created_at DESC);

-- 状态 + 创建时间
CREATE INDEX IF NOT EXISTS idx_jobs_status_created
ON jobs(status, created_at DESC);

-- 执行时间统计
CREATE INDEX IF NOT EXISTS idx_jobs_start_time
ON jobs(start_time DESC)
WHERE start_time IS NOT NULL;

-- 触发方式
CREATE INDEX IF NOT EXISTS idx_jobs_triggered_by
ON jobs(triggered_by, created_at DESC);

-- 复合索引：工作流 + 状态 + 创建时间
CREATE INDEX IF NOT EXISTS idx_jobs_workflow_status_created
ON jobs(workflow_id, status, created_at DESC);

-- 4. workflows 表优化
-- ==================

-- 启用状态
CREATE INDEX IF NOT EXISTS idx_workflows_is_enabled
ON workflows(is_enabled)
WHERE is_enabled = true;

-- 工作流列表更新时间排序
CREATE INDEX IF NOT EXISTS idx_workflows_updated_at_desc
ON workflows(updated_at DESC, id DESC);

-- 工作流下次执行时间排序
CREATE INDEX IF NOT EXISTS idx_workflows_next_run_at
ON workflows(next_run_at ASC, id ASC);

-- 启用 + 调度
CREATE INDEX IF NOT EXISTS idx_workflows_enabled_schedule
ON workflows(is_enabled, schedule)
WHERE is_enabled = true AND schedule != "";

-- 5. tags 关联表优化
-- ===================

-- tags 表名称唯一
CREATE UNIQUE INDEX IF NOT EXISTS idx_tags_name
ON tags(name);

-- podcasts_tags 关联表
CREATE INDEX IF NOT EXISTS idx_podcasts_tags_tag_id
ON podcasts_tags(tag_id);

CREATE INDEX IF NOT EXISTS idx_podcasts_tags_podcast_id
ON podcasts_tags(podcast_id);

-- episodes_tags 关联表
CREATE INDEX IF NOT EXISTS idx_episodes_tags_tag_id
ON episodes_tags(tag_id);

CREATE INDEX IF NOT EXISTS idx_episodes_tags_episode_id
ON episodes_tags(episode_id);

-- 6. job_executions 表优化
-- =========================

-- Job执行记录
CREATE INDEX IF NOT EXISTS idx_job_executions_job_id_status
ON job_executions(job_id, status, created_at DESC);

-- Podcast执行记录
CREATE INDEX IF NOT EXISTS idx_job_executions_podcast_status
ON job_executions(podcast_id, status)
WHERE podcast_id IS NOT NULL;

-- 失败执行重试
CREATE INDEX IF NOT EXISTS idx_job_executions_status_retry
ON job_executions(status, created_at DESC)
WHERE status = 'failed';

-- 7. reports 表优化
-- =================

-- 生成时间排序
CREATE INDEX IF NOT EXISTS idx_reports_created_at
ON reports(created_at DESC);

-- 8. sync_configs 表优化
-- ========================

-- 配置键唯一
CREATE UNIQUE INDEX IF NOT EXISTS idx_sync_configs_key
ON sync_configs(config_key);

-- ============================================
-- 索引创建完成
-- ============================================

-- 验证索引
SELECT name, tbl_name
FROM sqlite_master
WHERE type = 'index'
  AND name LIKE 'idx_%'
ORDER BY tbl_name, name;
