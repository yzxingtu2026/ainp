package app

import (
	"fmt"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/audit"
	"github.com/yzxingtu2026/ainp/platform/framework/capability"
	"github.com/yzxingtu2026/ainp/platform/framework/config"
	"github.com/yzxingtu2026/ainp/platform/framework/httpserver"
	"github.com/yzxingtu2026/ainp/platform/framework/menu"
	"github.com/yzxingtu2026/ainp/platform/framework/permission"
	"gorm.io/gorm"
)

type App struct {
	Config       *config.Config
	DB           *gorm.DB
	Capabilities *capability.Registry
	Actions      *action.Registry
	Menus        *menu.Registry
	Auditor      audit.Recorder
}

func New(cfg *config.Config, db *gorm.DB, auditor audit.Recorder) *App {
	checker := permission.NewChecker()
	return &App{
		Config:       cfg,
		DB:           db,
		Capabilities: capability.NewRegistry(),
		Actions:      action.NewRegistry(checker, auditor),
		Menus:        menu.NewRegistry(),
		Auditor:      auditor,
	}
}

func (a *App) RegisterCapability(c capability.Capability) error {
	return a.Capabilities.Register(c)
}

func (a *App) RegisterAction(act action.Action) error {
	if _, ok := a.Capabilities.Get(act.Meta.Capability); !ok {
		return fmt.Errorf("capability not registered: %s", act.Meta.Capability)
	}
	return a.Actions.Register(act)
}

func (a *App) RegisterMenu(m menu.Menu) error {
	if m.BindAction != "" {
		if _, ok := a.Actions.Get(m.BindAction); !ok {
			return fmt.Errorf("bind_action not registered: %s", m.BindAction)
		}
	}
	if m.Permission != "" {
		// permission 通常等于某 action.id；若 action 已注册则更稳，但不强制（可先注册菜单）
	}
	return a.Menus.Register(m)
}

func (a *App) Run() error {
	engine := httpserver.New(httpserver.Options{
		Mode:            a.Config.Server.Mode,
		DevTokenEnabled: a.Config.Auth.DevTokenEnabled,
		Actions:         a.Actions,
		Menus:           a.Menus,
		DB:              a.DB,
	})
	return engine.Run(a.Config.Server.Addr)
}
