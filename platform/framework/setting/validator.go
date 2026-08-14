package setting

import (
	"context"
	"fmt"
	"sync"
)

type Validation struct {
	TenantID string
	ActorID  string
	Key      string
	Values   map[string]any
}

type Validator func(context.Context, Validation) error

type ValidatorRegistry struct {
	mu         sync.RWMutex
	validators map[string][]Validator
}

func NewValidatorRegistry() *ValidatorRegistry {
	return &ValidatorRegistry{validators: map[string][]Validator{}}
}

func (r *ValidatorRegistry) Register(key string, validator Validator) error {
	if key == "" || validator == nil {
		return fmt.Errorf("setting validator key and function are required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.validators[key] = append(r.validators[key], validator)
	return nil
}

func (r *ValidatorRegistry) Validate(ctx context.Context, validation Validation) error {
	if validation.TenantID == "" || validation.Key == "" {
		return fmt.Errorf("invalid setting validation")
	}
	r.mu.RLock()
	validators := append([]Validator(nil), r.validators[validation.Key]...)
	r.mu.RUnlock()
	for _, validator := range validators {
		if err := validator(ctx, validation); err != nil {
			return err
		}
	}
	return nil
}
