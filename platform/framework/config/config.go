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
	I18n      I18nConfig     `yaml:"i18n"`
	Storage   StorageConfig  `yaml:"storage"`
	StateFile string         `yaml:"state_file"` // 安装状态文件路径，默认 configs/state.yaml
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
}

type JWTConfig struct {
	Secret string `yaml:"secret"`
	Issuer string `yaml:"issuer"`
	TTL    string `yaml:"ttl"` // e.g. 24h
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
		return 24 * time.Hour
	}
	d, err := time.ParseDuration(j.TTL)
	if err != nil || d <= 0 {
		return 24 * time.Hour
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
		c.Auth.JWT.TTL = "24h"
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
	if provider := os.Getenv("STORAGE_PROVIDER"); provider != "" {
		c.Storage.Provider = provider
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
	if _, err := time.ParseDuration(c.Auth.JWT.TTL); err != nil {
		return fmt.Errorf("auth.jwt.ttl is invalid: %w", err)
	}
	return nil
}
