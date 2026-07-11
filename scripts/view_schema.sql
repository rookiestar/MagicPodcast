-- =====================================================
-- 查看PodcastIndex数据库和视图的Schema
-- =====================================================

.mode column
.headers on

-- 1. 查看原始podcasts表的schema
SELECT "========== 原始 podcasts 表结构 ==========" as info;
PRAGMA table_info(podcasts);

-- 2. 查看视图的schema（如果已创建）
SELECT "========== v_unique_podcasts 视图结构 ==========" as info;
SELECT sql FROM sqlite_master WHERE type='view' AND name='v_unique_podcasts';

-- 3. 查看所有索引
SELECT "========== podcasts 表的索引 ==========" as info;
SELECT sql FROM sqlite_master WHERE type='index' AND tbl_name='podcasts';

-- 4. 显示视图字段说明
SELECT "========== 视图字段说明 ==========" as info;
SELECT 
    '字段名' as category,
    '说明' as description
UNION ALL
SELECT 'id', '主键，自增ID'
UNION ALL
SELECT 'title', '播客标题（用于分组的唯一标识）'
UNION ALL
SELECT 'itunesAuthor', '播客作者/主播'
UNION ALL
SELECT 'description', '播客描述'
UNION ALL
SELECT 'imageUrl', '封面图片URL'
UNION ALL
SELECT 'url', 'RSS Feed URL（订阅源）'
UNION ALL
SELECT 'itunesId', 'iTunes ID'
UNION ALL
SELECT 'language', '语言代码（如zh-CN）'
UNION ALL
SELECT 'link', '播客网站链接'
UNION ALL
SELECT 'newestEnclosureUrl', '最新单集音频URL'
UNION ALL
SELECT 'newestEnclosureDuration', '最新单集时长（秒）'
UNION ALL
SELECT 'lastUpdate', 'Feed最后更新时间（Unix时间戳）'
UNION ALL
SELECT 'newestItemPubdate', '最新单集发布时间（Unix时间戳）⭐'
UNION ALL
SELECT 'oldestItemPubdate', '最旧单集发布时间（Unix时间戳）'
UNION ALL
SELECT 'popularityScore', '受欢迎程度评分（0-10）⭐'
UNION ALL
SELECT 'priority', '抓取优先级（-1=暂停, 0=正常, 1-10=优先级递增）⭐'
UNION ALL
SELECT 'updateFrequency', '更新频率（0-10）'
UNION ALL
SELECT 'episodeCount', '单集总数⭐'
UNION ALL
SELECT 'dead', '是否失效（0=未失效, 1=已失效）⭐'
UNION ALL
SELECT 'lastHttpStatus', '最后HTTP状态码（200=正常）⭐'
UNION ALL
SELECT 'explicit', '内容分级（yes/no/clean）⭐';

-- 5. 显示筛选规则
SELECT "========== 筛选规则（按优先级） ==========" as info;
SELECT 
    '优先级' as rank_order,
    '字段' as field_name,
    '规则' as rule
UNION ALL
SELECT '1', 'dead', '0（未失效）优先于 1（已失效）'
UNION ALL
SELECT '2', 'lastHttpStatus', '200（正常）优先于其他状态码'
UNION ALL
SELECT '3', 'explicit', '不为空（有分级信息）优先于为空'
UNION ALL
SELECT '4', 'priority', '数值越小越优先（-1最低，0正常）'
UNION ALL
SELECT '5', 'newestItemPubdate', '数值越大越优先（最新发布时间）'
UNION ALL
SELECT '6', 'episodeCount', '数值越大越优先（单集数多）'
UNION ALL
SELECT '7', 'popularityScore', '数值越大越优先（受欢迎）'
UNION ALL
SELECT '8', 'id', '数值越小越优先（最早的记录）';

-- 6. 示例：对比原始表和视图
SELECT "========== 示例：对比原始表和视图 ==========" as info;
-- 查看特定标题的所有原始记录（如果存在）
-- SELECT COUNT(*) as "原始记录数", COUNT(DISTINCT title) as "唯一标题数" FROM podcasts;
