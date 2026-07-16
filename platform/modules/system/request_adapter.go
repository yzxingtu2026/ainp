package system

import (
	"context"
	"encoding/json"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
)

// typedAction keeps legacy service entry points usable while registered Actions
// adopt the shared decode, normalize, and validation lifecycle.
func typedAction[T any](handler func(context.Context, json.RawMessage) (any, error)) action.Handler {
	return action.JSON(func(ctx context.Context, request T) (any, error) {
		input, err := json.Marshal(request)
		if err != nil {
			return nil, errs.Wrap(errs.CodeInternal, "encode typed request failed", err)
		}
		return handler(ctx, input)
	})
}
