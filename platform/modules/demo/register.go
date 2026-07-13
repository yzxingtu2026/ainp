package demo

import (
	"context"
	"encoding/json"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/capability"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/menu"
)

func Register(a *app.App) error {
	if err := a.RegisterCapability(capability.Capability{
		ID:          "demo",
		Name:        "演示",
		Description: "Phase 1 骨架演示能力",
		Module:      "demo",
		Order:       100,
	}); err != nil {
		return err
	}

	if err := a.RegisterAction(action.Action{
		ID: "demo.hello",
		Handler: func(ctx context.Context, input json.RawMessage) (any, error) {
			ac := auth.MustFromContext(ctx)
			var req struct {
				Name string `json:"name"`
			}
			if len(input) > 0 {
				_ = json.Unmarshal(input, &req)
			}
			if req.Name == "" {
				req.Name = "AINP"
			}
			return map[string]any{
				"message":   "hello " + req.Name,
				"tenant_id": ac.TenantID,
				"user_id":   ac.UserID,
			}, nil
		},
		Meta: action.Metadata{
			Capability:  "demo",
			Name:        "Hello",
			Description: "演示需鉴权的 Action",
			HTTP: &action.HTTPMeta{
				Method: "POST",
				Path:   "/api/v1/demo/hello",
			},
		},
	}); err != nil {
		return err
	}

	if err := a.RegisterAction(action.Action{
		ID: "demo.echo",
		Handler: func(ctx context.Context, input json.RawMessage) (any, error) {
			if len(input) == 0 {
				return nil, errs.Validation("body required")
			}
			var payload any
			if err := json.Unmarshal(input, &payload); err != nil {
				return nil, errs.Validation("invalid json")
			}
			return map[string]any{"echo": payload}, nil
		},
		Meta: action.Metadata{
			Capability: "demo",
			Name:       "Echo",
			HTTP: &action.HTTPMeta{
				Method: "POST",
				Path:   "/api/v1/demo/echo",
			},
		},
	}); err != nil {
		return err
	}

	if err := a.RegisterMenu(menu.Menu{
		ID:     "demo.root",
		Title:  "演示",
		Icon:   "mdi:flask-outline",
		Type:   menu.TypeDirectory,
		Module: "demo",
		Order:  100,
	}); err != nil {
		return err
	}

	if err := a.RegisterMenu(menu.Menu{
		ID:         "demo.hello_page",
		ParentID:   "demo.root",
		Title:      "Hello",
		Icon:       "mdi:hand-wave-outline",
		Type:       menu.TypeMenu,
		Path:       "/demo/hello",
		Component:  "demo/hello",
		Permission: "demo.hello",
		BindAction: "demo.hello",
		Module:     "demo",
		Order:      1,
	}); err != nil {
		return err
	}

	return a.RegisterMenu(menu.Menu{
		ID:         "demo.hello_btn",
		ParentID:   "demo.hello_page",
		Title:      "打个招呼",
		Type:       menu.TypeButton,
		Permission: "demo.hello",
		BindAction: "demo.hello",
		HideInMenu: true,
		Module:     "demo",
		Order:      1,
	})
}
