package database

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

type MigrationExecutionReport struct {
	Version         int                  `json:"version"`
	Name            string               `json:"name"`
	Status          string               `json:"status"`
	Contract        MigrationContract    `json:"contract"`
	DDL             []DDLChange          `json:"actual_ddl"`
	TableChanges    []TableDataChange    `json:"table_changes"`
	ProtectedTables []string             `json:"protected_tables"`
	Violations      []MigrationViolation `json:"violations,omitempty"`
}

type MigrationGateError struct {
	Migration Migration
	Report    MigrationExecutionReport
}

func (e *MigrationGateError) Error() string {
	details := make([]string, 0, len(e.Report.Violations))
	for _, violation := range e.Report.Violations {
		details = append(details, violation.Detail)
	}
	return fmt.Sprintf("migration %d (%s) rejected by data safety gate: %s", e.Migration.Version, e.Migration.Name, strings.Join(details, "; "))
}

// MigrationPostCommitError means SQLite committed the migration transaction,
// but the connection could not be returned to its required safety state.
// Callers must never report this as a rollback.
type MigrationPostCommitError struct {
	Err error
}

func (e *MigrationPostCommitError) Error() string { return e.Err.Error() }
func (e *MigrationPostCommitError) Unwrap() error { return e.Err }

type productionMigrationRunner struct {
	migrations         []Migration
	now                func() time.Time
	restoreForeignKeys func(*gorm.DB) error
}

func newProductionMigrationRunner() productionMigrationRunner {
	return newMigrationRunner(migrationRegistry())
}

func newMigrationRunner(migrations []Migration) productionMigrationRunner {
	cloned := append([]Migration(nil), migrations...)
	sort.Slice(cloned, func(i, j int) bool { return cloned[i].Version < cloned[j].Version })
	return productionMigrationRunner{
		migrations: cloned,
		now:        func() time.Time { return time.Now().UTC() },
		restoreForeignKeys: func(db *gorm.DB) error {
			return db.Exec("PRAGMA foreign_keys = ON").Error
		},
	}
}

func (runner productionMigrationRunner) run(db *gorm.DB) ([]MigrationExecutionReport, error) {
	return runner.runWithValidation(db, nil)
}

func (runner productionMigrationRunner) runWithValidation(db *gorm.DB, validateBeforeCommit func(*gorm.DB, []MigrationExecutionReport) error) ([]MigrationExecutionReport, error) {
	if db == nil {
		return nil, fmt.Errorf("database is nil")
	}
	reports := make([]MigrationExecutionReport, 0)
	apply := func(rawTx *gorm.DB) error {
		tx := rawTx.Session(&gorm.Session{NewDB: true})
		if err := tx.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
            version INTEGER PRIMARY KEY,
            name TEXT NOT NULL,
            applied_at DATETIME NOT NULL
        )`).Error; err != nil {
			return fmt.Errorf("create schema_migrations: %w", err)
		}
		current, err := currentSchemaVersion(tx)
		if err != nil {
			return err
		}
		for _, migration := range runner.migrations {
			if migration.Version <= current {
				continue
			}
			before, err := captureMigrationDatabaseSnapshot(tx)
			if err != nil {
				return err
			}
			if violations := validateMigrationContractDefinition(migration.Contract); len(violations) > 0 {
				report := MigrationExecutionReport{
					Version: migration.Version, Name: migration.Name, Status: "rejected",
					Contract: migration.Contract, ProtectedTables: migrationProtectedTables(before, nil),
					Violations: violations,
				}
				reports = append(reports, report)
				return &MigrationGateError{Migration: migration, Report: report}
			}
			capture := &migrationDDLCapture{}
			migrationDB := tx.Session(&gorm.Session{NewDB: true, Logger: capture})
			if err := migration.Apply(migrationDB); err != nil {
				return fmt.Errorf("migration %d (%s) failed: %w", migration.Version, migration.Name, err)
			}
			after, err := captureMigrationDatabaseSnapshot(tx)
			if err != nil {
				return err
			}
			report := MigrationExecutionReport{
				Version: migration.Version, Name: migration.Name, Contract: migration.Contract,
				DDL: capture.Changes(), TableChanges: migrationTableChanges(before, after),
			}
			report.ProtectedTables = migrationProtectedTables(before, report.DDL)
			report.Violations = validateMigrationContract(before, after, report.DDL, migration.Contract)
			report.Status = "passed"
			if len(report.Violations) > 0 {
				report.Status = "rejected"
			}
			reports = append(reports, report)
			if len(report.Violations) > 0 {
				return &MigrationGateError{Migration: migration, Report: report}
			}
			if err := tx.Create(&SchemaMigration{
				Version: migration.Version, Name: migration.Name, AppliedAt: runner.now(),
			}).Error; err != nil {
				return fmt.Errorf("record migration %d (%s): %w", migration.Version, migration.Name, err)
			}
			current = migration.Version
		}
		if validateBeforeCommit != nil {
			if err := validateBeforeCommit(tx, reports); err != nil {
				return err
			}
		}
		return nil
	}

	needsForeignKeysDisabled, err := migrationSetNeedsForeignKeysDisabled(db, runner.migrations)
	if err != nil {
		return nil, err
	}
	if !needsForeignKeysDisabled {
		err := db.Transaction(apply)
		return reports, err
	}
	err = db.Connection(func(conn *gorm.DB) error {
		var foreignKeys int
		if err := conn.Raw("PRAGMA foreign_keys").Row().Scan(&foreignKeys); err != nil {
			return fmt.Errorf("read sqlite foreign_keys pragma: %w", err)
		}
		if foreignKeys == 1 {
			if err := conn.Exec("PRAGMA foreign_keys = OFF").Error; err != nil {
				return fmt.Errorf("disable sqlite foreign_keys for migration: %w", err)
			}
		}
		migrationErr := conn.Transaction(apply)
		if foreignKeys == 1 {
			restoreForeignKeys := runner.restoreForeignKeys
			if restoreForeignKeys == nil {
				restoreForeignKeys = func(db *gorm.DB) error { return db.Exec("PRAGMA foreign_keys = ON").Error }
			}
			if restoreErr := restoreForeignKeys(conn); migrationErr == nil && restoreErr != nil {
				return &MigrationPostCommitError{Err: fmt.Errorf("restore sqlite foreign_keys after migration: %w", restoreErr)}
			}
		}
		return migrationErr
	})
	return reports, err
}

func validateMigrationContractDefinition(contract MigrationContract) []MigrationViolation {
	violations := make([]MigrationViolation, 0)
	validSchemaOperations := map[string]struct{}{
		SchemaChangeAddColumn: {}, SchemaChangeCreateTable: {}, SchemaChangeDropTable: {},
		SchemaChangeRebuildTable: {}, SchemaChangeCreateIndex: {}, SchemaChangeDropIndex: {},
		SchemaChangeCreateTrigger: {}, SchemaChangeDropTrigger: {},
	}
	for _, rule := range contract.SchemaChanges {
		_, valid := validSchemaOperations[rule.Operation]
		if !valid || rule.Table == "" || (rule.Operation == SchemaChangeAddColumn && rule.Object == "") {
			violations = append(violations, MigrationViolation{Code: "invalid_schema_contract", Table: rule.Table, Operation: rule.Operation, Detail: "migration schema contract is incomplete"})
		}
	}
	validDataOperations := map[string]struct{}{
		DataChangeBackfill: {}, DataChangeNormalize: {}, DataChangeInsert: {}, DataChangeDelete: {},
	}
	validConditions := map[string]struct{}{
		DataConditionAny: {}, DataConditionNonEmpty: {}, DataConditionAllowedValues: {},
	}
	for _, rule := range contract.DataChanges {
		_, validOperation := validDataOperations[rule.Operation]
		_, validCondition := validConditions[rule.Condition.Type]
		needsColumns := rule.Operation == DataChangeBackfill || rule.Operation == DataChangeNormalize ||
			(rule.Condition.Type != "" && rule.Condition.Type != DataConditionAny)
		if !validOperation || !validCondition || rule.Table == "" || rule.MaxRows <= 0 || (needsColumns && len(rule.Columns) == 0) || (rule.Condition.Type == DataConditionAllowedValues && len(rule.Condition.Values) == 0) {
			violations = append(violations, MigrationViolation{Code: "invalid_data_contract", Table: rule.Table, Operation: rule.Operation, Detail: "migration data contract must declare table, operation, bounded rows and a verifiable condition"})
		}
	}
	return violations
}

func validateMigrationContract(before, after migrationDatabaseSnapshot, ddl []DDLChange, contract MigrationContract) []MigrationViolation {
	schemaChanges := migrationSchemaChanges(before, after)
	violations := validateMigrationDDL(ddl, contract.SchemaChanges)
	violations = append(violations, validateMigrationSchemaDiff(schemaChanges, contract.SchemaChanges)...)
	violations = append(violations, validateDeclaredSchemaChanges(ddl, schemaChanges, contract.SchemaChanges)...)
	rulesByTable := make(map[string][]DataChangeRule)
	for _, rule := range contract.DataChanges {
		rulesByTable[rule.Table] = append(rulesByTable[rule.Table], rule)
	}
	allTables := make(map[string]struct{}, len(before.Tables)+len(after.Tables))
	for table := range before.Tables {
		allTables[table] = struct{}{}
	}
	for table := range after.Tables {
		allTables[table] = struct{}{}
	}
	for table := range allTables {
		oldTable, hadOld := before.Tables[table]
		newTable, hasNew := after.Tables[table]
		rules := rulesByTable[table]
		if !hadOld {
			oldTable = migrationTableSnapshot{Rows: make(map[string]migrationRow)}
		}
		if !hasNew {
			newTable = migrationTableSnapshot{Rows: make(map[string]migrationRow)}
		}
		violations = append(violations, validateMigrationTableData(table, oldTable, newTable, rules)...)
	}
	sort.Slice(violations, func(i, j int) bool {
		if violations[i].Table == violations[j].Table {
			return violations[i].Code < violations[j].Code
		}
		return violations[i].Table < violations[j].Table
	})
	return violations
}

func validateMigrationDDL(observed []DDLChange, allowed []SchemaChangeRule) []MigrationViolation {
	violations := make([]MigrationViolation, 0)
	for _, change := range observed {
		matched := false
		for _, rule := range allowed {
			if migrationDDLMatchesRule(change, rule) {
				matched = true
				break
			}
		}
		if matched {
			continue
		}
		code := "undeclared_schema_change"
		if change.Operation == SchemaChangeDropTable || change.Operation == SchemaChangeRebuildTable {
			code = "undeclared_table_rebuild"
		}
		violations = append(violations, MigrationViolation{
			Code: code, Table: change.Table, Operation: change.Operation,
			Detail: fmt.Sprintf("migration performed undeclared %s on table %s", change.Operation, change.Table),
		})
	}
	return violations
}

func migrationDDLMatchesRule(change DDLChange, rule SchemaChangeRule) bool {
	if change.Operation == SchemaChangeUnsupported {
		return false
	}
	if rule.Operation == SchemaChangeRebuildTable &&
		(change.Operation == SchemaChangeRebuildTable || change.Operation == SchemaChangeDropTable) &&
		change.Table == rule.Table {
		return true
	}
	return change.Operation == rule.Operation && change.Table == rule.Table &&
		(rule.Object == "" || change.Object == rule.Object)
}

func validateMigrationSchemaDiff(observed []SchemaObjectChange, allowed []SchemaChangeRule) []MigrationViolation {
	violations := make([]MigrationViolation, 0)
	for _, change := range observed {
		matched := false
		for _, rule := range allowed {
			if migrationSchemaDiffMatchesRule(change, rule) {
				matched = true
				break
			}
		}
		if matched {
			continue
		}
		violations = append(violations, MigrationViolation{
			Code: "undeclared_schema_diff", Table: change.Table, Operation: change.Operation,
			Detail: fmt.Sprintf("migration produced undeclared %s for %s %s", change.Operation, change.Type, change.Object),
		})
	}
	return violations
}

func migrationSchemaDiffMatchesRule(change SchemaObjectChange, rule SchemaChangeRule) bool {
	switch rule.Operation {
	case SchemaChangeAddColumn:
		return change.Operation == SchemaChangeAddColumn && change.Type == "column" &&
			change.Table == rule.Table && change.Object == rule.Object
	case SchemaChangeCreateTable:
		return change.Operation == "create" && change.Type == "table" &&
			change.Object == rule.Table
	case SchemaChangeDropTable:
		return change.Operation == "drop" && change.Type == "table" &&
			change.Object == rule.Table
	case SchemaChangeCreateIndex:
		return change.Operation == "create" && change.Type == "index" &&
			change.Table == rule.Table && (rule.Object == "" || change.Object == rule.Object)
	case SchemaChangeDropIndex:
		return change.Operation == "drop" && change.Type == "index" &&
			change.Table == rule.Table && (rule.Object == "" || change.Object == rule.Object)
	case SchemaChangeCreateTrigger:
		return change.Operation == "create" && change.Type == "trigger" &&
			change.Table == rule.Table && (rule.Object == "" || change.Object == rule.Object)
	case SchemaChangeDropTrigger:
		return change.Operation == "drop" && change.Type == "trigger" &&
			change.Table == rule.Table && (rule.Object == "" || change.Object == rule.Object)
	}
	return false
}

func validateDeclaredSchemaChanges(ddl []DDLChange, schemaChanges []SchemaObjectChange, declared []SchemaChangeRule) []MigrationViolation {
	violations := make([]MigrationViolation, 0)
	for _, rule := range declared {
		observed := false
		for _, change := range ddl {
			if migrationDDLMatchesRule(change, rule) {
				observed = true
				break
			}
		}
		if !observed {
			for _, change := range schemaChanges {
				if migrationSchemaDiffMatchesRule(change, rule) {
					observed = true
					break
				}
			}
		}
		if !observed {
			violations = append(violations, MigrationViolation{
				Code: "declared_schema_change_missing", Table: rule.Table, Operation: rule.Operation,
				Detail: fmt.Sprintf("migration did not perform declared %s on table %s", rule.Operation, rule.Table),
			})
		}
	}
	return violations
}

func validateMigrationTableData(table string, before, after migrationTableSnapshot, rules []DataChangeRule) []MigrationViolation {
	allowedByColumn := make(map[string][]int)
	for index, rule := range rules {
		if rule.Operation != DataChangeBackfill && rule.Operation != DataChangeNormalize {
			continue
		}
		for _, column := range rule.Columns {
			allowedByColumn[column] = append(allowedByColumn[column], index)
		}
	}
	removed := int64(0)
	added := int64(0)
	unmatchedRemoved := int64(0)
	unmatchedAdded := int64(0)
	undeclaredUpdates := int64(0)
	changedByRule := make(map[int]map[string]struct{})
	for identity, oldRow := range before.Rows {
		newRow, exists := after.Rows[identity]
		if !exists {
			removed++
			matched := false
			for ruleIndex, rule := range rules {
				if rule.Operation != DataChangeDelete || !migrationRowSatisfies(oldRow, rule.Columns, rule.Condition) {
					continue
				}
				matched = true
				recordMigrationRuleMatch(changedByRule, ruleIndex, identity)
				break
			}
			if !matched {
				unmatchedRemoved++
			}
			continue
		}
		for column, oldValue := range oldRow.Values {
			newValue, stillExists := newRow.Values[column]
			if !stillExists {
				undeclaredUpdates++
				continue
			}
			if newValue == oldValue {
				continue
			}
			matched := false
			for _, ruleIndex := range allowedByColumn[column] {
				if rules[ruleIndex].Operation == DataChangeBackfill && !migrationValueIsEmpty(oldValue) {
					continue
				}
				if !migrationValueSatisfies(newValue, rules[ruleIndex].Condition) {
					continue
				}
				matched = true
				recordMigrationRuleMatch(changedByRule, ruleIndex, identity)
				break
			}
			if !matched {
				undeclaredUpdates++
			}
		}
	}
	for identity, newRow := range after.Rows {
		if _, exists := before.Rows[identity]; exists {
			continue
		}
		added++
		matched := false
		for ruleIndex, rule := range rules {
			if rule.Operation != DataChangeInsert || !migrationRowSatisfies(newRow, rule.Columns, rule.Condition) {
				continue
			}
			matched = true
			recordMigrationRuleMatch(changedByRule, ruleIndex, identity)
			break
		}
		if !matched {
			unmatchedAdded++
		}
	}

	violations := make([]MigrationViolation, 0, 4)
	if unmatchedRemoved > 0 {
		code := "undeclared_data_delete"
		if _, protected := explicitMigrationProtectedTables[table]; protected {
			code = "protected_data_decreased"
		}
		detail := fmt.Sprintf("table %s protected rows decreased %d->%d", table, before.Summary.Rows, after.Summary.Rows)
		if migrationRulesAllow(rules, DataChangeDelete) {
			code = "data_change_condition_failed"
			detail = fmt.Sprintf("table %s deleted %d row(s) outside the declared condition", table, unmatchedRemoved)
		}
		violations = append(violations, MigrationViolation{Code: code, Table: table, Operation: DataChangeDelete, Detail: detail})
	}
	if unmatchedAdded > 0 {
		code := "undeclared_data_insert"
		detail := fmt.Sprintf("table %s gained undeclared rows %d->%d", table, before.Summary.Rows, after.Summary.Rows)
		if migrationRulesAllow(rules, DataChangeInsert) {
			code = "data_change_condition_failed"
			detail = fmt.Sprintf("table %s inserted %d row(s) outside the declared condition", table, unmatchedAdded)
		}
		violations = append(violations, MigrationViolation{Code: code, Table: table, Operation: DataChangeInsert, Detail: detail})
	}
	if undeclaredUpdates > 0 {
		violations = append(violations, MigrationViolation{Code: "undeclared_data_update", Table: table, Operation: "update", Detail: fmt.Sprintf("table %s changed %d protected value(s)", table, undeclaredUpdates)})
	}
	for index, rule := range rules {
		if rule.MaxRows <= 0 {
			continue
		}
		actual := int64(0)
		switch rule.Operation {
		case DataChangeBackfill, DataChangeNormalize:
			actual = int64(len(changedByRule[index]))
		case DataChangeInsert:
			actual = added
		case DataChangeDelete:
			actual = removed
		}
		if actual > rule.MaxRows {
			violations = append(violations, MigrationViolation{Code: "data_change_limit_exceeded", Table: table, Operation: rule.Operation, Detail: fmt.Sprintf("table %s %s changed %d row(s), limit %d", table, rule.Operation, actual, rule.MaxRows)})
		}
	}
	return violations
}

func recordMigrationRuleMatch(changedByRule map[int]map[string]struct{}, ruleIndex int, identity string) {
	if changedByRule[ruleIndex] == nil {
		changedByRule[ruleIndex] = make(map[string]struct{})
	}
	changedByRule[ruleIndex][identity] = struct{}{}
}

func migrationRowSatisfies(row migrationRow, columns []string, condition DataChangeCondition) bool {
	if condition.Type == "" || condition.Type == DataConditionAny {
		return true
	}
	if len(columns) == 0 {
		return false
	}
	for _, column := range columns {
		value, exists := row.Values[column]
		if !exists || !migrationValueSatisfies(value, condition) {
			return false
		}
	}
	return true
}

func migrationValueIsEmpty(encoded string) bool {
	return encoded == "null" || encoded == "string:" || encoded == "bytes:"
}

func migrationValueSatisfies(encoded string, condition DataChangeCondition) bool {
	switch condition.Type {
	case "", DataConditionAny:
		return true
	case DataConditionNonEmpty:
		return encoded != "null" && encoded != "string:" && encoded != "bytes:"
	case DataConditionAllowedValues:
		for _, value := range condition.Values {
			if encoded == "string:"+value {
				return true
			}
		}
	}
	return false
}

func migrationRulesAllow(rules []DataChangeRule, operation string) bool {
	for _, rule := range rules {
		if rule.Operation == operation {
			return true
		}
	}
	return false
}

type migrationDDLCapture struct {
	changes []DDLChange
}

func (capture *migrationDDLCapture) LogMode(gormlogger.LogLevel) gormlogger.Interface { return capture }
func (capture *migrationDDLCapture) Info(context.Context, string, ...interface{})     {}
func (capture *migrationDDLCapture) Warn(context.Context, string, ...interface{})     {}
func (capture *migrationDDLCapture) Error(context.Context, string, ...interface{})    {}
func (capture *migrationDDLCapture) Trace(_ context.Context, _ time.Time, query func() (string, int64), _ error) {
	sqlText, _ := query()
	if change, ok := classifyMigrationDDL(sqlText); ok {
		capture.changes = append(capture.changes, change)
	}
}

func (capture *migrationDDLCapture) Changes() []DDLChange {
	return append([]DDLChange(nil), capture.changes...)
}

var (
	ddlAddColumn     = regexp.MustCompile(`(?i)^ALTER\s+TABLE\s+[^A-Za-z0-9_]*([A-Za-z0-9_]+)[^A-Za-z0-9_]+ADD(?:\s+COLUMN)?\s+[^A-Za-z0-9_]*([A-Za-z0-9_]+)`)
	ddlCreateTable   = regexp.MustCompile(`(?i)^CREATE\s+TABLE(?:\s+IF\s+NOT\s+EXISTS)?\s+[^A-Za-z0-9_]*([A-Za-z0-9_]+)`)
	ddlDropTable     = regexp.MustCompile(`(?i)^DROP\s+TABLE(?:\s+IF\s+EXISTS)?\s+[^A-Za-z0-9_]*([A-Za-z0-9_]+)`)
	ddlRenameTable   = regexp.MustCompile(`(?i)^ALTER\s+TABLE\s+[^A-Za-z0-9_]*([A-Za-z0-9_]+)[^A-Za-z0-9_]+RENAME\s+TO\s+[^A-Za-z0-9_]*([A-Za-z0-9_]+)`)
	ddlCreateIndex   = regexp.MustCompile(`(?i)^CREATE\s+(?:UNIQUE\s+)?INDEX(?:\s+IF\s+NOT\s+EXISTS)?\s+[^A-Za-z0-9_]*([A-Za-z0-9_]+)[^A-Za-z0-9_]+ON\s+[^A-Za-z0-9_]*([A-Za-z0-9_]+)`)
	ddlDropIndex     = regexp.MustCompile(`(?i)^DROP\s+INDEX(?:\s+IF\s+EXISTS)?\s+[^A-Za-z0-9_]*([A-Za-z0-9_]+)`)
	ddlCreateTrigger = regexp.MustCompile(`(?i)^CREATE\s+TRIGGER(?:\s+IF\s+NOT\s+EXISTS)?\s+[^A-Za-z0-9_]*([A-Za-z0-9_]+)`)
	ddlDropTrigger   = regexp.MustCompile(`(?i)^DROP\s+TRIGGER(?:\s+IF\s+EXISTS)?\s+[^A-Za-z0-9_]*([A-Za-z0-9_]+)`)
	ddlUnknownSchema = regexp.MustCompile(`(?i)^(?:CREATE|ALTER|DROP|REINDEX|VACUUM|PRAGMA|ANALYZE|ATTACH|DETACH)\b`)
)

func classifyMigrationDDL(sqlText string) (DDLChange, bool) {
	normalized := strings.TrimSpace(sqlText)
	if matches := ddlAddColumn.FindStringSubmatch(normalized); len(matches) == 3 {
		return DDLChange{Operation: SchemaChangeAddColumn, Table: matches[1], Object: matches[2]}, true
	}
	if matches := ddlRenameTable.FindStringSubmatch(normalized); len(matches) == 3 {
		table := matches[2]
		if strings.HasSuffix(matches[1], "__temp") {
			return DDLChange{Operation: SchemaChangeRebuildTable, Table: table}, true
		}
		return DDLChange{Operation: SchemaChangeRebuildTable, Table: table}, true
	}
	if matches := ddlCreateTable.FindStringSubmatch(normalized); len(matches) == 2 {
		table := matches[1]
		if strings.HasSuffix(table, "__temp") {
			return DDLChange{Operation: SchemaChangeRebuildTable, Table: strings.TrimSuffix(table, "__temp")}, true
		}
		return DDLChange{Operation: SchemaChangeCreateTable, Table: table}, true
	}
	if matches := ddlDropTable.FindStringSubmatch(normalized); len(matches) == 2 {
		return DDLChange{Operation: SchemaChangeDropTable, Table: matches[1]}, true
	}
	if matches := ddlCreateIndex.FindStringSubmatch(normalized); len(matches) == 3 {
		return DDLChange{Operation: SchemaChangeCreateIndex, Table: matches[2], Object: matches[1]}, true
	}
	if matches := ddlDropIndex.FindStringSubmatch(normalized); len(matches) == 2 {
		return DDLChange{Operation: SchemaChangeDropIndex, Object: matches[1]}, true
	}
	if matches := ddlCreateTrigger.FindStringSubmatch(normalized); len(matches) == 2 {
		return DDLChange{Operation: SchemaChangeCreateTrigger, Object: matches[1]}, true
	}
	if matches := ddlDropTrigger.FindStringSubmatch(normalized); len(matches) == 2 {
		return DDLChange{Operation: SchemaChangeDropTrigger, Object: matches[1]}, true
	}
	if ddlUnknownSchema.MatchString(normalized) {
		return DDLChange{Operation: SchemaChangeUnsupported}, true
	}
	return DDLChange{}, false
}
