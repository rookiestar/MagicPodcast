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
	if err := validateReviewedSchemaObjects(db, hasSearchFTS); err != nil {
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
	definition string
}

func validateReviewedSchemaObjects(db *sql.DB, hasSearchFTS bool) error {
	actual, err := schemaObjects(db)
	if err != nil {
		return err
	}
	expected, err := currentSanitizerSchemaObjects()
	if err != nil {
		return err
	}
	delete(actual, schemaObjectKey("index", "idx_tags_deleted_at"))
	if hasSearchFTS {
		for name := range reviewedSearchFTSTriggers {
			delete(actual, schemaObjectKey("trigger", name))
		}
	}
	for key, object := range actual {
		expectedObject, found := expected[key]
		if !found {
			return fmt.Errorf(
				"unreviewed database %s %s requires sanitizer review",
				object.objectType,
				object.name,
			)
		}
		if normalizeSQL(object.definition) != normalizeSQL(expectedObject.definition) {
			return fmt.Errorf(
				"database %s %s definition requires sanitizer review",
				object.objectType,
				object.name,
			)
		}
	}
	for key, object := range expected {
		if _, found := actual[key]; !found {
			return fmt.Errorf(
				"required database %s %s is missing",
				object.objectType,
				object.name,
			)
		}
	}
	return nil
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
		SELECT type, name, sql
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
		if err := rows.Scan(&object.objectType, &object.name, &object.definition); err != nil {
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
			object.objectType+"\x00"+object.name+"\x00"+normalizeSQL(object.definition),
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
