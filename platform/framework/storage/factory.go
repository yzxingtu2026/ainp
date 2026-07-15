package storage

import (
	"fmt"
	"strings"
)

// Config is the YAML-backed storage section.
type Config struct {
	Provider string      `yaml:"provider"`
	Local    LocalConfig `yaml:"local"`
}

// NewFromConfig constructs the configured ObjectStore.
func NewFromConfig(cfg Config, metrics MetricsRecorder) (ObjectStore, error) {
	provider := strings.TrimSpace(strings.ToLower(cfg.Provider))
	if provider == "" {
		provider = ProviderLocal
	}
	switch provider {
	case ProviderLocal:
		return NewLocalObjectStore(cfg.Local, metrics)
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnknownProvider, provider)
	}
}
