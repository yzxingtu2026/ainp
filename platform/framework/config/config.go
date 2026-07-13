package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	Logging  LoggingConfig  `yaml:"logging"`
	Auth     AuthConfig     `yaml:"auth"`
	I18n     I18nConfig     `yaml:"i18n"`
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
	if err := cfg.validate(); err != nil {
		return nil, err
	}
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
}

func (c *Config) validate() error {
	if c.Database.DSN == "" {
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
