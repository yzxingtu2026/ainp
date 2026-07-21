package setting

import (
	"context"
	"fmt"
	"sync"
)

type ChangeOperation string

const (
	ChangeUpdated ChangeOperation = "updated"
	ChangeReset   ChangeOperation = "reset"
)

type Change struct {
	TenantID  string
	UserID    string
	Key       string
	Operation ChangeOperation
}

type ChangeHandler func(context.Context, Change) error

type ChangeRegistry struct {
	mu       sync.RWMutex
	handlers map[string][]ChangeHandler
}

func NewChangeRegistry() *ChangeRegistry {
	return &ChangeRegistry{handlers: map[string][]ChangeHandler{}}
}

func (r *ChangeRegistry) Register(key string, handler ChangeHandler) error {
	if key == "" || handler == nil {
		return fmt.Errorf("setting change key and handler are required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[key] = append(r.handlers[key], handler)
	return nil
}

func (r *ChangeRegistry) Notify(ctx context.Context, change Change) error {
	if change.TenantID == "" || change.Key == "" {
		return fmt.Errorf("invalid setting change")
	}
	r.mu.RLock()
	handlers := append([]ChangeHandler{}, r.handlers[change.Key]...)
	r.mu.RUnlock()
	for _, handler := range handlers {
		if err := handler(ctx, change); err != nil {
			return err
		}
	}
	return nil
}
