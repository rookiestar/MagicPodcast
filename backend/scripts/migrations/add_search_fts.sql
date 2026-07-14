-- Search acceleration for title/body search.
-- Uses FTS4 because it is available in the Go SQLite runtime used by the app.
-- The index is maintained as a standalone rowid store. Keeping the source
-- tables as FTS4 external-content tables makes UPDATE maintenance fail with
-- "SQL logic error" in the runtime SQLite build used by MagicPodcast.
-- English and numeric keyword searches can use this index-backed path. Chinese
-- and shorter mixed queries keep the previous LIKE path for correctness.

DROP TRIGGER IF EXISTS podcast_search_fts_ai;
DROP TRIGGER IF EXISTS podcast_search_fts_ad;
DROP TRIGGER IF EXISTS podcast_search_fts_au;
DROP TRIGGER IF EXISTS episode_search_fts_ai;
DROP TRIGGER IF EXISTS episode_search_fts_ad;
DROP TRIGGER IF EXISTS episode_search_fts_au;

DROP TABLE IF EXISTS podcast_search_fts;
DROP TABLE IF EXISTS episode_search_fts;

CREATE VIRTUAL TABLE podcast_search_fts
USING fts4(
  title,
  author,
  description,
  tokenize=unicode61
);

CREATE VIRTUAL TABLE episode_search_fts
USING fts4(
  title,
  show_notes,
  tokenize=unicode61
);

INSERT INTO podcast_search_fts(rowid, title, author, description)
SELECT id, title, author, description FROM podcasts WHERE deleted_at IS NULL;

INSERT INTO episode_search_fts(rowid, title, show_notes)
SELECT id, title, show_notes FROM episodes WHERE deleted_at IS NULL;

CREATE TRIGGER IF NOT EXISTS podcast_search_fts_ai
AFTER INSERT ON podcasts
BEGIN
  INSERT INTO podcast_search_fts(rowid, title, author, description)
  SELECT new.id, new.title, new.author, new.description
  WHERE new.deleted_at IS NULL;
END;

CREATE TRIGGER IF NOT EXISTS podcast_search_fts_ad
AFTER DELETE ON podcasts
BEGIN
  DELETE FROM podcast_search_fts WHERE rowid = old.id;
END;

CREATE TRIGGER IF NOT EXISTS podcast_search_fts_au
AFTER UPDATE ON podcasts
BEGIN
  DELETE FROM podcast_search_fts WHERE rowid = old.id;

  INSERT INTO podcast_search_fts(rowid, title, author, description)
  SELECT new.id, new.title, new.author, new.description
  WHERE new.deleted_at IS NULL;
END;

CREATE TRIGGER IF NOT EXISTS episode_search_fts_ai
AFTER INSERT ON episodes
BEGIN
  INSERT INTO episode_search_fts(rowid, title, show_notes)
  SELECT new.id, new.title, new.show_notes
  WHERE new.deleted_at IS NULL;
END;

CREATE TRIGGER IF NOT EXISTS episode_search_fts_ad
AFTER DELETE ON episodes
BEGIN
  DELETE FROM episode_search_fts WHERE rowid = old.id;
END;

CREATE TRIGGER IF NOT EXISTS episode_search_fts_au
AFTER UPDATE ON episodes
BEGIN
  DELETE FROM episode_search_fts WHERE rowid = old.id;

  INSERT INTO episode_search_fts(rowid, title, show_notes)
  SELECT new.id, new.title, new.show_notes
  WHERE new.deleted_at IS NULL;
END;
