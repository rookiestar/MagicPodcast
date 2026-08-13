package dataprofile

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var reviewedLegacyColumns = map[string]map[string]string{
	"job_executions": {
		"job": "INTEGER",
	},
	"tags": {
		"deleted_at":  "DATETIME",
		"description": "TEXT",
	},
	"workflows": {
		"last_job": "INTEGER",
	},
}

type reviewedHistoricalIndex struct {
	table      string
	definition string
}

// These performance-only indexes were inspected from the production schema on
// 2026-08-14. Only the exact name, table, and SQL definition are compatible.
var reviewedProductionHistoricalIndexes = map[string]reviewedHistoricalIndex{
	"idx_episodes_duration": {
		table:      "episodes",
		definition: `CREATE INDEX idx_episodes_duration ON episodes(duration)`,
	},
	"idx_episodes_fetched_at": {
		table:      "episodes",
		definition: `CREATE INDEX idx_episodes_fetched_at ON episodes(fetched_at DESC) WHERE fetched_at IS NOT NULL`,
	},
	"idx_episodes_published_date": {
		table:      "episodes",
		definition: `CREATE INDEX idx_episodes_published_date ON episodes(podcast_id, published_date DESC)`,
	},
	"idx_episodes_tags_episode_id": {
		table:      "episodes_tags",
		definition: `CREATE INDEX idx_episodes_tags_episode_id ON episodes_tags(episode_id)`,
	},
	"idx_episodes_tags_tag_id": {
		table:      "episodes_tags",
		definition: `CREATE INDEX idx_episodes_tags_tag_id ON episodes_tags(tag_id)`,
	},
	"idx_episodes_updated_date": {
		table:      "episodes",
		definition: `CREATE INDEX idx_episodes_updated_date ON episodes(podcast_id, updated_date DESC) WHERE updated_date IS NOT NULL`,
	},
	"idx_job_executions_job_id_status": {
		table:      "job_executions",
		definition: `CREATE INDEX idx_job_executions_job_id_status ON job_executions(job_id, status, created_at DESC)`,
	},
	"idx_job_executions_podcast_status": {
		table:      "job_executions",
		definition: `CREATE INDEX idx_job_executions_podcast_status ON job_executions(podcast_id, status) WHERE podcast_id IS NOT NULL`,
	},
	"idx_job_executions_status_retry": {
		table:      "job_executions",
		definition: `CREATE INDEX idx_job_executions_status_retry ON job_executions(status, created_at DESC) WHERE status = 'failed'`,
	},
	"idx_jobs_start_time": {
		table:      "jobs",
		definition: `CREATE INDEX idx_jobs_start_time ON jobs(start_time DESC) WHERE start_time IS NOT NULL`,
	},
	"idx_jobs_status_created": {
		table:      "jobs",
		definition: `CREATE INDEX idx_jobs_status_created ON jobs(status, created_at DESC)`,
	},
	"idx_jobs_triggered_by": {
		table:      "jobs",
		definition: `CREATE INDEX idx_jobs_triggered_by ON jobs(triggered_by, created_at DESC)`,
	},
	"idx_jobs_workflow_created": {
		table:      "jobs",
		definition: `CREATE INDEX idx_jobs_workflow_created ON jobs(workflow_id, created_at DESC)`,
	},
	"idx_jobs_workflow_status_created": {
		table:      "jobs",
		definition: `CREATE INDEX idx_jobs_workflow_status_created ON jobs(workflow_id, status, created_at DESC)`,
	},
	"idx_podcasts_author_fts": {
		table:      "podcasts",
		definition: `CREATE INDEX idx_podcasts_author_fts ON podcasts(author COLLATE NOCASE)`,
	},
	"idx_podcasts_data_source": {
		table:      "podcasts",
		definition: `CREATE INDEX idx_podcasts_data_source ON podcasts(data_source)`,
	},
	"idx_podcasts_deleted_author": {
		table:      "podcasts",
		definition: `CREATE INDEX idx_podcasts_deleted_author ON podcasts(deleted_at, author COLLATE NOCASE)`,
	},
	"idx_podcasts_deleted_title": {
		table:      "podcasts",
		definition: `CREATE INDEX idx_podcasts_deleted_title ON podcasts(deleted_at, title COLLATE NOCASE)`,
	},
	"idx_podcasts_fetch_error_count": {
		table:      "podcasts",
		definition: `CREATE INDEX idx_podcasts_fetch_error_count ON podcasts(fetch_error_count DESC) WHERE fetch_error_count > 0`,
	},
	"idx_podcasts_is_dead": {
		table:      "podcasts",
		definition: `CREATE INDEX idx_podcasts_is_dead ON podcasts(is_dead) WHERE is_dead = true`,
	},
	"idx_podcasts_is_subscribed": {
		table:      "podcasts",
		definition: `CREATE INDEX idx_podcasts_is_subscribed ON podcasts(is_subscribed) WHERE is_subscribed = true`,
	},
	"idx_podcasts_last_fetched_at": {
		table:      "podcasts",
		definition: `CREATE INDEX idx_podcasts_last_fetched_at ON podcasts(last_fetched_at DESC) WHERE last_fetched_at IS NOT NULL`,
	},
	"idx_podcasts_newest_episode_date_desc": {
		table:      "podcasts",
		definition: `CREATE INDEX idx_podcasts_newest_episode_date_desc ON podcasts(newest_episode_date DESC)`,
	},
	"idx_podcasts_priority_dead": {
		table:      "podcasts",
		definition: `CREATE INDEX idx_podcasts_priority_dead ON podcasts(priority, is_dead) WHERE is_dead = false`,
	},
	"idx_podcasts_subscribed_newest_date": {
		table:      "podcasts",
		definition: `CREATE INDEX idx_podcasts_subscribed_newest_date ON podcasts(is_subscribed, newest_episode_date DESC) WHERE is_subscribed = true`,
	},
	"idx_podcasts_tags_podcast_id": {
		table:      "podcasts_tags",
		definition: `CREATE INDEX idx_podcasts_tags_podcast_id ON podcasts_tags(podcast_id)`,
	},
	"idx_podcasts_tags_tag_id": {
		table:      "podcasts_tags",
		definition: `CREATE INDEX idx_podcasts_tags_tag_id ON podcasts_tags(tag_id)`,
	},
	"idx_podcasts_title_fts": {
		table:      "podcasts",
		definition: `CREATE INDEX idx_podcasts_title_fts ON podcasts(title COLLATE NOCASE)`,
	},
	"idx_podcasts_valid_priority": {
		table:      "podcasts",
		definition: `CREATE INDEX idx_podcasts_valid_priority ON podcasts(is_dead, priority DESC) WHERE is_dead = false`,
	},
	"idx_reports_created_at": {
		table:      "reports",
		definition: `CREATE INDEX idx_reports_created_at ON reports(created_at DESC)`,
	},
	"idx_sync_configs_key": {
		table:      "sync_configs",
		definition: `CREATE UNIQUE INDEX idx_sync_configs_key ON sync_configs(config_key)`,
	},
	"idx_workflows_deleted_at": {
		table:      "workflows",
		definition: `CREATE INDEX idx_workflows_deleted_at ON workflows(deleted_at)`,
	},
	"idx_workflows_is_enabled": {
		table:      "workflows",
		definition: `CREATE INDEX idx_workflows_is_enabled ON workflows(is_enabled)`,
	},
	"idx_workflows_last_execution_at": {
		table:      "workflows",
		definition: `CREATE INDEX idx_workflows_last_execution_at ON workflows(last_execution_at)`,
	},
	"idx_workflows_last_job_id": {
		table:      "workflows",
		definition: `CREATE INDEX idx_workflows_last_job_id ON workflows(last_job_id)`,
	},
	"idx_workflows_next_run_at": {
		table:      "workflows",
		definition: `CREATE INDEX idx_workflows_next_run_at ON workflows(next_run_at)`,
	},
	"idx_workflows_scope_type": {
		table:      "workflows",
		definition: `CREATE INDEX idx_workflows_scope_type ON workflows(scope_type)`,
	},
	"idx_workflows_enabled_schedule": {
		table:      "workflows",
		definition: `CREATE INDEX idx_workflows_enabled_schedule ON workflows(is_enabled, schedule) WHERE is_enabled = true AND schedule != ''`,
	},
}

var reviewedProductionMissingCurrentIndexes = map[string]reviewedHistoricalIndex{
	"idx_jobs_compensated_by_job_id": {
		table:      "jobs",
		definition: "CREATE INDEX `idx_jobs_compensated_by_job_id` ON `jobs`(`compensated_by_job_id`)",
	},
	"idx_jobs_compensation_of_job_id": {
		table:      "jobs",
		definition: "CREATE INDEX `idx_jobs_compensation_of_job_id` ON `jobs`(`compensation_of_job_id`)",
	},
}

var reviewedSearchFTSTableColumns = map[string][]string{
	"podcast_search_fts":          {"title", "author", "description"},
	"podcast_search_fts_content":  {"docid", "c0title", "c1author", "c2description"},
	"podcast_search_fts_docsize":  {"docid", "size"},
	"podcast_search_fts_segdir":   {"level", "idx", "start_block", "leaves_end_block", "end_block", "root"},
	"podcast_search_fts_segments": {"blockid", "block"},
	"podcast_search_fts_stat":     {"id", "value"},
	"episode_search_fts":          {"title", "show_notes"},
	"episode_search_fts_content":  {"docid", "c0title", "c1show_notes"},
	"episode_search_fts_docsize":  {"docid", "size"},
	"episode_search_fts_segdir":   {"level", "idx", "start_block", "leaves_end_block", "end_block", "root"},
	"episode_search_fts_segments": {"blockid", "block"},
	"episode_search_fts_stat":     {"id", "value"},
}

const createPodcastSearchFTS = `CREATE VIRTUAL TABLE podcast_search_fts
USING fts4(
  title,
  author,
  description,
  tokenize=unicode61
)`

const createEpisodeSearchFTS = `CREATE VIRTUAL TABLE episode_search_fts
USING fts4(
  title,
  show_notes,
  tokenize=unicode61
)`

var reviewedSearchFTSTriggers = map[string]string{
	"podcast_search_fts_ai": `CREATE TRIGGER podcast_search_fts_ai
AFTER INSERT ON podcasts
BEGIN
  INSERT INTO podcast_search_fts(rowid, title, author, description)
  SELECT new.id, new.title, new.author, new.description
  WHERE new.deleted_at IS NULL;
END`,
	"podcast_search_fts_ad": `CREATE TRIGGER podcast_search_fts_ad
AFTER DELETE ON podcasts
BEGIN
  DELETE FROM podcast_search_fts WHERE rowid = old.id;
END`,
	"podcast_search_fts_au": `CREATE TRIGGER podcast_search_fts_au
AFTER UPDATE ON podcasts
BEGIN
  DELETE FROM podcast_search_fts WHERE rowid = old.id;

  INSERT INTO podcast_search_fts(rowid, title, author, description)
  SELECT new.id, new.title, new.author, new.description
  WHERE new.deleted_at IS NULL;
END`,
	"episode_search_fts_ai": `CREATE TRIGGER episode_search_fts_ai
AFTER INSERT ON episodes
BEGIN
  INSERT INTO episode_search_fts(rowid, title, show_notes)
  SELECT new.id, new.title, new.show_notes
  WHERE new.deleted_at IS NULL;
END`,
	"episode_search_fts_ad": `CREATE TRIGGER episode_search_fts_ad
AFTER DELETE ON episodes
BEGIN
  DELETE FROM episode_search_fts WHERE rowid = old.id;
END`,
	"episode_search_fts_au": `CREATE TRIGGER episode_search_fts_au
AFTER UPDATE ON episodes
BEGIN
  DELETE FROM episode_search_fts WHERE rowid = old.id;

  INSERT INTO episode_search_fts(rowid, title, show_notes)
  SELECT new.id, new.title, new.show_notes
  WHERE new.deleted_at IS NULL;
END`,
}

func normalizeReviewedProductionSchema(db *sql.DB) (bool, error) {
	hasSearchFTS, _, err := reviewedSearchFTSSchema(db)
	if err != nil {
		return false, err
	}
	actual, err := schemaColumns(db)
	if err != nil {
		return false, err
	}
	expected, err := currentSanitizerSchema()
	if err != nil {
		return false, err
	}
	var missing []string
	for table, columns := range expected {
		actualColumns, found := actual[table]
		if !found {
			missing = append(missing, "missing:"+table+".*")
			continue
		}
		for column := range columns {
			if _, found := actualColumns[column]; !found {
				missing = append(missing, "missing:"+table+"."+column)
			}
		}
	}
	if len(missing) != 0 {
		sort.Strings(missing)
		return false, fmt.Errorf("unknown sensitive columns require sanitizer review: %v", missing)
	}
	var legacyColumns [][2]string
	for table, columns := range actual {
		if _, isFTS := reviewedSearchFTSTableColumns[table]; isFTS {
			continue
		}
		if table == "tags_temp" {
			if err := validateReviewedTagsTemp(db, columns); err != nil {
				return false, err
			}
			continue
		}
		expectedColumns, knownTable := expected[table]
		if !knownTable {
			return false, fmt.Errorf("unknown sensitive columns require sanitizer review: [unexpected:%s.*]", table)
		}
		for column := range columns {
			if _, known := expectedColumns[column]; known {
				continue
			}
			expectedType := reviewedLegacyColumns[table][column]
			if expectedType == "" {
				return false, fmt.Errorf(
					"unknown sensitive columns require sanitizer review: [unexpected:%s.%s]",
					table,
					column,
				)
			}
			if err := validateColumnType(db, table, column, expectedType); err != nil {
				return false, err
			}
			legacyColumns = append(legacyColumns, [2]string{table, column})
		}
	}
	if err := validateReviewedLegacyIndexes(db); err != nil {
		return false, err
	}
	missingCurrentIndexes, err := validateReviewedSchemaObjects(db, hasSearchFTS)
	if err != nil {
		return false, err
	}
	sort.Slice(legacyColumns, func(i, j int) bool {
		if legacyColumns[i][0] == legacyColumns[j][0] {
			return legacyColumns[i][1] < legacyColumns[j][1]
		}
		return legacyColumns[i][0] < legacyColumns[j][0]
	})

	transaction, err := db.Begin()
	if err != nil {
		return false, fmt.Errorf("begin production schema normalization: %w", err)
	}
	defer transaction.Rollback()
	if hasSearchFTS {
		if err := dropReviewedSearchFTS(transaction); err != nil {
			return false, err
		}
	}
	if _, found := actual["tags_temp"]; found {
		if _, err := transaction.Exec(`DROP TABLE "tags_temp"`); err != nil {
			return false, fmt.Errorf("drop reviewed legacy tags_temp table: %w", err)
		}
	}
	if _, err := transaction.Exec("DROP INDEX IF EXISTS idx_tags_deleted_at"); err != nil {
		return false, fmt.Errorf("drop reviewed legacy tags deleted-at index: %w", err)
	}
	for _, field := range legacyColumns {
		statement := fmt.Sprintf(
			"ALTER TABLE %s DROP COLUMN %s",
			quoteIdentifier(field[0]),
			quoteIdentifier(field[1]),
		)
		if _, err := transaction.Exec(statement); err != nil {
			return false, fmt.Errorf("drop reviewed legacy column %s.%s: %w", field[0], field[1], err)
		}
	}
	for _, index := range missingCurrentIndexes {
		if _, err := transaction.Exec(index.definition); err != nil {
			return false, fmt.Errorf(
				"create reviewed current index %s: %w",
				index.name,
				err,
			)
		}
	}
	if err := transaction.Commit(); err != nil {
		return false, fmt.Errorf("commit production schema normalization: %w", err)
	}
	return hasSearchFTS, nil
}

func validateReviewedTagsTemp(db *sql.DB, columns map[string]struct{}) error {
	const reviewedColumn = "INSERT INTO tags (name) VALUES ('两性');"
	if len(columns) != 1 {
		return fmt.Errorf("legacy tags_temp schema requires review")
	}
	if _, found := columns[reviewedColumn]; !found {
		return fmt.Errorf("legacy tags_temp schema requires review")
	}
	return validateColumnType(db, "tags_temp", reviewedColumn, "TEXT")
}

func validateColumnType(db *sql.DB, table, column, expectedType string) error {
	query := fmt.Sprintf("PRAGMA table_info(%s)", quoteIdentifier(table))
	rows, err := db.Query(query)
	if err != nil {
		return fmt.Errorf("inspect reviewed legacy column %s.%s: %w", table, column, err)
	}
	defer rows.Close()
	for rows.Next() {
		var (
			cid          int
			name         string
			columnType   string
			notNull      int
			defaultValue any
			primaryKey   int
		)
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return err
		}
		if name == column {
			if !strings.EqualFold(strings.TrimSpace(columnType), expectedType) {
				return fmt.Errorf(
					"reviewed legacy column %s.%s has unexpected type %q",
					table,
					column,
					columnType,
				)
			}
			return nil
		}
	}
	return fmt.Errorf("reviewed legacy column %s.%s is missing", table, column)
}

func validateReviewedLegacyIndexes(db *sql.DB) error {
	rows, err := db.Query(`PRAGMA index_list("tags")`)
	if err != nil {
		return fmt.Errorf("inspect reviewed legacy indexes: %w", err)
	}
	type indexMetadata struct {
		name    string
		unique  int
		origin  string
		partial int
	}
	var indexes []indexMetadata
	for rows.Next() {
		var (
			sequence int
			index    indexMetadata
		)
		if err := rows.Scan(&sequence, &index.name, &index.unique, &index.origin, &index.partial); err != nil {
			rows.Close()
			return err
		}
		indexes = append(indexes, index)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	for _, index := range indexes {
		columns, err := orderedIndexColumns(db, index.name)
		if err != nil {
			return err
		}
		usesDeletedAt := false
		for _, column := range columns {
			if column == "deleted_at" {
				usesDeletedAt = true
				break
			}
		}
		if index.name != "idx_tags_deleted_at" && !usesDeletedAt {
			continue
		}
		if index.name != "idx_tags_deleted_at" ||
			index.unique != 0 ||
			index.origin != "c" ||
			index.partial != 0 ||
			len(columns) != 1 ||
			columns[0] != "deleted_at" {
			return fmt.Errorf("legacy tags deleted-at index requires review")
		}
	}
	return nil
}

func orderedIndexColumns(db *sql.DB, index string) ([]string, error) {
	query := fmt.Sprintf("PRAGMA index_info(%s)", quoteIdentifier(index))
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("inspect reviewed legacy index %s: %w", index, err)
	}
	defer rows.Close()
	var result []string
	for rows.Next() {
		var (
			sequence int
			columnID int
			name     sql.NullString
		)
		if err := rows.Scan(&sequence, &columnID, &name); err != nil {
			return nil, err
		}
		if name.Valid {
			result = append(result, name.String)
		} else {
			result = append(result, "")
		}
	}
	return result, rows.Err()
}

type schemaObject struct {
	objectType string
	name       string
	table      string
	definition string
}

func validateReviewedSchemaObjects(db *sql.DB, hasSearchFTS bool) ([]schemaObject, error) {
	actual, err := schemaObjects(db)
	if err != nil {
		return nil, err
	}
	expected, err := currentSanitizerSchemaObjects()
	if err != nil {
		return nil, err
	}
	delete(actual, schemaObjectKey("index", "idx_tags_deleted_at"))
	if hasSearchFTS {
		for name := range reviewedSearchFTSTriggers {
			delete(actual, schemaObjectKey("trigger", name))
		}
	}
	for key, object := range actual {
		reviewed, reviewedFound := reviewedProductionHistoricalIndexes[object.name]
		if object.objectType == "index" &&
			reviewedFound &&
			object.table == reviewed.table &&
			normalizeSQL(object.definition) == normalizeSQL(reviewed.definition) {
			continue
		}
		expectedObject, found := expected[key]
		if !found {
			if object.objectType == "index" && reviewedFound {
				return nil, fmt.Errorf(
					"database index %s definition requires sanitizer review",
					object.name,
				)
			}
			return nil, fmt.Errorf(
				"unreviewed database %s %s requires sanitizer review",
				object.objectType,
				object.name,
			)
		}
		if object.table != expectedObject.table ||
			normalizeSQL(object.definition) != normalizeSQL(expectedObject.definition) {
			return nil, fmt.Errorf(
				"database %s %s definition requires sanitizer review",
				object.objectType,
				object.name,
			)
		}
	}
	var missingCurrentIndexes []schemaObject
	for key, object := range expected {
		if _, found := actual[key]; !found {
			if object.objectType == "index" {
				reviewed, reviewedFound := reviewedProductionMissingCurrentIndexes[object.name]
				if reviewedFound &&
					object.table == reviewed.table &&
					normalizeSQL(object.definition) == normalizeSQL(reviewed.definition) {
					missingCurrentIndexes = append(missingCurrentIndexes, object)
					continue
				}
			}
			return nil, fmt.Errorf(
				"required database %s %s is missing",
				object.objectType,
				object.name,
			)
		}
	}
	sort.Slice(missingCurrentIndexes, func(i, j int) bool {
		return missingCurrentIndexes[i].name < missingCurrentIndexes[j].name
	})
	return missingCurrentIndexes, nil
}

func currentSanitizerSchemaObjects() (map[string]schemaObject, error) {
	directory, err := os.MkdirTemp("", "magicpodcast-sanitizer-objects-*")
	if err != nil {
		return nil, fmt.Errorf("create sanitizer object workspace: %w", err)
	}
	defer os.RemoveAll(directory)
	path := filepath.Join(directory, "reference.db")
	if err := buildFixtureDatabase(path); err != nil {
		return nil, fmt.Errorf("build sanitizer object reference: %w", err)
	}
	reference, err := openSQLDatabase(path, true)
	if err != nil {
		return nil, fmt.Errorf("open sanitizer object reference: %w", err)
	}
	defer reference.Close()
	objects, err := schemaObjects(reference)
	if err != nil {
		return nil, err
	}
	actualFingerprint := schemaObjectsFingerprint(objects)
	if actualFingerprint != sanitizerSchemaObjectsFingerprint {
		return nil, fmt.Errorf(
			"sanitizer schema-object contract requires review: got %s",
			actualFingerprint,
		)
	}
	return objects, nil
}

func schemaObjects(db *sql.DB) (map[string]schemaObject, error) {
	rows, err := db.Query(`
		SELECT type, name, tbl_name, sql
		FROM sqlite_master
		WHERE type IN ('index', 'trigger', 'view')
		  AND sql IS NOT NULL
		ORDER BY type, name`)
	if err != nil {
		return nil, fmt.Errorf("inspect database schema objects: %w", err)
	}
	defer rows.Close()
	result := make(map[string]schemaObject)
	for rows.Next() {
		var object schemaObject
		if err := rows.Scan(
			&object.objectType,
			&object.name,
			&object.table,
			&object.definition,
		); err != nil {
			return nil, err
		}
		result[schemaObjectKey(object.objectType, object.name)] = object
	}
	return result, rows.Err()
}

func schemaObjectKey(objectType, name string) string {
	return objectType + "\x00" + name
}

func schemaObjectsFingerprint(objects map[string]schemaObject) string {
	var definitions []string
	for _, object := range objects {
		definitions = append(
			definitions,
			object.objectType+"\x00"+object.name+"\x00"+object.table+"\x00"+
				normalizeSQL(object.definition),
		)
	}
	sort.Strings(definitions)
	sum := sha256.Sum256([]byte(strings.Join(definitions, "\n")))
	return fmt.Sprintf("%x", sum)
}

func reviewedSearchFTSSchema(db *sql.DB) (bool, map[string]struct{}, error) {
	rows, err := db.Query(`
		SELECT type, name, COALESCE(sql, '')
		FROM sqlite_master
		WHERE (type = 'table' AND (
		        name LIKE 'podcast_search_fts%'
		     OR name LIKE 'episode_search_fts%'
		      ))
		   OR (type = 'trigger' AND LOWER(COALESCE(sql, '')) LIKE '%_search_fts%')
		ORDER BY type, name`)
	if err != nil {
		return false, nil, fmt.Errorf("inspect search FTS schema: %w", err)
	}
	defer rows.Close()
	tableDefinitions := make(map[string]string)
	triggerDefinitions := make(map[string]string)
	for rows.Next() {
		var objectType, name, definition string
		if err := rows.Scan(&objectType, &name, &definition); err != nil {
			return false, nil, err
		}
		switch objectType {
		case "table":
			tableDefinitions[name] = definition
		case "trigger":
			triggerDefinitions[name] = definition
		}
	}
	if err := rows.Err(); err != nil {
		return false, nil, err
	}
	if len(tableDefinitions) == 0 && len(triggerDefinitions) == 0 {
		return false, nil, nil
	}
	if len(tableDefinitions) != len(reviewedSearchFTSTableColumns) ||
		len(triggerDefinitions) != len(reviewedSearchFTSTriggers) {
		return false, nil, fmt.Errorf("search FTS schema requires review")
	}
	for table, expectedColumns := range reviewedSearchFTSTableColumns {
		if _, found := tableDefinitions[table]; !found {
			return false, nil, fmt.Errorf("search FTS schema requires review")
		}
		columns, err := orderedTableColumns(db, table)
		if err != nil {
			return false, nil, err
		}
		if strings.Join(columns, "\x00") != strings.Join(expectedColumns, "\x00") {
			return false, nil, fmt.Errorf("search FTS schema requires review")
		}
	}
	for table, expectedDefinition := range map[string]string{
		"podcast_search_fts": createPodcastSearchFTS,
		"episode_search_fts": createEpisodeSearchFTS,
	} {
		if normalizeSQL(tableDefinitions[table]) != normalizeSQL(expectedDefinition) {
			return false, nil, fmt.Errorf("search FTS schema requires review")
		}
	}
	for trigger, expectedDefinition := range reviewedSearchFTSTriggers {
		if normalizeSQL(triggerDefinitions[trigger]) != normalizeSQL(expectedDefinition) {
			return false, nil, fmt.Errorf("search FTS schema requires review")
		}
	}
	ignored := make(map[string]struct{}, len(reviewedSearchFTSTableColumns))
	for table := range reviewedSearchFTSTableColumns {
		ignored[table] = struct{}{}
	}
	return true, ignored, nil
}

func orderedTableColumns(db *sql.DB, table string) ([]string, error) {
	query := fmt.Sprintf("PRAGMA table_info(%s)", quoteIdentifier(table))
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("inspect search FTS table %s: %w", table, err)
	}
	defer rows.Close()
	var result []string
	for rows.Next() {
		var (
			cid          int
			name         string
			columnType   string
			notNull      int
			defaultValue any
			primaryKey   int
		)
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return nil, err
		}
		result = append(result, name)
	}
	return result, rows.Err()
}

func normalizeSQL(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

func dropReviewedSearchFTS(transaction *sql.Tx) error {
	var triggerNames []string
	for name := range reviewedSearchFTSTriggers {
		triggerNames = append(triggerNames, name)
	}
	sort.Strings(triggerNames)
	for _, name := range triggerNames {
		if _, err := transaction.Exec("DROP TRIGGER " + quoteIdentifier(name)); err != nil {
			return fmt.Errorf("drop reviewed search FTS trigger %s: %w", name, err)
		}
	}
	for _, table := range []string{"podcast_search_fts", "episode_search_fts"} {
		if _, err := transaction.Exec("DROP TABLE " + quoteIdentifier(table)); err != nil {
			return fmt.Errorf("drop reviewed search FTS table %s: %w", table, err)
		}
	}
	return nil
}

func rebuildReviewedSearchFTS(transaction *sql.Tx) error {
	statements := []string{
		createPodcastSearchFTS,
		createEpisodeSearchFTS,
		`INSERT INTO podcast_search_fts(rowid, title, author, description)
		 SELECT id, title, author, description FROM podcasts WHERE deleted_at IS NULL`,
		`INSERT INTO episode_search_fts(rowid, title, show_notes)
		 SELECT id, title, show_notes FROM episodes WHERE deleted_at IS NULL`,
	}
	var triggerNames []string
	for name := range reviewedSearchFTSTriggers {
		triggerNames = append(triggerNames, name)
	}
	sort.Strings(triggerNames)
	for _, name := range triggerNames {
		statements = append(statements, reviewedSearchFTSTriggers[name])
	}
	for _, statement := range statements {
		if _, err := transaction.Exec(statement); err != nil {
			return fmt.Errorf("rebuild sanitized search FTS: %w", err)
		}
	}
	return nil
}

func verifyReviewedSearchFTS(db *sql.DB) error {
	hasSearchFTS, _, err := reviewedSearchFTSSchema(db)
	if err != nil {
		return err
	}
	if !hasSearchFTS {
		return nil
	}
	for _, check := range []struct {
		query string
		label string
	}{
		{`SELECT COUNT(*)
		  FROM podcast_search_fts AS f
		  LEFT JOIN podcasts AS p ON p.id = f.rowid AND p.deleted_at IS NULL
		  WHERE p.id IS NULL
		     OR f.title IS NOT p.title
		     OR f.author IS NOT p.author
		     OR f.description IS NOT p.description`, "podcast FTS contents"},
		{`SELECT COUNT(*)
		  FROM podcasts AS p
		  LEFT JOIN podcast_search_fts AS f ON f.rowid = p.id
		  WHERE p.deleted_at IS NULL AND f.rowid IS NULL`, "podcast FTS coverage"},
		{`SELECT COUNT(*)
		  FROM episode_search_fts AS f
		  LEFT JOIN episodes AS e ON e.id = f.rowid AND e.deleted_at IS NULL
		  WHERE e.id IS NULL
		     OR f.title IS NOT e.title
		     OR f.show_notes IS NOT e.show_notes`, "episode FTS contents"},
		{`SELECT COUNT(*)
		  FROM episodes AS e
		  LEFT JOIN episode_search_fts AS f ON f.rowid = e.id
		  WHERE e.deleted_at IS NULL AND f.rowid IS NULL`, "episode FTS coverage"},
	} {
		var count int64
		if err := db.QueryRow(check.query).Scan(&count); err != nil {
			return fmt.Errorf("verify %s: %w", check.label, err)
		}
		if count != 0 {
			return fmt.Errorf("%s do not match sanitized source data", check.label)
		}
	}
	return nil
}
