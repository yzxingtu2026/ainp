package system

import (
	"context"
	"encoding/json"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/capability"
)

// RegisterSetupActions 注册 setup 系列 Action（setup / normal 模式均调用）。
func RegisterSetupActions(a *app.App, svc *SetupService) error {
	if err := a.RegisterCapability(capability.Capability{
		ID:             "setup",
		Name:           "安装向导",
		NameKey:        "capability.setup.name",
		Description:    "系统初始化安装向导",
		DescriptionKey: "capability.setup.description",
		Module:         "system",
		Order:          -1,
	}); err != nil {
		return err
	}

	auditOn := true
	auditOff := false

	// setup.status — 始终可用，返回安装状态
	if err := a.RegisterAction(action.Action{
		ID: "setup.status",
		Handler: func(ctx context.Context, input json.RawMessage) (any, error) {
			return svc.Status(ctx, input)
		},
		Meta: action.Metadata{
			Capability:     "setup",
			Access:         action.AccessPublic,
			Name:           "安装状态",
			NameKey:        "action.setup.status.name",
			Description:    "返回当前安装状态",
			DescriptionKey: "action.setup.status.description",
			Audit:          &auditOff,
			HTTP: &action.HTTPMeta{
				Method: "GET",
				Path:   "/api/v1/setup/status",
			},
		},
	}); err != nil {
		return err
	}

	// setup.check_env — 环境检查
	if err := a.RegisterAction(action.Action{
		ID: "setup.check_env",
		Handler: func(ctx context.Context, input json.RawMessage) (any, error) {
			return svc.CheckEnv(ctx, input)
		},
		Meta: action.Metadata{
			Capability:     "setup",
			Access:         action.AccessPublic,
			Name:           "环境检查",
			NameKey:        "action.setup.check_env.name",
			Description:    "检查运行环境是否就绪",
			DescriptionKey: "action.setup.check_env.description",
			Audit:          &auditOff,
			HTTP: &action.HTTPMeta{
				Method: "GET",
				Path:   "/api/v1/setup/check-env",
			},
		},
	}); err != nil {
		return err
	}

	// setup.test_db — 测试数据库连通性
	if err := a.RegisterAction(action.Action{
		ID: "setup.test_db",
		Handler: func(ctx context.Context, input json.RawMessage) (any, error) {
			return svc.TestDB(ctx, input)
		},
		Meta: action.Metadata{
			Capability:     "setup",
			Access:         action.AccessPublic,
			Name:           "测试数据库",
			NameKey:        "action.setup.test_db.name",
			Description:    "测试数据库连通性",
			DescriptionKey: "action.setup.test_db.description",
			Audit:          &auditOff,
			HTTP: &action.HTTPMeta{
				Method: "POST",
				Path:   "/api/v1/setup/test-db",
			},
		},
	}); err != nil {
		return err
	}

	// setup.initialize — 创建初始租户+管理员
	if err := a.RegisterAction(action.Action{
		ID: "setup.initialize",
		Handler: func(ctx context.Context, input json.RawMessage) (any, error) {
			return svc.Initialize(ctx, input)
		},
		Meta: action.Metadata{
			Capability:     "setup",
			Access:         action.AccessPublic,
			Name:           "初始化系统",
			NameKey:        "action.setup.initialize.name",
			Description:    "创建初始租户与管理员账号",
			DescriptionKey: "action.setup.initialize.description",
			Audit:          &auditOn,
			HTTP: &action.HTTPMeta{
				Method: "POST",
				Path:   "/api/v1/setup/initialize",
			},
		},
	}); err != nil {
		return err
	}

	return nil
}
