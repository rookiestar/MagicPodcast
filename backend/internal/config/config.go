package config

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

// Config 表示应用程序的配置
type Config struct {
	Server        ServerConfig        `mapstructure:"server"`
	Database      DatabaseConfig      `mapstructure:"database"`
	XYZAPI        XYZAPIConfig        `mapstructure:"xyz_api"`
	Sync          SyncConfig          `mapstructure:"sync"`
	PodcastIndex  PodcastIndexConfig  `mapstructure:"podcast_index"`
	Logging       LoggingConfig       `mapstructure:"logging"`
	User          UserConfig          `mapstructure:"user"`
}

// ServerConfig 服务器配置
type ServerConfig struct {
	Port         int           `mapstructure:"port"`
	Mode         string        `mapstructure:"mode"` // debug, release
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
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
	Enabled          bool   `mapstructure:"enabled"`
	Schedule         string `mapstructure:"schedule"`          // Cron 表达式
	Concurrency      int    `mapstructure:"concurrency"`       // 并发数
	RequestInterval  int    `mapstructure:"request_interval"`  // 毫秒
	MaxPodcasts      int    `mapstructure:"max_podcasts"`      // 0 表示不限制
}

// PodcastIndexConfig PodcastIndex数据库配置
type PodcastIndexConfig struct {
	Path string `mapstructure:"path"` // PodcastIndex SQLite数据库文件路径
}

// LoggingConfig 日志配置
type LoggingConfig struct {
	Level      string `mapstructure:"level"`       // debug, info, warn, error
	Format     string `mapstructure:"format"`      // json, text
	Output     string `mapstructure:"output"`      // 空表示标准输出
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

var cfg *Config

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

	// 验证配置
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}

// Get 获取配置实例
func Get() *Config {
	return cfg
}

// Validate 验证配置
func (c *Config) Validate() error {
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
