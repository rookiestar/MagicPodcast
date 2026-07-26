package config

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
	"magicpodcast/internal/feed"
)

// Config 表示应用程序的配置
type Config struct {
	Server       ServerConfig       `mapstructure:"server"`
	Database     DatabaseConfig     `mapstructure:"database"`
	XYZAPI       XYZAPIConfig       `mapstructure:"xyz_api"`
	Sync         SyncConfig         `mapstructure:"sync"`
	PodcastIndex PodcastIndexConfig `mapstructure:"podcast_index"`
	Logging      LoggingConfig      `mapstructure:"logging"`
	Search       SearchConfig       `mapstructure:"search"`
	User         UserConfig         `mapstructure:"user"`
	Email        EmailConfig        `mapstructure:"email"`
	LLM          LLMConfig          `mapstructure:"llm"`
	// Feed carries the startup-loaded Feed fetcher / coordinator configuration.
	// It is applied once at process start (no hot reload) via
	// feed.ConfigureSharedRuntime, so changes require a restart.
	Feed feed.FeedConfig `mapstructure:"feed"`
}

// ServerConfig 服务器配置
type ServerConfig struct {
	Host         string        `mapstructure:"host"`
	Port         int           `mapstructure:"port"`
	Mode         string        `mapstructure:"mode"` // debug, release
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
	CORS         CORSConfig    `mapstructure:"cors"`
}

// CORSConfig CORS配置
type CORSConfig struct {
	Enabled          bool     `mapstructure:"enabled"`
	AllowOrigins     []string `mapstructure:"allow_origins"`
	AllowMethods     []string `mapstructure:"allow_methods"`
	AllowHeaders     []string `mapstructure:"allow_headers"`
	ExposeHeaders    []string `mapstructure:"expose_headers"`
	AllowCredentials bool     `mapstructure:"allow_credentials"`
	MaxAge           int      `mapstructure:"max_age"` // 秒
}

// DatabaseConfig 数据库配置
type DatabaseConfig struct {
	Path            string        `mapstructure:"path"`
	Debug           bool          `mapstructure:"debug"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	BusyTimeoutMS   int           `mapstructure:"busy_timeout_ms"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
}

// XYZAPIConfig 小宇宙 API 配置
type XYZAPIConfig struct {
	URL           string `mapstructure:"url"`
	Timeout       int    `mapstructure:"timeout"` // 秒
	MaxRetries    int    `mapstructure:"max_retries"`
	RetryInterval int    `mapstructure:"retry_interval"` // 毫秒
}

// SyncConfig 同步配置
type SyncConfig struct {
	Enabled         bool   `mapstructure:"enabled"`
	Schedule        string `mapstructure:"schedule"`         // Cron 表达式
	Concurrency     int    `mapstructure:"concurrency"`      // 并发数
	RequestInterval int    `mapstructure:"request_interval"` // 毫秒
	MaxPodcasts     int    `mapstructure:"max_podcasts"`     // 0 表示不限制
}

// PodcastIndexConfig PodcastIndex数据库配置
type PodcastIndexConfig struct {
	Path string `mapstructure:"path"` // PodcastIndex SQLite数据库文件路径
}

// LoggingConfig 日志配置
type LoggingConfig struct {
	Level      string `mapstructure:"level"`  // debug, info, warn, error
	Format     string `mapstructure:"format"` // json, text
	Output     string `mapstructure:"output"` // 空表示标准输出
	Rotate     bool   `mapstructure:"rotate"`
	MaxSize    int    `mapstructure:"max_size"`    // MB
	MaxAge     int    `mapstructure:"max_age"`     // 天
	MaxBackups int    `mapstructure:"max_backups"` // 文件数
}

// UserConfig 用户配置
type UserConfig struct {
	Phone        string `mapstructure:"phone"`
	AccessToken  string `mapstructure:"access_token"`
	RefreshToken string `mapstructure:"refresh_token"`
}

// EmailConfig 邮件通知配置
type EmailConfig struct {
	Enabled  bool   `mapstructure:"enabled"`   // 是否启用邮件通知
	SMTPHost string `mapstructure:"smtp_host"` // SMTP服务器地址
	SMTPPort int    `mapstructure:"smtp_port"` // SMTP端口（通常465或587）
	Username string `mapstructure:"username"`  // 发件邮箱账号
	Password string `mapstructure:"password"`  // 邮箱授权码/密码
	From     string `mapstructure:"from"`      // 发件人显示名称
	To       string `mapstructure:"to"`        // 收件邮箱地址
	UseTLS   bool   `mapstructure:"use_tls"`   // 是否使用TLS
}

// LLMProvider LLM提供商类型
type LLMProvider string

const (
	LLMProviderSiliconFlow LLMProvider = "siliconflow"
	LLMProviderOpenAI      LLMProvider = "openai"
	LLMProviderAnthropic   LLMProvider = "anthropic"
	LLMProviderZhipuAI     LLMProvider = "zhipuai"
	LLMProviderDeepSeek    LLMProvider = "deepseek"
)

// LLMConfig LLM配置
type LLMConfig struct {
	Enabled             bool        `mapstructure:"enabled"`
	Provider            LLMProvider `mapstructure:"provider"`
	APIKey              string      `mapstructure:"api_key"`
	BaseURL             string      `mapstructure:"base_url"`
	DefaultModel        string      `mapstructure:"default_model"`
	Timeout             int         `mapstructure:"timeout"`
	MaxRetries          int         `mapstructure:"max_retries"`
	RetryInterval       int         `mapstructure:"retry_interval"`
	MaxConcurrent       int         `mapstructure:"max_concurrent"`
	RateLimitPerMinute  int         `mapstructure:"rate_limit_per_minute"`
	MaxTokensPerRequest int         `mapstructure:"max_tokens_per_request"`
	PromptsDir          string      `mapstructure:"prompts_dir"`   // Prompt模板目录
	SystemPrompt        string      `mapstructure:"system_prompt"` // 全局System Prompt
}

// SearchWeights 搜索字段权重
type SearchWeights struct {
	PodcastTitle   float64 `mapstructure:"podcast_title"`
	EpisodeTitle   float64 `mapstructure:"episode_title"`
	Author         float64 `mapstructure:"author"`
	PodcastDesc    float64 `mapstructure:"podcast_desc"`
	EpisodeContent float64 `mapstructure:"episode_content"`
}

// SearchMatchMultipliers 搜索匹配类型乘数
type SearchMatchMultipliers struct {
	Exact      float64 `mapstructure:"exact"`
	Prefix     float64 `mapstructure:"prefix"`
	Contains   float64 `mapstructure:"contains"`
	Occurrence float64 `mapstructure:"occurrence"`
}

// SearchConfig 搜索配置
type SearchConfig struct {
	Weights          SearchWeights          `mapstructure:"weights"`
	MatchMultipliers SearchMatchMultipliers `mapstructure:"match_multipliers"`
	DefaultPageSize  int                    `mapstructure:"default_page_size"`
	MaxPageSize      int                    `mapstructure:"max_page_size"`
}

var cfg *Config

const defaultServerHost = "127.0.0.1"

// Load 加载配置文件
func Load(configPath string) (*Config, error) {
	viper.SetConfigFile(configPath)
	viper.SetConfigType("yaml")

	// 环境变量前缀
	viper.SetEnvPrefix("MAGICPODCAST")
	viper.AutomaticEnv()
	// 让嵌套键（如 feed.user_agent）能匹配带下划线的环境变量
	// （MAGICPODCAST_FEED_USER_AGENT）。绑定到具体叶子键后生效。
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	bindFeedEnvKeys()

	// 读取配置文件
	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// 解析配置
	cfg = &Config{}
	if err := viper.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// 从环境变量覆盖敏感配置
	cfg.applyEnvOverrides()
	if err := applyFeedJSONEnvOverrides(cfg); err != nil {
		return nil, fmt.Errorf("feed environment override failed: %w", err)
	}

	// 验证配置
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}
	if err := cfg.Feed.Validate(); err != nil {
		return nil, fmt.Errorf("feed config validation failed: %w", err)
	}

	return cfg, nil
}

// bindFeedEnvKeys binds the feed configuration leaf keys to their
// MAGICPODCAST_FEED_* environment variables so operators can override any
// startup-loaded feed knob without editing the YAML. BindEnv must run before
// Unmarshal so the bindings participate in decoding.
func bindFeedEnvKeys() {
	for _, key := range feedEnvKeys {
		_ = viper.BindEnv(key)
	}
}

// feedEnvKeys contains scalar Feed keys overridable through Viper. Collection
// values use the explicit JSON decoder below because Viper treats a JSON ENV
// value as a scalar string during nested unmarshal.
var feedEnvKeys = []string{
	"feed.user_agent",
	"feed.timeouts.connect",
	"feed.timeouts.tls",
	"feed.timeouts.header",
	"feed.timeouts.overall",
	"feed.headers.accept",
	"feed.headers.accept_language",
	"feed.retry.budget",
	"feed.retry.jitter",
	"feed.circuit.domain_evidence_min_distinct_feeds",
	"feed.circuit.half_open_max",
	"feed.circuit.successes_to_close",
	"feed.user_agent_recovery.initial_cooldown",
	"feed.user_agent_recovery.probe_failure_cooldown",
	"feed.user_agent_recovery.required_successes",
	"feed.snapshot.durable",
	"feed.snapshot.bounds.max_entries",
	"feed.snapshot.bounds.max_response_bytes",
	"feed.snapshot.bounds.max_total_bytes",
	"feed.diagnostics.admin_enabled",
	"feed.diagnostics.configured_egress_label",
}

// applyFeedJSONEnvOverrides handles the two Feed values that are collections
// rather than scalar leaves. Viper's scalar bindings do not reliably decode a
// JSON object/array into nested mapstructure fields, so these overrides are
// parsed explicitly. Durations in domain policies accept Go duration strings
// (for example "30m") or integer nanoseconds for process-level tooling.
func applyFeedJSONEnvOverrides(cfg *Config) error {
	if raw, ok := os.LookupEnv("MAGICPODCAST_FEED_CIRCUIT_THRESHOLDS_PER_CATEGORY"); ok {
		var thresholds map[string]int
		if err := json.Unmarshal([]byte(raw), &thresholds); err != nil {
			return fmt.Errorf("MAGICPODCAST_FEED_CIRCUIT_THRESHOLDS_PER_CATEGORY: %w", err)
		}
		cfg.Feed.Circuit.ThresholdsPerCategory = thresholds
	}

	if raw, ok := os.LookupEnv("MAGICPODCAST_FEED_DOMAIN_POLICIES"); ok {
		var policies []feedDomainPolicyJSON
		if err := json.Unmarshal([]byte(raw), &policies); err != nil {
			return fmt.Errorf("MAGICPODCAST_FEED_DOMAIN_POLICIES: %w", err)
		}
		decoded := make([]feed.FeedDomainPolicy, 0, len(policies))
		for i, policy := range policies {
			decodedPolicy, err := policy.decode()
			if err != nil {
				return fmt.Errorf("MAGICPODCAST_FEED_DOMAIN_POLICIES[%d]: %w", i, err)
			}
			decoded = append(decoded, decodedPolicy)
		}
		cfg.Feed.DomainPolicies = decoded
	}
	return nil
}

type feedDomainPolicyJSON struct {
	Domain                         string          `json:"domain"`
	MaxConcurrency                 int             `json:"max_concurrency"`
	MinRefreshInterval             json.RawMessage `json:"min_refresh_interval"`
	MaxJitter                      json.RawMessage `json:"max_jitter"`
	CircuitCooldown                json.RawMessage `json:"circuit_cooldown"`
	RetryBackoffInitial            json.RawMessage `json:"retry_backoff_initial"`
	RetryBackoffMax                json.RawMessage `json:"retry_backoff_max"`
	HalfOpenMaxRequests            int             `json:"half_open_max_requests"`
	SuccessesToClose               int             `json:"successes_to_close"`
	DomainEvidenceMinDistinctFeeds int             `json:"domain_evidence_min_distinct_feeds"`
	EvidenceWindow                 json.RawMessage `json:"evidence_window"`
}

func (p feedDomainPolicyJSON) decode() (feed.FeedDomainPolicy, error) {
	minRefreshInterval, err := decodeFeedDuration(p.MinRefreshInterval, "min_refresh_interval")
	if err != nil {
		return feed.FeedDomainPolicy{}, err
	}
	maxJitter, err := decodeFeedDuration(p.MaxJitter, "max_jitter")
	if err != nil {
		return feed.FeedDomainPolicy{}, err
	}
	circuitCooldown, err := decodeFeedDuration(p.CircuitCooldown, "circuit_cooldown")
	if err != nil {
		return feed.FeedDomainPolicy{}, err
	}
	retryBackoffInitial, err := decodeFeedDuration(p.RetryBackoffInitial, "retry_backoff_initial")
	if err != nil {
		return feed.FeedDomainPolicy{}, err
	}
	retryBackoffMax, err := decodeFeedDuration(p.RetryBackoffMax, "retry_backoff_max")
	if err != nil {
		return feed.FeedDomainPolicy{}, err
	}
	evidenceWindow, err := decodeFeedDuration(p.EvidenceWindow, "evidence_window")
	if err != nil {
		return feed.FeedDomainPolicy{}, err
	}
	return feed.FeedDomainPolicy{
		Domain:                         p.Domain,
		MaxConcurrency:                 p.MaxConcurrency,
		MinRefreshInterval:             minRefreshInterval,
		MaxJitter:                      maxJitter,
		CircuitCooldown:                circuitCooldown,
		RetryBackoffInitial:            retryBackoffInitial,
		RetryBackoffMax:                retryBackoffMax,
		HalfOpenMaxRequests:            p.HalfOpenMaxRequests,
		SuccessesToClose:               p.SuccessesToClose,
		DomainEvidenceMinDistinctFeeds: p.DomainEvidenceMinDistinctFeeds,
		EvidenceWindow:                 evidenceWindow,
	}, nil
}

func decodeFeedDuration(raw json.RawMessage, field string) (time.Duration, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		duration, parseErr := time.ParseDuration(text)
		if parseErr != nil {
			return 0, fmt.Errorf("%s must be a Go duration string: %w", field, parseErr)
		}
		return duration, nil
	}
	var nanos int64
	if err := json.Unmarshal(raw, &nanos); err != nil {
		return 0, fmt.Errorf("%s must be a duration string or integer nanoseconds: %w", field, err)
	}
	return time.Duration(nanos), nil
}

// applyEnvOverrides 从环境变量覆盖本机运行配置
func (c *Config) applyEnvOverrides() {
	if strings.TrimSpace(c.Server.Host) == "" {
		c.Server.Host = defaultServerHost
	}
	if host := strings.TrimSpace(viper.GetString("server_host")); host != "" {
		c.Server.Host = host
	}
	if mode := viper.GetString("server_mode"); mode != "" {
		c.Server.Mode = mode
	}
	if port := viper.GetInt("server_port"); port != 0 {
		c.Server.Port = port
	}
	if viper.IsSet("database_debug") {
		c.Database.Debug = viper.GetBool("database_debug")
	}
	if path := viper.GetString("database_path"); path != "" {
		c.Database.Path = path
	}
	if timeout := viper.GetInt("database_busy_timeout_ms"); timeout != 0 {
		c.Database.BusyTimeoutMS = timeout
	}

	// LLM API Key
	if key := viper.GetString("llm_api_key"); key != "" {
		c.LLM.APIKey = key
	}
	if provider := viper.GetString("llm_provider"); provider != "" {
		c.LLM.Provider = LLMProvider(provider)
	}
	if baseURL := viper.GetString("llm_base_url"); baseURL != "" {
		c.LLM.BaseURL = baseURL
	}
	if model := viper.GetString("llm_default_model"); model != "" {
		c.LLM.DefaultModel = model
	}

	// Email SMTP配置
	if host := viper.GetString("smtp_host"); host != "" {
		c.Email.SMTPHost = host
	}
	if port := viper.GetInt("smtp_port"); port != 0 {
		c.Email.SMTPPort = port
	}
	if username := viper.GetString("smtp_username"); username != "" {
		c.Email.Username = username
	}
	if password := viper.GetString("smtp_password"); password != "" {
		c.Email.Password = password
	}

	// User凭证
	if phone := viper.GetString("user_phone"); phone != "" {
		c.User.Phone = phone
	}
	if accessToken := viper.GetString("user_access_token"); accessToken != "" {
		c.User.AccessToken = accessToken
	}
	if refreshToken := viper.GetString("user_refresh_token"); refreshToken != "" {
		c.User.RefreshToken = refreshToken
	}
}

// Get 获取配置实例
func Get() *Config {
	return cfg
}

// Validate 验证配置
func (c *Config) Validate() error {
	// 生产和开发服务都只允许绑定到数值 loopback 地址，避免局域网绕过 Cloudflare Access。
	host := strings.TrimSpace(c.Server.Host)
	parsedHost := net.ParseIP(host)
	if parsedHost == nil || !parsedHost.IsLoopback() {
		return fmt.Errorf("server host must be a loopback IP address: %q", c.Server.Host)
	}
	c.Server.Host = host

	// 验证服务器端口
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", c.Server.Port)
	}

	// 验证服务器模式
	if c.Server.Mode != "debug" && c.Server.Mode != "release" {
		return fmt.Errorf("invalid server mode: %s (must be 'debug' or 'release')", c.Server.Mode)
	}

	// 验证数据库路径
	if c.Database.Path == "" {
		return fmt.Errorf("database path cannot be empty")
	}

	// 验证 XYZ API URL
	if c.XYZAPI.URL == "" {
		return fmt.Errorf("xyz_api url cannot be empty")
	}

	return nil
}

// IsDevelopment 是否为开发环境
func (c *Config) IsDevelopment() bool {
	return c.Server.Mode == "debug"
}

// IsProduction 是否为生产环境
func (c *Config) IsProduction() bool {
	return c.Server.Mode == "release"
}
