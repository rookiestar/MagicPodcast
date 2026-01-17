SELECT
    id,
    title,
    itunesAuthor,
    description,
    episodeCount,
    popularityScore,
    language
FROM podcasts
WHERE language like 'zh%cn'
  AND dead = 0
  AND episodeCount > 50
  AND popularityScore >= 7
ORDER BY popularityScore DESC, episodeCount DESC
LIMIT 30;

SELECT *
FROM podcasts
WHERE title = '歪波音室';