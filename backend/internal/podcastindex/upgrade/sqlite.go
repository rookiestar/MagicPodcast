package upgrade

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"

	"magicpodcast/internal/podcastindex"
	_ "modernc.org/sqlite"
)

func OpenSQLite(path string, readOnly bool) (*sql.DB, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve SQLite path: %w", err)
	}
	dataSource := absPath
	if readOnly {
		dataSource = "file:" + filepath.ToSlash(absPath) + "?mode=ro"
	}
	db, err := sql.Open("sqlite", dataSource)
	if err != nil {
		return nil, fmt.Errorf("open SQLite database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping SQLite database: %w", err)
	}
	return db, nil
}

func ValidateCandidate(path, viewSQL string, fullIntegrityCheck bool) (ValidationResult, error) {
	result := ValidationResult{Path: path}
	sha256, sizeBytes, err := SHA256File(path)
	if err != nil {
		return resultWithError(result, err)
	}
	result.SHA256 = sha256
	result.SizeBytes = sizeBytes

	db, err := OpenSQLite(path, false)
	if err != nil {
		return resultWithError(result, err)
	}
	integrityCheck := "quick_check"
	if fullIntegrityCheck {
		integrityCheck = "integrity_check"
	}
	result.IntegrityCheck = integrityCheck
	if err := runIntegrityCheck(db, integrityCheck); err != nil {
		db.Close()
		return resultWithError(result, err)
	}

	schema, err := inspectSchema(db)
	result.Schema = schema
	if err != nil {
		db.Close()
		return resultWithError(result, err)
	}
	metrics, err := collectMetrics(db, path)
	result.Metrics = metrics
	if err != nil {
		db.Close()
		return resultWithError(result, err)
	}
	if metrics.TotalRows == 0 {
		db.Close()
		return resultWithError(result, fmt.Errorf("candidate podcasts table is empty"))
	}
	if strings.TrimSpace(viewSQL) == "" {
		db.Close()
		return resultWithError(result, fmt.Errorf("view SQL is required"))
	}
	if _, err := db.Exec("DROP VIEW IF EXISTS v_unique_podcasts"); err != nil {
		db.Close()
		return resultWithError(result, fmt.Errorf("remove candidate v_unique_podcasts: %w", err))
	}
	if _, err := db.Exec(viewSQL); err != nil {
		db.Close()
		return resultWithError(result, fmt.Errorf("create candidate v_unique_podcasts: %w", err))
	}
	result.ViewCreated = true
	viewColumns, err := inspectViewColumns(db)
	if err != nil {
		db.Close()
		return resultWithError(result, err)
	}
	result.Schema.ViewName = "v_unique_podcasts"
	result.Schema.ViewColumns = viewColumns
	if err := requireViewColumns(viewColumns); err != nil {
		db.Close()
		return resultWithError(result, err)
	}
	viewCount, err := countView(db)
	if err != nil {
		db.Close()
		return resultWithError(result, err)
	}
	if viewCount == 0 {
		db.Close()
		return resultWithError(result, fmt.Errorf("candidate v_unique_podcasts view is empty"))
	}
	result.Query.ViewCount = viewCount
	if err := db.Close(); err != nil {
		return resultWithError(result, fmt.Errorf("close candidate SQLite database: %w", err))
	}

	queryCompatibility, err := runQueryCompatibility(path)
	result.Query = queryCompatibility
	if err != nil {
		return resultWithError(result, err)
	}
	finalSHA256, finalSizeBytes, err := SHA256File(path)
	if err != nil {
		return resultWithError(result, fmt.Errorf("hash final candidate SQLite: %w", err))
	}
	result.SHA256 = finalSHA256
	result.SizeBytes = finalSizeBytes
	result.Passed = true
	return result, nil
}

func resultWithError(result ValidationResult, err error) (ValidationResult, error) {
	result.Passed = false
	result.Error = err.Error()
	return result, err
}

func runIntegrityCheck(db *sql.DB, pragma string) error {
	row := db.QueryRow("PRAGMA " + pragma)
	var result string
	if err := row.Scan(&result); err != nil {
		return fmt.Errorf("run SQLite %s: %w", pragma, err)
	}
	if strings.ToLower(strings.TrimSpace(result)) != "ok" {
		return fmt.Errorf("SQLite %s returned %q", pragma, result)
	}
	return nil
}

func inspectSchema(db *sql.DB) (SchemaSummary, error) {
	result := SchemaSummary{TableName: "podcasts"}
	rows, err := db.Query("PRAGMA table_info(podcasts)")
	if err != nil {
		return result, fmt.Errorf("inspect podcasts schema: %w", err)
	}
	defer rows.Close()
	columnsByName := make(map[string]SchemaColumn)
	for rows.Next() {
		var column SchemaColumn
		var notNull, primaryKey int
		var defaultValue sql.NullString
		if err := rows.Scan(&column.CID, &column.Name, &column.DeclaredType, &notNull, &defaultValue, &primaryKey); err != nil {
			return result, fmt.Errorf("scan podcasts schema: %w", err)
		}
		column.TypeClass = sqliteTypeClass(column.DeclaredType)
		column.NotNull = notNull != 0
		column.PrimaryKey = primaryKey != 0
		result.Columns = append(result.Columns, column)
		columnsByName[column.Name] = column
	}
	if err := rows.Err(); err != nil {
		return result, fmt.Errorf("read podcasts schema: %w", err)
	}
	if len(result.Columns) == 0 {
		return result, fmt.Errorf("candidate database has no podcasts table")
	}
	for _, requirement := range RequiredPodcastColumns {
		column, ok := columnsByName[requirement.Name]
		if !ok {
			return result, fmt.Errorf("candidate podcasts table is missing required column %q", requirement.Name)
		}
		if column.TypeClass != requirement.TypeClass {
			return result, fmt.Errorf("candidate column %q has type %q (%s), expected %s", requirement.Name, column.DeclaredType, column.TypeClass, requirement.TypeClass)
		}
	}
	result.RequiredFields = true
	if err := db.QueryRow("PRAGMA user_version").Scan(&result.UserVersion); err != nil {
		return result, fmt.Errorf("read SQLite user_version: %w", err)
	}
	if err := db.QueryRow("PRAGMA page_count").Scan(&result.PageCount); err != nil {
		return result, fmt.Errorf("read SQLite page_count: %w", err)
	}
	if err := db.QueryRow("PRAGMA page_size").Scan(&result.PageSize); err != nil {
		return result, fmt.Errorf("read SQLite page_size: %w", err)
	}
	return result, nil
}

func inspectViewColumns(db *sql.DB) ([]string, error) {
	rows, err := db.Query("PRAGMA table_info(v_unique_podcasts)")
	if err != nil {
		return nil, fmt.Errorf("inspect v_unique_podcasts schema: %w", err)
	}
	defer rows.Close()
	var columns []string
	for rows.Next() {
		var cid int
		var name, declaredType string
		var notNull, primaryKey int
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &declaredType, &notNull, &defaultValue, &primaryKey); err != nil {
			return nil, fmt.Errorf("scan v_unique_podcasts schema: %w", err)
		}
		columns = append(columns, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read v_unique_podcasts schema: %w", err)
	}
	if len(columns) == 0 {
		return nil, fmt.Errorf("candidate v_unique_podcasts view has no columns")
	}
	return columns, nil
}

func requireViewColumns(columns []string) error {
	available := make(map[string]struct{}, len(columns))
	for _, column := range columns {
		available[column] = struct{}{}
	}
	for _, required := range RequiredViewColumns {
		if _, ok := available[required]; !ok {
			return fmt.Errorf("candidate v_unique_podcasts view is missing required column %q", required)
		}
	}
	return nil
}

func sqliteTypeClass(declaredType string) string {
	typeName := strings.ToUpper(strings.TrimSpace(declaredType))
	switch {
	case strings.Contains(typeName, "INT"):
		return "integer"
	case strings.Contains(typeName, "CHAR"), strings.Contains(typeName, "CLOB"), strings.Contains(typeName, "TEXT"):
		return "text"
	default:
		return "other"
	}
}

func collectMetrics(db *sql.DB, path string) (DatabaseMetrics, error) {
	metrics := DatabaseMetrics{Path: path, DeadDistribution: map[string]int64{}}
	row := db.QueryRow(`
		SELECT COUNT(*),
		       COALESCE(SUM(CASE WHEN dead = 0 THEN 1 ELSE 0 END), 0),
		       COALESCE(SUM(CASE WHEN lastHttpStatus = 200 THEN 1 ELSE 0 END), 0),
		       COALESCE(SUM(CASE WHEN dead <> 0 THEN 1 ELSE 0 END), 0),
		       COALESCE(MAX(newestItemPubdate), 0),
		       COALESCE(MIN(newestItemPubdate), 0)
		FROM podcasts`)
	if err := row.Scan(&metrics.TotalRows, &metrics.LiveRows, &metrics.HTTP200Rows, &metrics.DeadRows, &metrics.FreshestItemDate, &metrics.OldestItemDate); err != nil {
		return metrics, fmt.Errorf("collect PodcastIndex metrics: %w", err)
	}
	rows, err := db.Query("SELECT CAST(dead AS TEXT), COUNT(*) FROM podcasts GROUP BY dead ORDER BY dead")
	if err != nil {
		return metrics, fmt.Errorf("collect dead distribution: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var value string
		var count int64
		if err := rows.Scan(&value, &count); err != nil {
			return metrics, fmt.Errorf("scan dead distribution: %w", err)
		}
		metrics.DeadDistribution[value] = count
	}
	if err := rows.Err(); err != nil {
		return metrics, fmt.Errorf("read dead distribution: %w", err)
	}
	return metrics, nil
}

func ReadDatabaseMetrics(path string) (DatabaseMetrics, error) {
	db, err := OpenSQLite(path, true)
	if err != nil {
		return DatabaseMetrics{Path: path}, err
	}
	defer db.Close()
	return collectMetrics(db, path)
}

func countView(db *sql.DB) (int64, error) {
	var count int64
	if err := db.QueryRow("SELECT COUNT(*) FROM v_unique_podcasts").Scan(&count); err != nil {
		return 0, fmt.Errorf("count v_unique_podcasts: %w", err)
	}
	return count, nil
}

func runQueryCompatibility(path string) (QueryCompatibility, error) {
	compatibility := QueryCompatibility{}
	db, err := OpenSQLite(path, true)
	if err != nil {
		compatibility.Error = err.Error()
		return compatibility, err
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM v_unique_podcasts").Scan(&compatibility.ViewCount); err != nil {
		db.Close()
		compatibility.Error = err.Error()
		return compatibility, fmt.Errorf("query v_unique_podcasts: %w", err)
	}
	var itunesID sql.NullInt64
	if err := db.QueryRow(`
		SELECT url, title,
		       CASE WHEN typeof(itunesId) IN ('integer', 'real') THEN CAST(itunesId AS INTEGER) ELSE NULL END
		FROM v_unique_podcasts
		WHERE url IS NOT NULL AND url <> '' AND title IS NOT NULL AND title <> ''
		ORDER BY id
		LIMIT 1`).Scan(&compatibility.URL, &compatibility.Title, &itunesID); err != nil {
		db.Close()
		compatibility.Error = err.Error()
		return compatibility, fmt.Errorf("select representative PodcastIndex row: %w", err)
	}
	if err := db.Close(); err != nil {
		compatibility.Error = err.Error()
		return compatibility, fmt.Errorf("close compatibility database: %w", err)
	}

	query, err := podcastindex.NewQuery(path)
	if err != nil {
		compatibility.Error = err.Error()
		return compatibility, err
	}
	defer query.Close()
	if info, err := query.FindByFeedURL(compatibility.URL); err != nil || info == nil {
		if err == nil {
			err = fmt.Errorf("URL lookup returned no row")
		}
		compatibility.Error = err.Error()
		return compatibility, fmt.Errorf("URL lookup compatibility failed: %w", err)
	}
	compatibility.URLChecked = true
	if results, err := query.FindByTitle(compatibility.Title); err != nil || len(results) == 0 {
		if err == nil {
			err = fmt.Errorf("title lookup returned no row")
		}
		compatibility.Error = err.Error()
		return compatibility, fmt.Errorf("title lookup compatibility failed: %w", err)
	}
	compatibility.TitleChecked = true
	if itunesID.Valid {
		compatibility.ITunesID = itunesID.Int64
		if info, err := query.FindByITunesID(int(itunesID.Int64)); err != nil || info == nil {
			if err == nil {
				err = fmt.Errorf("iTunes ID lookup returned no row")
			}
			compatibility.Error = err.Error()
			return compatibility, fmt.Errorf("iTunes ID lookup compatibility failed: %w", err)
		}
		compatibility.ITunesIDChecked = true
	} else {
		return compatibility, fmt.Errorf("candidate has no numeric iTunes ID row for compatibility check")
	}
	compatibility.Passed = true
	return compatibility, nil
}
