package dataprofile

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const SanitizerVersion = "v9"

// episodes.video_availability is a reviewed public tri-state. Snapshot export
// preserves it; no HLS URL, credential, or private note is stored in the field.
const sanitizerSchemaFingerprint = "ab0316ab2ac4c6d4f3ec31ce9a7c5fa27c3f2fe0603c7050ae5428fdb41f084e"
const sanitizerSchemaObjectsFingerprint = "403203ff8bd57de8270f23d428fdc03e7c73f56112722822b333c3335caf7923"

var richTextURLPattern = regexp.MustCompile(`https?://[^\s<>"']+`)

// SanitizeSnapshot fails closed on every schema field not present in the
// current reviewed schema. A new field must therefore receive an explicit
// sanitizer review even when its name does not advertise that it is private.
func SanitizeSnapshot(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("snapshot database is nil")
	}
	hadSearchFTS, err := normalizeReviewedProductionSchema(db)
	if err != nil {
		return err
	}
	unknown, err := unknownSensitiveColumns(db)
	if err != nil {
		return err
	}
	if len(unknown) != 0 {
		return fmt.Errorf("unknown sensitive columns require sanitizer review: %v", unknown)
	}

	transaction, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin sanitizer transaction: %w", err)
	}
	defer transaction.Rollback()
	if _, err := transaction.Exec("DELETE FROM sync_configs"); err != nil {
		return fmt.Errorf("clear sensitive sync configuration: %w", err)
	}
	for _, statement := range []string{
		`UPDATE podcasts
			 SET feed_url = 'snapshot://podcast/' || id,
			     newest_enclosure_url = '',
			     notes = '',
			     custom_cover_url = ''`,
		`UPDATE episodes
			 SET medium_url = '',
			     notes = ''`,
		`UPDATE workflows
		 SET scope_config = CASE
		       WHEN scope_type = 'specific_podcasts' THEN json_object(
		         'podcast_ids',
		         COALESCE(json_extract(scope_config, '$.podcast_ids'), json('[]'))
		       )
		       ELSE NULL
		     END,
		     rules_config = json_remove(COALESCE(rules_config, '{}'), '$.llm_user_prompt')`,
		"UPDATE reports SET llm_error = ''",
		`UPDATE job_executions
		 SET error_message = '',
		     log_info = '',
		     feed_user_agent_approved_by = '',
		     podcast_feed_url = '',
		     feed_source_url = ''`,
		`UPDATE job_feed_attempts
		 SET feed_user_agent_approved_by = '',
		     source_url = ''`,
		"UPDATE feed_user_agent_gates SET approved_by = ''",
		"UPDATE feed_user_agent_gate_audits SET actor = ''",
		`UPDATE podcast_alternative_feeds
		 SET main_feed_url = '',
		     alternative_feed_url = ''`,
		"DELETE FROM feed_snapshots",
		// Processing rows and their schedule history reference local artifact
		// paths, opaque provider identities, and private processing intent that
		// are not part of a database-only snapshot. Removing the whole graph is
		// safer and more truthful than retaining broken artifact, scheduling, or
		// external-delivery state.
		"DELETE FROM episode_audio_assets",
		"DELETE FROM knowledge_deliveries",
		"DELETE FROM processing_checkpoints",
		"DELETE FROM processing_schedule_items",
		"DELETE FROM episode_artifact_audio_recoveries",
		"DELETE FROM episode_artifact_sets",
		"DELETE FROM episode_processing_runs",
		"DELETE FROM processing_schedule_runs",
	} {
		if _, err := transaction.Exec(statement); err != nil {
			return fmt.Errorf("apply snapshot redaction: %w", err)
		}
	}
	if err := sanitizeEpisodeGUIDs(transaction); err != nil {
		return err
	}
	if err := sanitizeRichTextURLs(transaction); err != nil {
		return err
	}
	if hadSearchFTS {
		if err := rebuildReviewedSearchFTS(transaction); err != nil {
			return err
		}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit sanitizer transaction: %w", err)
	}
	return VerifySanitizedSnapshot(db)
}

func VerifySanitizedSnapshot(db *sql.DB) error {
	unknown, err := unknownSensitiveColumns(db)
	if err != nil {
		return err
	}
	if len(unknown) != 0 {
		return fmt.Errorf("unknown sensitive columns require sanitizer review: %v", unknown)
	}
	var count int64
	if err := db.QueryRow("SELECT COUNT(*) FROM sync_configs").Scan(&count); err != nil {
		return fmt.Errorf("verify sync configuration redaction: %w", err)
	}
	if count != 0 {
		return fmt.Errorf("sensitive sync configuration remains after sanitization")
	}
	if err := verifyRichTextURLs(db); err != nil {
		return err
	}
	if err := verifyEpisodeGUIDs(db); err != nil {
		return err
	}
	if err := verifyReviewedSearchFTS(db); err != nil {
		return err
	}
	for _, check := range []struct {
		query string
		label string
	}{
		{"SELECT COUNT(*) FROM podcasts WHERE notes <> '' OR custom_cover_url <> ''", "podcast private fields"},
		{`SELECT COUNT(*) FROM podcasts
			  WHERE feed_url <> 'snapshot://podcast/' || id
			     OR newest_enclosure_url <> ''`, "podcast source URLs"},
		{"SELECT COUNT(*) FROM episodes WHERE medium_url <> '' OR notes <> ''", "episode private fields"},
		{`SELECT COUNT(*) FROM workflows
		  WHERE json_extract(COALESCE(scope_config, '{}'), '$.custom_urls') IS NOT NULL
		     OR json_extract(COALESCE(rules_config, '{}'), '$.llm_user_prompt') IS NOT NULL`, "workflow private configuration"},
		{"SELECT COUNT(*) FROM reports WHERE llm_error <> ''", "report LLM errors"},
		{`SELECT COUNT(*) FROM job_executions
		  WHERE error_message <> ''
		     OR log_info <> ''
		     OR feed_user_agent_approved_by <> ''
		     OR podcast_feed_url <> ''
		     OR feed_source_url <> ''`, "job execution private fields"},
		{`SELECT COUNT(*) FROM job_feed_attempts
		  WHERE feed_user_agent_approved_by <> ''
		     OR source_url <> ''`, "job attempt private fields"},
		{"SELECT COUNT(*) FROM feed_user_agent_gates WHERE approved_by <> ''", "feed gate operator identities"},
		{"SELECT COUNT(*) FROM feed_user_agent_gate_audits WHERE actor <> ''", "feed gate audit identities"},
		{`SELECT COUNT(*) FROM podcast_alternative_feeds
		  WHERE main_feed_url <> '' OR alternative_feed_url <> ''`, "alternative Feed URLs"},
		{"SELECT COUNT(*) FROM feed_snapshots", "durable feed bodies"},
		{"SELECT COUNT(*) FROM episode_audio_assets", "managed episode audio paths and sources"},
		{"SELECT COUNT(*) FROM knowledge_deliveries", "knowledge delivery identities"},
		{"SELECT COUNT(*) FROM processing_checkpoints", "processing provider checkpoints"},
		{"SELECT COUNT(*) FROM processing_schedule_items", "processing schedule candidate history"},
		{"SELECT COUNT(*) FROM episode_artifact_audio_recoveries", "audio recovery state"},
		{"SELECT COUNT(*) FROM episode_artifact_sets", "local processing artifact paths"},
		{"SELECT COUNT(*) FROM episode_processing_runs", "processing run metadata"},
		{"SELECT COUNT(*) FROM processing_schedule_runs", "processing schedule trigger history"},
	} {
		if err := db.QueryRow(check.query).Scan(&count); err != nil {
			return fmt.Errorf("verify %s redaction: %w", check.label, err)
		}
		if count != 0 {
			return fmt.Errorf("%s remain after sanitization", check.label)
		}
	}
	return nil
}

type richTextColumn struct {
	table  string
	column string
}

var richTextColumns = []richTextColumn{
	{table: "podcasts", column: "description"},
	{table: "podcasts", column: "cover_url"},
	{table: "podcasts", column: "link"},
	{table: "podcasts", column: "podcast_guid"},
	{table: "episodes", column: "show_notes"},
	{table: "episodes", column: "content"},
	{table: "episodes", column: "link"},
	{table: "episodes", column: "image_url"},
	{table: "reports", column: "content"},
	{table: "reports", column: "summary"},
	{table: "reports", column: "llm_summary"},
	{table: "reports", column: "structured_episodes"},
}

func sanitizeEpisodeGUIDs(transaction *sql.Tx) error {
	rows, err := transaction.Query("SELECT id, COALESCE(guid, '') FROM episodes ORDER BY id")
	if err != nil {
		return fmt.Errorf("read episode GUIDs for URL redaction: %w", err)
	}
	type update struct {
		id int64
	}
	var updates []update
	for rows.Next() {
		var id int64
		var value string
		if err := rows.Scan(&id, &value); err != nil {
			rows.Close()
			return fmt.Errorf("scan episode GUIDs for URL redaction: %w", err)
		}
		if sanitizeURLs(value) != value {
			updates = append(updates, update{id: id})
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("read episode GUIDs for URL redaction: %w", err)
	}
	rows.Close()
	for _, item := range updates {
		if _, err := transaction.Exec(
			"UPDATE episodes SET guid = 'snapshot://episode-guid/' || id WHERE id = ?",
			item.id,
		); err != nil {
			return fmt.Errorf("redact episode GUID URL: %w", err)
		}
	}
	return nil
}

func verifyEpisodeGUIDs(db *sql.DB) error {
	rows, err := db.Query("SELECT id, COALESCE(guid, '') FROM episodes ORDER BY id")
	if err != nil {
		return fmt.Errorf("read episode GUIDs for URL verification: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var value string
		if err := rows.Scan(&id, &value); err != nil {
			return fmt.Errorf("scan episode GUIDs for URL verification: %w", err)
		}
		if sanitizeURLs(value) != value {
			return fmt.Errorf("episodes.guid row %d retains URL credentials, query, or fragment", id)
		}
	}
	return rows.Err()
}

func sanitizeRichTextURLs(transaction *sql.Tx) error {
	for _, field := range richTextColumns {
		query := fmt.Sprintf(
			"SELECT rowid, COALESCE(%s, '') FROM %s",
			quoteIdentifier(field.column),
			quoteIdentifier(field.table),
		)
		rows, err := transaction.Query(query)
		if err != nil {
			return fmt.Errorf("read %s.%s for URL redaction: %w", field.table, field.column, err)
		}
		type update struct {
			rowID int64
			value string
		}
		var updates []update
		for rows.Next() {
			var rowID int64
			var value string
			if err := rows.Scan(&rowID, &value); err != nil {
				rows.Close()
				return fmt.Errorf("scan %s.%s for URL redaction: %w", field.table, field.column, err)
			}
			sanitized := sanitizeURLs(value)
			if sanitized != value {
				updates = append(updates, update{rowID: rowID, value: sanitized})
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return fmt.Errorf("read %s.%s for URL redaction: %w", field.table, field.column, err)
		}
		rows.Close()
		for _, item := range updates {
			statement := fmt.Sprintf(
				"UPDATE %s SET %s = ? WHERE rowid = ?",
				quoteIdentifier(field.table),
				quoteIdentifier(field.column),
			)
			if _, err := transaction.Exec(statement, item.value, item.rowID); err != nil {
				return fmt.Errorf("redact %s.%s URLs: %w", field.table, field.column, err)
			}
		}
	}
	return nil
}

func verifyRichTextURLs(db *sql.DB) error {
	for _, field := range richTextColumns {
		query := fmt.Sprintf(
			"SELECT rowid, COALESCE(%s, '') FROM %s",
			quoteIdentifier(field.column),
			quoteIdentifier(field.table),
		)
		rows, err := db.Query(query)
		if err != nil {
			return fmt.Errorf("read %s.%s for URL verification: %w", field.table, field.column, err)
		}
		for rows.Next() {
			var rowID int64
			var value string
			if err := rows.Scan(&rowID, &value); err != nil {
				rows.Close()
				return fmt.Errorf("scan %s.%s for URL verification: %w", field.table, field.column, err)
			}
			if sanitizeURLs(value) != value {
				rows.Close()
				return fmt.Errorf("%s.%s row %d retains URL credentials, query, or fragment", field.table, field.column, rowID)
			}
			if field.column == "structured_episodes" && value != "" {
				var structured any
				if err := json.Unmarshal([]byte(value), &structured); err != nil {
					rows.Close()
					return fmt.Errorf("verify %s.%s JSON: %w", field.table, field.column, err)
				}
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return fmt.Errorf("read %s.%s for URL verification: %w", field.table, field.column, err)
		}
		rows.Close()
	}
	return nil
}

func sanitizeURLs(value string) string {
	return richTextURLPattern.ReplaceAllStringFunc(value, func(candidate string) string {
		trailing := ""
		for len(candidate) > 0 {
			last := candidate[len(candidate)-1]
			if !strings.ContainsRune(")]}.,;", rune(last)) {
				break
			}
			trailing = string(last) + trailing
			candidate = candidate[:len(candidate)-1]
		}
		parsed, err := url.Parse(candidate)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return candidate + trailing
		}
		parsed.User = nil
		parsed.RawQuery = ""
		parsed.ForceQuery = false
		parsed.Fragment = ""
		return parsed.String() + trailing
	})
}

func unknownSensitiveColumns(db *sql.DB) ([]string, error) {
	expected, err := currentSanitizerSchema()
	if err != nil {
		return nil, err
	}
	actual, err := schemaColumns(db)
	if err != nil {
		return nil, err
	}
	_, ignoredSearchFTSTables, err := reviewedSearchFTSSchema(db)
	if err != nil {
		return nil, err
	}
	for table := range ignoredSearchFTSTables {
		delete(actual, table)
	}
	var unknown []string
	for table, columns := range actual {
		expectedColumns, tableKnown := expected[table]
		if !tableKnown {
			unknown = append(unknown, "unexpected:"+table+".*")
			continue
		}
		for column := range columns {
			if _, known := expectedColumns[column]; !known {
				unknown = append(unknown, "unexpected:"+table+"."+column)
			}
		}
	}
	for table, columns := range expected {
		actualColumns, tableFound := actual[table]
		if !tableFound {
			unknown = append(unknown, "missing:"+table+".*")
			continue
		}
		for column := range columns {
			if _, found := actualColumns[column]; !found {
				unknown = append(unknown, "missing:"+table+"."+column)
			}
		}
	}
	sort.Strings(unknown)
	return unknown, nil
}

func currentSanitizerSchema() (map[string]map[string]struct{}, error) {
	directory, err := os.MkdirTemp("", "magicpodcast-sanitizer-schema-*")
	if err != nil {
		return nil, fmt.Errorf("create sanitizer schema workspace: %w", err)
	}
	defer os.RemoveAll(directory)
	path := filepath.Join(directory, "reference.db")
	if err := buildFixtureDatabase(path); err != nil {
		return nil, fmt.Errorf("build sanitizer schema reference: %w", err)
	}
	reference, err := openSQLDatabase(path, true)
	if err != nil {
		return nil, fmt.Errorf("open sanitizer schema reference: %w", err)
	}
	defer reference.Close()
	schema, err := schemaColumns(reference)
	if err != nil {
		return nil, err
	}
	actualFingerprint := schemaFingerprint(schema)
	if actualFingerprint != sanitizerSchemaFingerprint {
		return nil, fmt.Errorf(
			"sanitizer schema contract requires review: got %s",
			actualFingerprint,
		)
	}
	return schema, nil
}

func schemaFingerprint(schema map[string]map[string]struct{}) string {
	var fields []string
	for table, columns := range schema {
		for column := range columns {
			fields = append(fields, table+"."+column)
		}
	}
	sort.Strings(fields)
	sum := sha256.Sum256([]byte(strings.Join(fields, "\n")))
	return fmt.Sprintf("%x", sum)
}

func schemaColumns(db *sql.DB) (map[string]map[string]struct{}, error) {
	rows, err := db.Query(`
		SELECT name
		FROM sqlite_master
		WHERE type = 'table'
		  AND name NOT LIKE 'sqlite_%'
		ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list snapshot tables: %w", err)
	}
	defer rows.Close()
	result := make(map[string]map[string]struct{})
	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return nil, err
		}
		tables = append(tables, table)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for _, table := range tables {
		query := fmt.Sprintf("PRAGMA table_xinfo(%s)", quoteIdentifier(table))
		columns, err := db.Query(query)
		if err != nil {
			return nil, fmt.Errorf("inspect table %s: %w", table, err)
		}
		for columns.Next() {
			var (
				cid          int
				name         string
				columnType   string
				notNull      int
				defaultValue any
				primaryKey   int
				hidden       int
			)
			if err := columns.Scan(
				&cid,
				&name,
				&columnType,
				&notNull,
				&defaultValue,
				&primaryKey,
				&hidden,
			); err != nil {
				columns.Close()
				return nil, err
			}
			if result[table] == nil {
				result[table] = make(map[string]struct{})
			}
			result[table][name] = struct{}{}
		}
		if err := columns.Err(); err != nil {
			columns.Close()
			return nil, err
		}
		columns.Close()
	}
	return result, nil
}

func quoteIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}
