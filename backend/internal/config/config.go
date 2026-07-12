package config

import (
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/spf13/viper"
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

	// 验证配置
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
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
