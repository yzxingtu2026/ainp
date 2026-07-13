package system

import (
	"context"
	"encoding/json"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/capability"
	"github.com/yzxingtu2026/ainp/platform/framework/menu"
)

func Register(a *app.App) error {
	if err := a.RegisterCapability(capability.Capability{
		ID:          "system",
		Name:        "系统",
		Description: "系统域：鉴权与基础能力",
		Module:      "system",
		Order:       0,
	}); err != nil {
		return err
	}

	svc := NewAuthService(a.DB)
	auditOn := true
	auditOff := false

	if err := a.RegisterAction(action.Action{
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
	}); err != nil {
		return err
	}

	if err := a.RegisterAction(action.Action{
		ID: "system.auth_login",
		Handler: func(ctx context.Context, input json.RawMessage) (any, error) {
			return svc.Login(ctx, a.Tokens, input)
		},
		Meta: action.Metadata{
			Capability:  "system",
			Name:        "登录",
			Description: "校验账号密码并签发 JWT",
			Audit:       &auditOn,
			HTTP: &action.HTTPMeta{
				Method: "POST",
				Path:   "/api/v1/system/auth/login",
				Public: true,
			},
		},
	}); err != nil {
		return err
	}

	if err := a.RegisterAction(action.Action{
		ID: "system.auth_me",
		Handler: func(ctx context.Context, input json.RawMessage) (any, error) {
			return svc.Me(ctx)
		},
		Meta: action.Metadata{
			Capability:  "system",
			Name:        "当前用户",
			Description: "返回当前登录用户摘要与权限",
			Audit:       &auditOff,
			HTTP: &action.HTTPMeta{
				Method: "GET",
				Path:   "/api/v1/system/auth/me",
			},
		},
	}); err != nil {
		return err
	}

	if err := a.RegisterAction(action.Action{
		ID: "system.auth_permissions",
		Handler: func(ctx context.Context, input json.RawMessage) (any, error) {
			return svc.Permissions(ctx)
		},
		Meta: action.Metadata{
			Capability:  "system",
			Name:        "权限列表",
			Description: "返回当前用户角色与 permission 列表",
			Audit:       &auditOff,
			HTTP: &action.HTTPMeta{
				Method: "GET",
				Path:   "/api/v1/system/auth/permissions",
			},
		},
	}); err != nil {
		return err
	}

	if err := a.RegisterMenu(menu.Menu{
		ID:     "system.root",
		Title:  "系统",
		Icon:   "mdi:cog-outline",
		Type:   menu.TypeDirectory,
		Module: "system",
		Order:  10,
	}); err != nil {
		return err
	}

	if err := a.RegisterMenu(menu.Menu{
		ID:         "system.profile",
		ParentID:   "system.root",
		Title:      "个人中心",
		Icon:       "mdi:account-circle-outline",
		Type:       menu.TypeMenu,
		Path:       "/system/profile",
		Component:  "system/profile",
		Permission: "system.auth_me",
		BindAction: "system.auth_me",
		Module:     "system",
		Order:      1,
	}); err != nil {
		return err
	}

	return a.RegisterMenu(menu.Menu{
		ID:         "system.profile_refresh",
		ParentID:   "system.profile",
		Title:      "刷新资料",
		Type:       menu.TypeButton,
		Permission: "system.auth_me",
		BindAction: "system.auth_me",
		HideInMenu: true,
		Module:     "system",
		Order:      1,
	})
}
