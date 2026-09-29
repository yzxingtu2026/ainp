package state

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/config"
	"gopkg.in/yaml.v3"
)

// State 持久化安装状态，写入独立文件以保持主 config.yaml 只读。
type State struct {
	Installed       bool      `yaml:"installed"`
	InstalledAt     time.Time `yaml:"installed_at,omitempty"`
	InitialTenantID string    `yaml:"initial_tenant_id,omitempty"`
	DSN             string    `yaml:"dsn,omitempty"` // 向导写入的实际 DSN（敏感，生产建议 env 注入）
	Redis           Redis     `yaml:"redis,omitempty"`
	Jobs            Jobs      `yaml:"jobs,omitempty"`
}

type Redis struct {
	Addr string `yaml:"addr,omitempty"`
	DB   int    `yaml:"db,omitempty"`
}

type Jobs struct {
	Enabled        *bool `yaml:"enabled,omitempty"`
	EmbeddedWorker bool  `yaml:"embedded_worker,omitempty"`
}

// ApplyConfig applies setup-owned runtime values after config and environment loading.
// Secrets remain environment-owned and are never persisted in state.yaml.
func (s *State) ApplyConfig(cfg *config.Config) {
	if cfg.Database.DSN == "" && s.DSN != "" {
		cfg.Database.DSN = s.DSN
	}
	if s.Redis.Addr != "" && os.Getenv("REDIS_ADDR") == "" {
		cfg.Redis.Addr = s.Redis.Addr
		cfg.Redis.DB = s.Redis.DB
	}
	if s.Jobs.Enabled != nil {
		cfg.Jobs.Enabled = s.Jobs.Enabled
		cfg.Jobs.EmbeddedWorker = s.Jobs.EmbeddedWorker
	}
}

// DefaultPath 默认状态文件路径。
const DefaultPath = "configs/state.yaml"

// Load 从指定路径加载状态文件；文件不存在时返回零值（Installed: false）。
func Load(path string) (*State, error) {
	if path == "" {
		path = DefaultPath
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &State{}, nil
		}
		return nil, fmt.Errorf("read state: %w", err)
	}
	var s State
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse state: %w", err)
	}
	return &s, nil
}

// Save 原子写入状态文件（write tmp + rename）。
func Save(path string, s *State) error {
	if path == "" {
		path = DefaultPath
	}
	data, err := yaml.Marshal(s)
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir state dir: %w", err)
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write state tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename state: %w", err)
	}
	return nil
}
