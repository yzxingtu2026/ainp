package system

import (
	"context"
	"encoding/json"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/capability"
)

func Register(a *app.App) error {
	if err := a.RegisterCapability(capability.Capability{
		ID:     "system",
		Name:   "系统",
		Module: "platform",
		Order:  0,
	}); err != nil {
		return err
	}

	auditOff := false
	return a.RegisterAction(action.Action{
		ID: "system.ping",
		Handler: func(ctx context.Context, input json.RawMessage) (any, error) {
			return map[string]any{"pong": true}, nil
		},
		Meta: action.Metadata{
			Capability: "system",
			Name:       "Ping",
			Audit:      &auditOff,
			HTTP: &action.HTTPMeta{
				Method: "GET",
				Path:   "/api/v1/system/ping",
				Public: true,
			},
		},
	})
}
