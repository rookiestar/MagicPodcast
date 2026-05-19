-- Search acceleration for title/body search.
-- Uses FTS4 because it is available in the Go SQLite runtime used by the app.
-- English and numeric keyword searches can use this index-backed path. Chinese
-- and shorter mixed queries keep the previous LIKE path for correctness.

CREATE VIRTUAL TABLE IF NOT EXISTS podcast_search_fts
USING fts4(
  title,
  author,
  description,
  content='podcasts',
  tokenize=unicode61
);

CREATE VIRTUAL TABLE IF NOT EXISTS episode_search_fts
USING fts4(
  title,
  show_notes,
  content='episodes',
  tokenize=unicode61
);

INSERT INTO podcast_search_fts(podcast_search_fts) VALUES('rebuild');
INSERT INTO episode_search_fts(episode_search_fts) VALUES('rebuild');

CREATE TRIGGER IF NOT EXISTS podcast_search_fts_ai
AFTER INSERT ON podcasts
BEGIN
  INSERT INTO podcast_search_fts(docid, title, author, description)
  VALUES (new.id, new.title, new.author, new.description);
END;

CREATE TRIGGER IF NOT EXISTS podcast_search_fts_ad
AFTER DELETE ON podcasts
BEGIN
  INSERT INTO podcast_search_fts(podcast_search_fts, docid, title, author, description)
  VALUES('delete', old.id, old.title, old.author, old.description);
END;

CREATE TRIGGER IF NOT EXISTS podcast_search_fts_au
AFTER UPDATE ON podcasts
BEGIN
  INSERT INTO podcast_search_fts(podcast_search_fts, docid, title, author, description)
  VALUES('delete', old.id, old.title, old.author, old.description);

  INSERT INTO podcast_search_fts(docid, title, author, description)
  VALUES (new.id, new.title, new.author, new.description);
END;

CREATE TRIGGER IF NOT EXISTS episode_search_fts_ai
AFTER INSERT ON episodes
BEGIN
  INSERT INTO episode_search_fts(docid, title, show_notes)
  VALUES (new.id, new.title, new.show_notes);
END;

CREATE TRIGGER IF NOT EXISTS episode_search_fts_ad
AFTER DELETE ON episodes
BEGIN
  INSERT INTO episode_search_fts(episode_search_fts, docid, title, show_notes)
  VALUES('delete', old.id, old.title, old.show_notes);
END;

CREATE TRIGGER IF NOT EXISTS episode_search_fts_au
AFTER UPDATE ON episodes
BEGIN
  INSERT INTO episode_search_fts(episode_search_fts, docid, title, show_notes)
  VALUES('delete', old.id, old.title, old.show_notes);

  INSERT INTO episode_search_fts(docid, title, show_notes)
  VALUES (new.id, new.title, new.show_notes);
END;
