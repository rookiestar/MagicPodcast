package database

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

type migrationColumn struct {
	Name       string
	Type       string
	NotNull    bool
	DefaultSQL string
	PrimaryKey int
}

type migrationRow struct {
	Values map[string]string
}

type migrationTableSnapshot struct {
	Columns      []migrationColumn
	IdentityCols []string
	Rows         map[string]migrationRow
	Summary      TableDataSummary
}

type migrationSchemaObject struct {
	Type  string
	Name  string
	Table string
	SHA   string
}

type migrationDatabaseSnapshot struct {
	Schema      map[string]migrationSchemaObject
	Tables      map[string]migrationTableSnapshot
	ForeignKeys []ForeignKeyEdge
}

func captureMigrationDatabaseSnapshot(db *gorm.DB) (migrationDatabaseSnapshot, error) {
	snapshot := migrationDatabaseSnapshot{
		Schema: make(map[string]migrationSchemaObject),
		Tables: make(map[string]migrationTableSnapshot),
	}
	type schemaRow struct {
		Type    string
		Name    string
		TblName string         `gorm:"column:tbl_name"`
		SQL     sql.NullString `gorm:"column:sql"`
	}
	var objects []schemaRow
	if err := db.Raw(`
		SELECT type, name, tbl_name, sql
		FROM sqlite_master
		WHERE name NOT LIKE 'sqlite_%'
		  AND type IN ('table', 'index', 'trigger', 'view')
		ORDER BY type, name
	`).Scan(&objects).Error; err != nil {
		return migrationDatabaseSnapshot{}, fmt.Errorf("inspect sqlite schema objects: %w", err)
	}
	for _, object := range objects {
		sqlText := ""
		if object.SQL.Valid {
			sqlText = object.SQL.String
		}
		snapshot.Schema[object.Type+":"+object.Name] = migrationSchemaObject{
			Type: object.Type, Name: object.Name, Table: object.TblName,
			SHA: hashMigrationStrings([]string{sqlText}),
		}
		if object.Type != "table" {
			continue
		}
		table, err := captureMigrationTable(db, object.Name)
		if err != nil {
			return migrationDatabaseSnapshot{}, err
		}
		snapshot.Tables[object.Name] = table
		edges, err := inspectMigrationForeignKeys(db, object.Name)
		if err != nil {
			return migrationDatabaseSnapshot{}, err
		}
		snapshot.ForeignKeys = append(snapshot.ForeignKeys, edges...)
	}
	sort.Slice(snapshot.ForeignKeys, func(i, j int) bool {
		if snapshot.ForeignKeys[i].Parent == snapshot.ForeignKeys[j].Parent {
			return snapshot.ForeignKeys[i].Child < snapshot.ForeignKeys[j].Child
		}
		return snapshot.ForeignKeys[i].Parent < snapshot.ForeignKeys[j].Parent
	})
	return snapshot, nil
}

func captureMigrationTable(db *gorm.DB, tableName string) (migrationTableSnapshot, error) {
	type columnRow struct {
		CID        int            `gorm:"column:cid"`
		Name       string         `gorm:"column:name"`
		Type       string         `gorm:"column:type"`
		NotNull    int            `gorm:"column:notnull"`
		Default    sql.NullString `gorm:"column:dflt_value"`
		PrimaryKey int            `gorm:"column:pk"`
	}
	var columnRows []columnRow
	if err := db.Raw("PRAGMA table_info(" + quoteMigrationIdentifier(tableName) + ")").Scan(&columnRows).Error; err != nil {
		return migrationTableSnapshot{}, fmt.Errorf("inspect table %s columns: %w", tableName, err)
	}
	sort.Slice(columnRows, func(i, j int) bool { return columnRows[i].CID < columnRows[j].CID })
	table := migrationTableSnapshot{Rows: make(map[string]migrationRow)}
	primaryKeys := make(map[int]string)
	for _, column := range columnRows {
		defaultSQL := ""
		if column.Default.Valid {
			defaultSQL = column.Default.String
		}
		table.Columns = append(table.Columns, migrationColumn{
			Name: column.Name, Type: column.Type, NotNull: column.NotNull != 0,
			DefaultSQL: defaultSQL, PrimaryKey: column.PrimaryKey,
		})
		if column.PrimaryKey > 0 {
			primaryKeys[column.PrimaryKey] = column.Name
		}
	}
	for order := 1; order <= len(primaryKeys); order++ {
		if name, ok := primaryKeys[order]; ok {
			table.IdentityCols = append(table.IdentityCols, name)
		}
	}
	if len(table.IdentityCols) == 0 {
		for _, column := range table.Columns {
			table.IdentityCols = append(table.IdentityCols, column.Name)
		}
	}

	rows, err := db.Raw("SELECT * FROM " + quoteMigrationIdentifier(tableName)).Rows()
	if err != nil {
		return migrationTableSnapshot{}, fmt.Errorf("read table %s for migration snapshot: %w", tableName, err)
	}
	defer rows.Close()
	columnNames, err := rows.Columns()
	if err != nil {
		return migrationTableSnapshot{}, fmt.Errorf("read table %s column names: %w", tableName, err)
	}
	for rows.Next() {
		values := make([]any, len(columnNames))
		destinations := make([]any, len(columnNames))
		for i := range values {
			destinations[i] = &values[i]
		}
		if err := rows.Scan(destinations...); err != nil {
			return migrationTableSnapshot{}, fmt.Errorf("scan table %s migration snapshot: %w", tableName, err)
		}
		encoded := make(map[string]string, len(columnNames))
		for i, name := range columnNames {
			encoded[name] = encodeMigrationValue(values[i])
		}
		identityParts := make([]string, 0, len(table.IdentityCols))
		for _, name := range table.IdentityCols {
			identityParts = append(identityParts, name+"="+encoded[name])
		}
		identity := hashMigrationStrings(identityParts)
		if _, exists := table.Rows[identity]; exists {
			return migrationTableSnapshot{}, fmt.Errorf("table %s has duplicate migration identity", tableName)
		}
		table.Rows[identity] = migrationRow{Values: encoded}
	}
	if err := rows.Err(); err != nil {
		return migrationTableSnapshot{}, fmt.Errorf("iterate table %s migration snapshot: %w", tableName, err)
	}
	table.Summary = summarizeMigrationTable(tableName, table)
	return table, nil
}

func inspectMigrationForeignKeys(db *gorm.DB, child string) ([]ForeignKeyEdge, error) {
	type foreignKeyRow struct {
		Table string `gorm:"column:table"`
	}
	var rows []foreignKeyRow
	if err := db.Raw("PRAGMA foreign_key_list(" + quoteMigrationIdentifier(child) + ")").Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("inspect table %s foreign keys: %w", child, err)
	}
	edges := make([]ForeignKeyEdge, 0, len(rows))
	seen := make(map[string]struct{})
	for _, row := range rows {
		key := row.Table + "\x00" + child
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		edges = append(edges, ForeignKeyEdge{Parent: row.Table, Child: child})
	}
	return edges, nil
}

func summarizeMigrationTable(name string, table migrationTableSnapshot) TableDataSummary {
	identities := make([]string, 0, len(table.Rows))
	contents := make([]string, 0, len(table.Rows))
	for identity, row := range table.Rows {
		identities = append(identities, identity)
		parts := []string{identity}
		for _, column := range table.Columns {
			parts = append(parts, column.Name+"="+row.Values[column.Name])
		}
		contents = append(contents, strings.Join(parts, "\x1f"))
	}
	sort.Strings(identities)
	sort.Strings(contents)
	return TableDataSummary{
		Table: name, Rows: int64(len(table.Rows)),
		IdentitySHA256: hashMigrationStrings(identities),
		ContentSHA256:  hashMigrationStrings(contents),
	}
}

func migrationSnapshotSummaries(snapshot migrationDatabaseSnapshot) []TableDataSummary {
	summaries := make([]TableDataSummary, 0, len(snapshot.Tables))
	for _, table := range snapshot.Tables {
		summaries = append(summaries, table.Summary)
	}
	sort.Slice(summaries, func(i, j int) bool { return summaries[i].Table < summaries[j].Table })
	return summaries
}

func migrationDatabaseFingerprint(snapshot migrationDatabaseSnapshot) string {
	parts := make([]string, 0, len(snapshot.Schema)+len(snapshot.Tables)*3)
	for key, object := range snapshot.Schema {
		parts = append(parts, "schema:"+key+":"+object.SHA)
	}
	for table, state := range snapshot.Tables {
		parts = append(parts,
			"table:"+table+":rows:"+strconv.FormatInt(state.Summary.Rows, 10),
			"table:"+table+":identity:"+state.Summary.IdentitySHA256,
			"table:"+table+":content:"+state.Summary.ContentSHA256,
		)
	}
	sort.Strings(parts)
	return hashMigrationStrings(parts)
}

func migrationExistingRowsKept(before, after migrationTableSnapshot, allowedColumns map[string]struct{}) (bool, bool) {
	identitiesKept := true
	contentKept := true
	for identity, oldRow := range before.Rows {
		newRow, exists := after.Rows[identity]
		if !exists {
			identitiesKept = false
			contentKept = false
			continue
		}
		for column, oldValue := range oldRow.Values {
			if _, allowed := allowedColumns[column]; allowed {
				continue
			}
			if newValue, exists := newRow.Values[column]; !exists || newValue != oldValue {
				contentKept = false
			}
		}
	}
	return identitiesKept, contentKept
}

func migrationTableChanges(before, after migrationDatabaseSnapshot) []TableDataChange {
	all := make(map[string]struct{}, len(before.Tables)+len(after.Tables))
	for table := range before.Tables {
		all[table] = struct{}{}
	}
	for table := range after.Tables {
		all[table] = struct{}{}
	}
	tables := make([]string, 0, len(all))
	for table := range all {
		tables = append(tables, table)
	}
	sort.Strings(tables)
	changes := make([]TableDataChange, 0, len(tables))
	for _, table := range tables {
		oldTable := before.Tables[table]
		newTable := after.Tables[table]
		identitiesKept, contentKept := migrationExistingRowsKept(oldTable, newTable, nil)
		changes = append(changes, TableDataChange{
			Table: table, BeforeRows: oldTable.Summary.Rows, AfterRows: newTable.Summary.Rows,
			ExistingIdentitiesKept: identitiesKept, ExistingContentKept: contentKept,
		})
	}
	return changes
}

func migrationSchemaChanges(before, after migrationDatabaseSnapshot) []SchemaObjectChange {
	all := make(map[string]struct{}, len(before.Schema)+len(after.Schema))
	for key := range before.Schema {
		all[key] = struct{}{}
	}
	for key := range after.Schema {
		all[key] = struct{}{}
	}
	keys := make([]string, 0, len(all))
	for key := range all {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	changes := make([]SchemaObjectChange, 0)
	for _, key := range keys {
		oldObject, hadOld := before.Schema[key]
		newObject, hasNew := after.Schema[key]
		switch {
		case !hadOld:
			changes = append(changes, SchemaObjectChange{Operation: "create", Type: newObject.Type, Table: newObject.Table, Object: newObject.Name, AfterSHA: newObject.SHA})
		case !hasNew:
			changes = append(changes, SchemaObjectChange{Operation: "drop", Type: oldObject.Type, Table: oldObject.Table, Object: oldObject.Name, BeforeSHA: oldObject.SHA})
		case oldObject.SHA != newObject.SHA:
			if oldObject.Type == "table" {
				columnChanges := migrationColumnChanges(oldObject.Name, before.Tables[oldObject.Name], after.Tables[newObject.Name])
				if len(columnChanges) > 0 {
					changes = append(changes, columnChanges...)
					continue
				}
			}
			changes = append(changes, SchemaObjectChange{Operation: "modify", Type: newObject.Type, Table: newObject.Table, Object: newObject.Name, BeforeSHA: oldObject.SHA, AfterSHA: newObject.SHA})
		}
	}
	return changes
}

func migrationColumnChanges(table string, before, after migrationTableSnapshot) []SchemaObjectChange {
	oldColumns := make(map[string]migrationColumn, len(before.Columns))
	newColumns := make(map[string]migrationColumn, len(after.Columns))
	for _, column := range before.Columns {
		oldColumns[column.Name] = column
	}
	for _, column := range after.Columns {
		newColumns[column.Name] = column
	}
	changes := make([]SchemaObjectChange, 0)
	for name, column := range oldColumns {
		newColumn, exists := newColumns[name]
		if !exists {
			changes = append(changes, SchemaObjectChange{Operation: "drop_column", Type: "column", Table: table, Object: name, BeforeSHA: hashMigrationStrings([]string{fmt.Sprint(column)})})
			continue
		}
		if column != newColumn {
			changes = append(changes, SchemaObjectChange{Operation: "modify_column", Type: "column", Table: table, Object: name, BeforeSHA: hashMigrationStrings([]string{fmt.Sprint(column)}), AfterSHA: hashMigrationStrings([]string{fmt.Sprint(newColumn)})})
		}
	}
	for name, column := range newColumns {
		if _, exists := oldColumns[name]; !exists {
			changes = append(changes, SchemaObjectChange{Operation: SchemaChangeAddColumn, Type: "column", Table: table, Object: name, AfterSHA: hashMigrationStrings([]string{fmt.Sprint(column)})})
		}
	}
	sort.Slice(changes, func(i, j int) bool {
		if changes[i].Operation == changes[j].Operation {
			return changes[i].Object < changes[j].Object
		}
		return changes[i].Operation < changes[j].Operation
	})
	return changes
}

func migrationProtectedTables(snapshot migrationDatabaseSnapshot, ddl []DDLChange) []string {
	protected := make(map[string]struct{}, len(explicitMigrationProtectedTables))
	for table := range explicitMigrationProtectedTables {
		if _, exists := snapshot.Tables[table]; exists {
			protected[table] = struct{}{}
		}
	}
	children := make(map[string][]string)
	for _, edge := range snapshot.ForeignKeys {
		children[edge.Parent] = append(children[edge.Parent], edge.Child)
	}
	queue := make([]string, 0)
	for _, change := range ddl {
		if change.Table != "" {
			queue = append(queue, change.Table)
		}
	}
	visited := make(map[string]struct{})
	for len(queue) > 0 {
		table := queue[0]
		queue = queue[1:]
		if _, seen := visited[table]; seen {
			continue
		}
		visited[table] = struct{}{}
		protected[table] = struct{}{}
		queue = append(queue, children[table]...)
	}
	result := make([]string, 0, len(protected))
	for table := range protected {
		result = append(result, table)
	}
	sort.Strings(result)
	return result
}

func quoteMigrationIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

func encodeMigrationValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return "null"
	case []byte:
		return "bytes:" + hex.EncodeToString(typed)
	case string:
		return "string:" + typed
	case int64:
		return "int:" + strconv.FormatInt(typed, 10)
	case float64:
		return "float:" + strconv.FormatFloat(typed, 'g', -1, 64)
	case bool:
		return "bool:" + strconv.FormatBool(typed)
	case time.Time:
		return "time:" + typed.UTC().Format(time.RFC3339Nano)
	default:
		return fmt.Sprintf("%T:%v", value, value)
	}
}

func hashMigrationStrings(values []string) string {
	hash := sha256.New()
	for _, value := range values {
		_, _ = hash.Write([]byte(strconv.Itoa(len(value))))
		_, _ = hash.Write([]byte{':'})
		_, _ = hash.Write([]byte(value))
	}
	return hex.EncodeToString(hash.Sum(nil))
}
