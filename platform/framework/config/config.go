package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server    ServerConfig   `yaml:"server"`
	Database  DatabaseConfig `yaml:"database"`
	Logging   LoggingConfig  `yaml:"logging"`
	Auth      AuthConfig     `yaml:"auth"`
	MCP       MCPConfig      `yaml:"mcp"`
	I18n      I18nConfig     `yaml:"i18n"`
	Storage   StorageConfig  `yaml:"storage"`
	Outbox    OutboxConfig   `yaml:"outbox"`
	Redis     RedisConfig    `yaml:"redis"`
	Jobs      JobsConfig     `yaml:"jobs"`
	AI        AIConfig       `yaml:"ai"`
	StateFile string         `yaml:"state_file"` // 安装状态文件路径，默认 configs/state.yaml
}

type MCPConfig struct {
	BaseURL string `yaml:"base_url"`
}

type AIConfig struct {
	CredentialEncryptionKey string `yaml:"credential_encryption_key"`
}

// RedisConfig is deployment secret/config for Asynq (not tenant settings).
type RedisConfig struct {
	Addr         string `yaml:"addr"`
	Username     string `yaml:"username"`
	Password     string `yaml:"password"`
	DB           int    `yaml:"db"`
	PoolSize     int    `yaml:"pool_size"`
	DialTimeout  string `yaml:"dial_timeout"`
	ReadTimeout  string `yaml:"read_timeout"`
	WriteTimeout string `yaml:"write_timeout"`
}

func (c RedisConfig) DialTimeoutDuration() time.Duration {
	return parseDurationOr(c.DialTimeout, 5*time.Second)
}
func (c RedisConfig) ReadTimeoutDuration() time.Duration {
	return parseDurationOr(c.ReadTimeout, 3*time.Second)
}
func (c RedisConfig) WriteTimeoutDuration() time.Duration {
	return parseDurationOr(c.WriteTimeout, 3*time.Second)
}

// JobsConfig controls background job workers / enqueue (Issue #21).
type JobsConfig struct {
	// Enabled defaults to false when omitted (HTTP can run without Redis).
	Enabled          *bool          `yaml:"enabled"`
	Concurrency      int            `yaml:"concurrency"`
	Queues           map[string]int `yaml:"queues"`
	DefaultTimeout   string         `yaml:"default_timeout"`
	MaxPayloadBytes  int            `yaml:"max_payload_bytes"`
	ShutdownTimeout  string         `yaml:"shutdown_timeout"`
	RecoveryInterval string         `yaml:"recovery_interval"`
	EmbeddedWorker   bool           `yaml:"embedded_worker"` // dev-only: run worker inside HTTP process
}

func (c JobsConfig) IsEnabled() bool {
	if c.Enabled == nil {
		return false
	}
	return *c.Enabled
}

func (c JobsConfig) DefaultTimeoutDuration() time.Duration {
	return parseDurationOr(c.DefaultTimeout, 5*time.Minute)
}
func (c JobsConfig) ShutdownTimeoutDuration() time.Duration {
	return parseDurationOr(c.ShutdownTimeout, 30*time.Second)
}
func (c JobsConfig) RecoveryIntervalDuration() time.Duration {
	return parseDurationOr(c.RecoveryInterval, 30*time.Second)
}

// OutboxConfig controls Transactional Outbox dispatcher (Issue #20).
type OutboxConfig struct {
	// Enabled defaults to true when omitted; set false to disable the dispatcher.
	Enabled         *bool  `yaml:"enabled"`
	Transport       string `yaml:"transport"` // postgres（默认）；redis 后续
	BatchSize       int    `yaml:"batch_size"`
	PollInterval    string `yaml:"poll_interval"`
	LockTimeout     string `yaml:"lock_timeout"`
	MaxAttempts     int    `yaml:"max_attempts"`
	BackoffBase     string `yaml:"backoff_base"`
	BackoffMax      string `yaml:"backoff_max"`
	ShutdownTimeout string `yaml:"shutdown_timeout"`
}

func (c OutboxConfig) IsEnabled() bool {
	if c.Enabled == nil {
		return true
	}
	return *c.Enabled
}

func (c OutboxConfig) PollIntervalDuration() time.Duration {
	return parseDurationOr(c.PollInterval, time.Second)
}
func (c OutboxConfig) LockTimeoutDuration() time.Duration {
	return parseDurationOr(c.LockTimeout, 30*time.Second)
}
func (c OutboxConfig) BackoffBaseDuration() time.Duration {
	return parseDurationOr(c.BackoffBase, time.Second)
}
func (c OutboxConfig) BackoffMaxDuration() time.Duration {
	return parseDurationOr(c.BackoffMax, 5*time.Minute)
}
func (c OutboxConfig) ShutdownTimeoutDuration() time.Duration {
	return parseDurationOr(c.ShutdownTimeout, 15*time.Second)
}

func parseDurationOr(raw string, fallback time.Duration) time.Duration {
	if raw == "" {
		return fallback
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return fallback
	}
	return d
}

type ServerConfig struct {
	Addr string `yaml:"addr"`
	Mode string `yaml:"mode"`
}

type DatabaseConfig struct {
	DSN string `yaml:"dsn"`
}

type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

type AuthConfig struct {
	// DevTokenEnabled 开发桩；生产必须为 false。正式 JWT 优先。
	DevTokenEnabled bool      `yaml:"dev_token_enabled"`
	JWT             JWTConfig `yaml:"jwt"`
	RefreshTTL      string    `yaml:"refresh_ttl"`
}

type JWTConfig struct {
	Secret string `yaml:"secret"`
	Issuer string `yaml:"issuer"`
	TTL    string `yaml:"ttl"` // e.g. 15m
}

type I18nConfig struct {
	DefaultLanguage string `yaml:"default_language"` // 缺省语言；空则使用 zh-CN
	Dir             string `yaml:"dir"`              // 翻译文件目录，默认 "i18n"
}

type StorageConfig struct {
	Provider string             `yaml:"provider"` // local（默认）；s3/oss 后续扩展
	Local    StorageLocalConfig `yaml:"local"`
}

type StorageLocalConfig struct {
	RootDir string `yaml:"root_dir"`
}

func (j JWTConfig) Duration() time.Duration {
	if j.TTL == "" {
		return 15 * time.Minute
	}
	d, err := time.ParseDuration(j.TTL)
	if err != nil || d <= 0 {
		return 15 * time.Minute
	}
	return d
}

func (a AuthConfig) RefreshDuration() time.Duration {
	d, err := time.ParseDuration(a.RefreshTTL)
	if err != nil || d <= 0 {
		return 30 * 24 * time.Hour
	}
	return d
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	cfg.applyDefaults()
	cfg.applyEnvOverrides()
	return &cfg, nil
}

func (c *Config) applyDefaults() {
	if c.Server.Addr == "" {
		c.Server.Addr = ":8080"
	}
	if c.Server.Mode == "" {
		c.Server.Mode = "debug"
	}
	if c.Logging.Level == "" {
		c.Logging.Level = "info"
	}
	if c.Logging.Format == "" {
		c.Logging.Format = "text"
	}
	if c.Auth.JWT.Issuer == "" {
		c.Auth.JWT.Issuer = "ainp"
	}
	if c.Auth.JWT.TTL == "" {
		c.Auth.JWT.TTL = "15m"
	}
	if c.Auth.RefreshTTL == "" {
		c.Auth.RefreshTTL = "720h"
	}
	if c.MCP.BaseURL == "" {
		c.MCP.BaseURL = "http://127.0.0.1" + c.Server.Addr
	}
	if c.StateFile == "" {
		c.StateFile = "configs/state.yaml"
	}
	if c.Storage.Provider == "" {
		c.Storage.Provider = "local"
	}
	if c.Storage.Local.RootDir == "" {
		c.Storage.Local.RootDir = "data/objects"
	}
	if c.Outbox.Transport == "" {
		c.Outbox.Transport = "postgres"
	}
	if c.Outbox.BatchSize <= 0 {
		c.Outbox.BatchSize = 32
	}
	if c.Outbox.PollInterval == "" {
		c.Outbox.PollInterval = "1s"
	}
	if c.Outbox.LockTimeout == "" {
		c.Outbox.LockTimeout = "30s"
	}
	if c.Outbox.MaxAttempts <= 0 {
		c.Outbox.MaxAttempts = 10
	}
	if c.Outbox.BackoffBase == "" {
		c.Outbox.BackoffBase = "1s"
	}
	if c.Outbox.BackoffMax == "" {
		c.Outbox.BackoffMax = "5m"
	}
	if c.Outbox.ShutdownTimeout == "" {
		c.Outbox.ShutdownTimeout = "15s"
	}
	if c.Redis.Addr == "" {
		c.Redis.Addr = "127.0.0.1:6379"
	}
	if c.Redis.PoolSize <= 0 {
		c.Redis.PoolSize = 10
	}
	if c.Redis.DialTimeout == "" {
		c.Redis.DialTimeout = "5s"
	}
	if c.Redis.ReadTimeout == "" {
		c.Redis.ReadTimeout = "3s"
	}
	if c.Redis.WriteTimeout == "" {
		c.Redis.WriteTimeout = "3s"
	}
	if c.Jobs.Concurrency <= 0 {
		c.Jobs.Concurrency = 10
	}
	if len(c.Jobs.Queues) == 0 {
		c.Jobs.Queues = map[string]int{
			"critical": 6, "default": 3, "file": 2, "notification": 2, "ai": 1,
		}
	}
	if c.Jobs.DefaultTimeout == "" {
		c.Jobs.DefaultTimeout = "5m"
	}
	if c.Jobs.MaxPayloadBytes <= 0 {
		c.Jobs.MaxPayloadBytes = 262144
	}
	if c.Jobs.ShutdownTimeout == "" {
		c.Jobs.ShutdownTimeout = "30s"
	}
	if c.Jobs.RecoveryInterval == "" {
		c.Jobs.RecoveryInterval = "30s"
	}
}

// applyEnvOverrides 环境变量优先级高于配置文件。
func (c *Config) applyEnvOverrides() {
	if dsn := os.Getenv("DATABASE_DSN"); dsn != "" {
		c.Database.DSN = dsn
	}
	if secret := os.Getenv("JWT_SECRET"); secret != "" {
		c.Auth.JWT.Secret = secret
	}
	if root := os.Getenv("STORAGE_LOCAL_ROOT"); root != "" {
		c.Storage.Local.RootDir = root
	}
	if baseURL := os.Getenv("MCP_BASE_URL"); baseURL != "" {
		c.MCP.BaseURL = baseURL
	}
	if provider := os.Getenv("STORAGE_PROVIDER"); provider != "" {
		c.Storage.Provider = provider
	}
	if addr := os.Getenv("REDIS_ADDR"); addr != "" {
		c.Redis.Addr = addr
	}
	if pass := os.Getenv("REDIS_PASSWORD"); pass != "" {
		c.Redis.Password = pass
	}
	if user := os.Getenv("REDIS_USERNAME"); user != "" {
		c.Redis.Username = user
	}
	if key := os.Getenv("AI_CREDENTIAL_ENCRYPTION_KEY"); key != "" {
		c.AI.CredentialEncryptionKey = key
	}
}

// Validate 校验必填项；setup 模式（DSN 可为空）由调用方在 Load 后自行判断。
func (c *Config) Validate(requireDSN bool) error {
	if requireDSN && c.Database.DSN == "" {
		return fmt.Errorf("database.dsn is required")
	}
	if c.Auth.JWT.Secret == "" {
		return fmt.Errorf("auth.jwt.secret is required")
	}
	if c.AI.CredentialEncryptionKey == "" {
		return fmt.Errorf("ai.credential_encryption_key is required")
	}
	if duration, err := time.ParseDuration(c.Auth.JWT.TTL); err != nil || duration <= 0 {
		return fmt.Errorf("auth.jwt.ttl is invalid")
	}
	if duration, err := time.ParseDuration(c.Auth.RefreshTTL); err != nil || duration <= 0 {
		return fmt.Errorf("auth.refresh_ttl is invalid")
	}
	return nil
}
