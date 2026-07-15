package idempotency

import (
	"context"
	"encoding/json"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
)

// ActionAdapter wraps Executor to implement action.IdempotencyExecutor interface.
type ActionAdapter struct {
	exec *Executor
}

// NewActionAdapter creates an adapter for the action registry.
func NewActionAdapter(exec *Executor) *ActionAdapter {
	return &ActionAdapter{exec: exec}
}

// Execute satisfies action.IdempotencyExecutor.
func (a *ActionAdapter) Execute(ctx context.Context, actionID string, input json.RawMessage, handler action.Handler, mode string, cacheBusinessErrors bool, ttl, inProgressTimeout time.Duration) (any, bool, error) {
	result, err := a.exec.Execute(ctx, ExecuteInput{
		ActionID: actionID,
		Input:    input,
		Meta: &IdempotencyConfig{
			Mode:                mode,
			TTL:                 ttl,
			InProgressTimeout:   inProgressTimeout,
			CacheBusinessErrors: cacheBusinessErrors,
		},
		Handler: HandlerFunc(handler),
	})
	if err != nil {
		return nil, false, err
	}
	return result.Data, result.IsReplay, nil
}
