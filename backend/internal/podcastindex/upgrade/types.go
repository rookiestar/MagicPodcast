package upgrade

import "time"

const (
	DefaultDownloadURL       = "https://public.podcastindex.org/podcastindex_feeds.db.tgz"
	DefaultFailureSince      = "2026-06-02"
	ManifestSchemaVersion    = 1
	CutoverConfirmation      = "I_UNDERSTAND_PODCASTINDEX_CUTOVER"
	RollbackConfirmation     = "I_UNDERSTAND_PODCASTINDEX_ROLLBACK"
	DefaultReserveFloorBytes = int64(20 * 1024 * 1024 * 1024)
)

// ColumnRequirement describes the SQLite type class required by the current
// PodcastIndex query and deduplication contract.
type ColumnRequirement struct {
	Name        string `json:"name"`
	TypeClass   string `json:"type_class"`
	Description string `json:"description,omitempty"`
}

// RequiredPodcastColumns is deliberately defined from the real query/view
// contract, not from the legacy PodcastIndex MySQL schema.
var RequiredPodcastColumns = []ColumnRequirement{
	{Name: "id", TypeClass: "integer", Description: "stable PodcastIndex row id"},
	{Name: "url", TypeClass: "text", Description: "RSS feed URL"},
	{Name: "title", TypeClass: "text", Description: "podcast title"},
	{Name: "lastUpdate", TypeClass: "integer"},
	{Name: "link", TypeClass: "text"},
	{Name: "lastHttpStatus", TypeClass: "integer"},
	{Name: "dead", TypeClass: "integer"},
	{Name: "itunesId", TypeClass: "integer"},
	{Name: "itunesAuthor", TypeClass: "text"},
	{Name: "explicit", TypeClass: "integer"},
	{Name: "imageUrl", TypeClass: "text"},
	{Name: "newestItemPubdate", TypeClass: "integer"},
	{Name: "language", TypeClass: "text"},
	{Name: "oldestItemPubdate", TypeClass: "integer"},
	{Name: "episodeCount", TypeClass: "integer"},
	{Name: "popularityScore", TypeClass: "integer"},
	{Name: "priority", TypeClass: "integer"},
	{Name: "updateFrequency", TypeClass: "integer"},
	{Name: "newestEnclosureUrl", TypeClass: "text"},
	{Name: "podcastGuid", TypeClass: "text"},
	{Name: "description", TypeClass: "text"},
	{Name: "newestEnclosureDuration", TypeClass: "integer"},
}

var RequiredViewColumns = []string{
	"id",
	"title",
	"itunesAuthor",
	"description",
	"imageUrl",
	"url",
	"itunesId",
	"language",
	"link",
	"newestEnclosureUrl",
	"newestEnclosureDuration",
	"lastUpdate",
	"newestItemPubdate",
	"oldestItemPubdate",
	"popularityScore",
	"priority",
	"updateFrequency",
	"episodeCount",
	"dead",
	"lastHttpStatus",
	"explicit",
}

type Fingerprint struct {
	URL                string            `json:"url"`
	StatusCode         int               `json:"status_code"`
	ContentLength      int64             `json:"content_length"`
	ContentType        string            `json:"content_type"`
	ETag               string            `json:"etag"`
	LastModified       string            `json:"last_modified"`
	AcceptRanges       string            `json:"accept_ranges,omitempty"`
	ContentDisposition string            `json:"content_disposition,omitempty"`
	CheckedAt          time.Time         `json:"checked_at"`
	Headers            map[string]string `json:"headers,omitempty"`
}

type DiskStats struct {
	Path           string `json:"path"`
	FilesystemID   string `json:"filesystem_id"`
	CapacityBytes  int64  `json:"capacity_bytes"`
	AvailableBytes int64  `json:"available_bytes"`
}

type DiskReport struct {
	Path               string `json:"path"`
	FilesystemID       string `json:"filesystem_id"`
	CapacityBytes      int64  `json:"capacity_bytes"`
	AvailableBytes     int64  `json:"available_bytes"`
	ArchiveBytes       int64  `json:"archive_bytes"`
	ExtractedBytes     int64  `json:"extracted_bytes"`
	SafetyReserveBytes int64  `json:"safety_reserve_bytes"`
	RequiredBytes      int64  `json:"required_bytes"`
	Passed             bool   `json:"passed"`
	Reason             string `json:"reason,omitempty"`
}

type ArchiveEntry struct {
	Name      string `json:"name"`
	SizeBytes int64  `json:"size_bytes"`
	Mode      int64  `json:"mode"`
	Type      string `json:"type"`
}

type ArchiveInspection struct {
	ArchivePath      string         `json:"archive_path"`
	GzipValid        bool           `json:"gzip_valid"`
	TarValid         bool           `json:"tar_valid"`
	DatabaseEntry    ArchiveEntry   `json:"database_entry"`
	Entries          []ArchiveEntry `json:"entries"`
	ExtractedBytes   int64          `json:"extracted_bytes"`
	ExpectedDatabase string         `json:"expected_database,omitempty"`
}

type SchemaColumn struct {
	CID          int    `json:"cid"`
	Name         string `json:"name"`
	DeclaredType string `json:"declared_type"`
	TypeClass    string `json:"type_class"`
	NotNull      bool   `json:"not_null"`
	PrimaryKey   bool   `json:"primary_key"`
}

type SchemaSummary struct {
	TableName      string         `json:"table_name"`
	Columns        []SchemaColumn `json:"columns"`
	ViewName       string         `json:"view_name"`
	ViewColumns    []string       `json:"view_columns"`
	UserVersion    int64          `json:"user_version"`
	PageCount      int64          `json:"page_count"`
	PageSize       int64          `json:"page_size"`
	RequiredFields bool           `json:"required_fields"`
}

type DatabaseMetrics struct {
	Path             string           `json:"path"`
	TotalRows        int64            `json:"total_rows"`
	LiveRows         int64            `json:"live_rows"`
	HTTP200Rows      int64            `json:"http_200_rows"`
	DeadRows         int64            `json:"dead_rows"`
	FreshestItemDate int64            `json:"freshest_item_date"`
	OldestItemDate   int64            `json:"oldest_item_date"`
	DeadDistribution map[string]int64 `json:"dead_distribution,omitempty"`
}

type MetricDelta struct {
	Baseline       int64   `json:"baseline"`
	Candidate      int64   `json:"candidate"`
	Delta          int64   `json:"delta"`
	ChangeFraction float64 `json:"change_fraction"`
}

type MetricsComparison struct {
	TotalRows             MetricDelta      `json:"total_rows"`
	LiveRows              MetricDelta      `json:"live_rows"`
	HTTP200Rows           MetricDelta      `json:"http_200_rows"`
	DeadRows              MetricDelta      `json:"dead_rows"`
	DeadDistributionDelta map[string]int64 `json:"dead_distribution_delta"`
	LiveRateBaseline      float64          `json:"live_rate_baseline"`
	LiveRateCandidate     float64          `json:"live_rate_candidate"`
	HTTP200RateBaseline   float64          `json:"http_200_rate_baseline"`
	HTTP200RateCandidate  float64          `json:"http_200_rate_candidate"`
	FreshestItemDateDelta int64            `json:"freshest_item_date_delta"`
	OldestItemDateDelta   int64            `json:"oldest_item_date_delta"`
	MaxChangeFraction     float64          `json:"max_change_fraction"`
	Passed                bool             `json:"passed"`
	Reasons               []string         `json:"reasons,omitempty"`
}

type QueryCompatibility struct {
	ViewCount             int64    `json:"view_count"`
	URLChecked            bool     `json:"url_checked"`
	URL                   string   `json:"url,omitempty"`
	TitleChecked          bool     `json:"title_checked"`
	Title                 string   `json:"title,omitempty"`
	ITunesIDChecked       bool     `json:"itunes_id_checked"`
	ITunesID              int64    `json:"itunes_id,omitempty"`
	IdentityLookupChecked bool     `json:"identity_lookup_checked"`
	IdentityLookupIndexed bool     `json:"identity_lookup_indexed"`
	IdentityLookupPlan    []string `json:"identity_lookup_plan,omitempty"`
	Passed                bool     `json:"passed"`
	Error                 string   `json:"error,omitempty"`
}

type ValidationResult struct {
	Path           string             `json:"path"`
	SHA256         string             `json:"sha256"`
	SizeBytes      int64              `json:"size_bytes"`
	IntegrityCheck string             `json:"integrity_check"`
	Schema         SchemaSummary      `json:"schema"`
	Metrics        DatabaseMetrics    `json:"metrics"`
	Query          QueryCompatibility `json:"query_compatibility"`
	ViewCreated    bool               `json:"view_created"`
	Passed         bool               `json:"passed"`
	Error          string             `json:"error,omitempty"`
}

type FailedSample struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	Author      string `json:"author,omitempty"`
	FeedURL     string `json:"feed_url"`
	ITunesID    string `json:"itunes_id,omitempty"`
	PodcastGUID string `json:"podcast_guid,omitempty"`
}

type CandidateMatch struct {
	SampleID          int64          `json:"sample_id"`
	SampleTitle       string         `json:"sample_title"`
	CurrentFeedURL    string         `json:"current_feed_url"`
	CandidateID       int64          `json:"candidate_id,omitempty"`
	CandidateTitle    string         `json:"candidate_title,omitempty"`
	CandidateAuthor   string         `json:"candidate_author,omitempty"`
	CandidateURL      string         `json:"candidate_url,omitempty"`
	CandidateITunesID string         `json:"candidate_itunes_id,omitempty"`
	CandidateGUID     string         `json:"candidate_guid,omitempty"`
	IdentityMethod    string         `json:"identity_method,omitempty"`
	Confidence        string         `json:"confidence,omitempty"`
	IdentityConfirmed bool           `json:"identity_confirmed"`
	TitleOnly         bool           `json:"title_only"`
	Accessible        *Accessibility `json:"accessibility,omitempty"`
}

type Accessibility struct {
	URL        string    `json:"url"`
	FinalURL   string    `json:"final_url,omitempty"`
	StatusCode int       `json:"status_code,omitempty"`
	OK         bool      `json:"ok"`
	Error      string    `json:"error,omitempty"`
	CheckedAt  time.Time `json:"checked_at"`
}

type SampleComparison struct {
	ExpectedSamples             int              `json:"expected_samples"`
	ActualSamples               int              `json:"actual_samples"`
	Matched                     int              `json:"matched"`
	NoMatch                     int              `json:"no_match"`
	IdentityConfirmed           int              `json:"identity_confirmed"`
	TitleOnly                   int              `json:"title_only"`
	AccessibleIdentityConfirmed int              `json:"accessible_identity_confirmed"`
	AccessibleAny               int              `json:"accessible_any"`
	AccessibilityChecked        bool             `json:"accessibility_checked"`
	Matches                     []CandidateMatch `json:"matches,omitempty"`
	Error                       string           `json:"error,omitempty"`
}

type CutoverRecord struct {
	Status             string    `json:"status"`
	ServiceStopped     bool      `json:"service_stopped"`
	DryRun             bool      `json:"dry_run"`
	CandidatePath      string    `json:"candidate_path,omitempty"`
	LivePath           string    `json:"live_path,omitempty"`
	BackupPath         string    `json:"backup_path,omitempty"`
	BackupSHA256       string    `json:"backup_sha256,omitempty"`
	FailedPath         string    `json:"failed_path,omitempty"`
	RollbackTested     bool      `json:"rollback_tested"`
	ProductionVerified bool      `json:"production_verified"`
	StartedAt          time.Time `json:"started_at,omitempty"`
	CompletedAt        time.Time `json:"completed_at,omitempty"`
	Error              string    `json:"error,omitempty"`
}

type Decision struct {
	Go        bool      `json:"go"`
	Reasons   []string  `json:"reasons,omitempty"`
	CheckedAt time.Time `json:"checked_at"`
}

type Manifest struct {
	SchemaVersion int                `json:"schema_version"`
	RunID         string             `json:"run_id"`
	Scope         string             `json:"scope"`
	CreatedAt     time.Time          `json:"created_at"`
	UpdatedAt     time.Time          `json:"updated_at"`
	Source        SourceManifest     `json:"source"`
	Archive       ArchiveInspection  `json:"archive"`
	Disk          DiskReport         `json:"disk"`
	Candidate     ValidationResult   `json:"candidate"`
	Baseline      *DatabaseMetrics   `json:"baseline,omitempty"`
	Quality       *MetricsComparison `json:"quality_comparison,omitempty"`
	Comparison    *SampleComparison  `json:"comparison,omitempty"`
	Cutover       CutoverRecord      `json:"cutover"`
	Decision      Decision           `json:"decision"`
	Blockers      []string           `json:"blockers,omitempty"`
}

type SourceManifest struct {
	URL                         string       `json:"url"`
	Transport                   string       `json:"transport,omitempty"`
	ProxyEndpoint               string       `json:"proxy_endpoint,omitempty"`
	LicensingNote               string       `json:"licensing_note"`
	ThirdPartyContentConstraint string       `json:"third_party_content_constraint"`
	DownloadedAt                *time.Time   `json:"downloaded_at,omitempty"`
	Before                      *Fingerprint `json:"before,omitempty"`
	After                       *Fingerprint `json:"after,omitempty"`
	SHA256                      string       `json:"sha256,omitempty"`
	SizeBytes                   int64        `json:"size_bytes,omitempty"`
}

type DownloadResult struct {
	StagingDir  string             `json:"staging_dir"`
	ArchivePath string             `json:"archive_path,omitempty"`
	PartialPath string             `json:"partial_path,omitempty"`
	Before      Fingerprint        `json:"before"`
	After       *Fingerprint       `json:"after,omitempty"`
	SHA256      string             `json:"sha256,omitempty"`
	SizeBytes   int64              `json:"size_bytes,omitempty"`
	Archive     *ArchiveInspection `json:"archive,omitempty"`
	Disk        DiskReport         `json:"disk"`
	Error       string             `json:"error,omitempty"`
}
