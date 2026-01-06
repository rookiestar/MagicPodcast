-- =====================================================
-- PodcastIndex 唯一播客视图
-- 功能：为每个title筛选出最优的一条播客记录
-- =====================================================

-- 创建视图：每个title的最佳播客
CREATE VIEW IF NOT EXISTS v_unique_podcasts AS
WITH ranked_podcasts AS (
    SELECT
        id,
        title,
        itunesAuthor,
        description,
        imageUrl,
        url,
        itunesId,
        language,
        link,
        newestEnclosureUrl,
        newestEnclosureDuration,
        lastUpdate,
        newestItemPubdate,
        oldestItemPubdate,
        popularityScore,
        priority,
        updateFrequency,
        episodeCount,
        dead,
        lastHttpStatus,
        explicit,
        -- 计算排名：优先级从高到低，newestItemPubdate越新越好
        ROW_NUMBER() OVER (
            PARTITION BY title
            ORDER BY
                -- 1. 优先选择 dead = 0（未失效）的记录
                dead ASC,
                -- 2. 优先选择 lastHttpStatus = 200 的记录
                CASE WHEN lastHttpStatus = 200 THEN 0 ELSE 1 END,
                -- 3. 优先选择 explicit 不为NULL的记录
                CASE WHEN explicit IS NOT NULL THEN 0 ELSE 1 END,
                -- 4. priority 越小越好（-1表示暂停，应该最后选择）
                CASE WHEN priority = -1 THEN 999 ELSE priority END ASC,
                -- 5. newestItemPubdate 最新的优先
                newestItemPubdate DESC,
                -- 6. episodeCount 最多的优先
                episodeCount DESC,
                -- 7. popularityScore 最高的优先
                popularityScore DESC,
                -- 8. id 最小的优先（最早的记录）
                id ASC
        ) AS rank_num
    FROM podcasts
)
SELECT
    id,
    title,
    itunesAuthor,
    description,
    imageUrl,
    url,
    itunesId,
    language,
    link,
    newestEnclosureUrl,
    newestEnclosureDuration,
    lastUpdate,
    newestItemPubdate,
    oldestItemPubdate,
    popularityScore,
    priority,
    updateFrequency,
    episodeCount,
    dead,
    lastHttpStatus,
    explicit
FROM ranked_podcasts
WHERE rank_num = 1;
