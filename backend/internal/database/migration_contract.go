package database

const (
	SchemaChangeAddColumn     = "add_column"
	SchemaChangeCreateTable   = "create_table"
	SchemaChangeDropTable     = "drop_table"
	SchemaChangeRebuildTable  = "rebuild_table"
	SchemaChangeCreateIndex   = "create_index"
	SchemaChangeDropIndex     = "drop_index"
	SchemaChangeCreateTrigger = "create_trigger"
	SchemaChangeDropTrigger   = "drop_trigger"
	SchemaChangeUnsupported   = "unsupported_schema_change"

	DataChangeBackfill  = "backfill"
	DataChangeNormalize = "normalize"
	DataChangeInsert    = "insert"
	DataChangeDelete    = "delete"

	DataConditionAny           = "any"
	DataConditionNonEmpty      = "non_empty"
	DataConditionAllowedValues = "allowed_values"
)

// SchemaChangeRule is the narrow schema authority granted to one migration.
// Existing business tables receive no implicit authority.
type SchemaChangeRule struct {
	Operation string `json:"operation"`
	Table     string `json:"table"`
	Object    string `json:"object,omitempty"`
}

// DataChangeCondition makes an allowed rewrite mechanically verifiable.
// Values are migration-owned canonical values, never copied from user data.
type DataChangeCondition struct {
	Type   string   `json:"type"`
	Values []string `json:"values,omitempty"`
}

// DataChangeRule permits only the named table, columns, operation and bound.
// All existing row identities and unlisted columns remain protected.
type DataChangeRule struct {
	Operation string              `json:"operation"`
	Table     string              `json:"table"`
	Columns   []string            `json:"columns,omitempty"`
	MaxRows   int64               `json:"max_rows,omitempty"`
	Condition DataChangeCondition `json:"condition"`
}

// MigrationContract defaults to zero data change and zero undeclared DDL.
type MigrationContract struct {
	SchemaChanges []SchemaChangeRule `json:"schema_changes,omitempty"`
	DataChanges   []DataChangeRule   `json:"data_changes,omitempty"`
}

type DDLChange struct {
	Operation string `json:"operation"`
	Table     string `json:"table,omitempty"`
	Object    string `json:"object,omitempty"`
}

type SchemaObjectChange struct {
	Operation string `json:"operation"`
	Type      string `json:"type"`
	Table     string `json:"table,omitempty"`
	Object    string `json:"object"`
	BeforeSHA string `json:"before_sha256,omitempty"`
	AfterSHA  string `json:"after_sha256,omitempty"`
}

type TableDataSummary struct {
	Table          string `json:"table"`
	Rows           int64  `json:"rows"`
	IdentitySHA256 string `json:"identity_sha256"`
	ContentSHA256  string `json:"content_sha256"`
}

type TableDataChange struct {
	Table                  string `json:"table"`
	BeforeRows             int64  `json:"before_rows"`
	AfterRows              int64  `json:"after_rows"`
	ExistingIdentitiesKept bool   `json:"existing_identities_kept"`
	ExistingContentKept    bool   `json:"existing_content_kept"`
}

type ForeignKeyEdge struct {
	Parent string `json:"parent"`
	Child  string `json:"child"`
}

type MigrationViolation struct {
	Code      string `json:"code"`
	Table     string `json:"table,omitempty"`
	Operation string `json:"operation,omitempty"`
	Detail    string `json:"detail"`
}

var explicitMigrationProtectedTables = map[string]struct{}{
	"podcasts":                  {},
	"podcasts_tags":             {},
	"episodes":                  {},
	"episodes_tags":             {},
	"episode_triage_decisions":  {},
	"consumption_queue_orders":  {},
	"episode_completions":       {},
	"episode_processing_runs":   {},
	"processing_checkpoints":    {},
	"episode_artifact_sets":     {},
	"knowledge_deliveries":      {},
	"episode_audio_assets":      {},
	"processing_schedule_runs":  {},
	"processing_schedule_items": {},
}
