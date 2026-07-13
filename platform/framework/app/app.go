package app

import (
	"fmt"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/audit"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/capability"
	"github.com/yzxingtu2026/ainp/platform/framework/config"
	"github.com/yzxingtu2026/ainp/platform/framework/httpserver"
	"github.com/yzxingtu2026/ainp/platform/framework/i18n"
	"github.com/yzxingtu2026/ainp/platform/framework/menu"
	"github.com/yzxingtu2026/ainp/platform/framework/permission"
	"gorm.io/gorm"
)

type App struct {
	Config       *config.Config
	DB           *gorm.DB
	Tokens       *auth.TokenService
	Capabilities *capability.Registry
	Actions      *action.Registry
	Menus        *menu.Registry
	Auditor      audit.Recorder
}

func New(cfg *config.Config, db *gorm.DB, auditor audit.Recorder, tokens *auth.TokenService) *App {
	checker := permission.NewChecker()
	return &App{
		Config:       cfg,
		DB:           db,
		Tokens:       tokens,
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
	return a.Menus.Register(m)
}

func (a *App) Run() error {
	// 初始化默认语言
	if a.Config.I18n.DefaultLanguage != "" {
		i18n.SetDefaultLang(a.Config.I18n.DefaultLanguage)
	}

	engine := httpserver.New(httpserver.Options{
		Mode:            a.Config.Server.Mode,
		DevTokenEnabled: a.Config.Auth.DevTokenEnabled,
		Tokens:          a.Tokens,
		Actions:         a.Actions,
		Menus:           a.Menus,
		DB:              a.DB,
	})
	return engine.Run(a.Config.Server.Addr)
}
