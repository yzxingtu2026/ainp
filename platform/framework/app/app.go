package app

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/attachment"
	"github.com/yzxingtu2026/ainp/platform/framework/audit"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/capability"
	"github.com/yzxingtu2026/ainp/platform/framework/codeset"
	"github.com/yzxingtu2026/ainp/platform/framework/config"
	"github.com/yzxingtu2026/ainp/platform/framework/httpserver"
	"github.com/yzxingtu2026/ainp/platform/framework/i18n"
	"github.com/yzxingtu2026/ainp/platform/framework/menu"
	"github.com/yzxingtu2026/ainp/platform/framework/permission"
	"github.com/yzxingtu2026/ainp/platform/framework/setting"
	"github.com/yzxingtu2026/ainp/platform/framework/statemachine"
	"github.com/yzxingtu2026/ainp/platform/framework/storage"
	"gorm.io/gorm"
)

type App struct {
	Config             *config.Config
	DB                 *gorm.DB
	Tokens             *auth.TokenService
	Authorization      auth.AuthorizationProvider
	Capabilities       *capability.Registry
	Actions            *action.Registry
	Menus              *menu.Registry
	CodeSets           *codeset.Registry
	Settings           *setting.Registry
	StateMachines      *statemachine.Registry
	Storage            storage.ObjectStore
	AttachmentPolicies *attachment.PolicyRegistry
	TransferTokens     *attachment.TransferTokenService
	OnHTTP             func(engine *gin.Engine)
	Auditor            audit.Recorder
	Installed          bool
}

func New(cfg *config.Config, db *gorm.DB, auditor audit.Recorder, tokens *auth.TokenService) *App {
	checker := permission.NewChecker()
	codeSets := codeset.NewRegistry()
	_ = codeSets.Register(codeset.ActivationStatus())
	_ = codeSets.Register(codeset.AttachmentStatus())
	return &App{
		Config:             cfg,
		DB:                 db,
		Tokens:             tokens,
		Capabilities:       capability.NewRegistry(),
		Actions:            action.NewRegistry(checker, auditor),
		Menus:              menu.NewRegistry(),
		CodeSets:           codeSets,
		Settings:           setting.NewRegistry(),
		StateMachines:      statemachine.NewRegistry(),
		AttachmentPolicies: attachment.NewPolicyRegistry(),
		Auditor:            auditor,
		Installed:          true,
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

func (a *App) RegisterCodeSet(set codeset.CodeSet) error { return a.CodeSets.Register(set) }

func (a *App) RegisterSettingCategory(c setting.Category) error {
	return a.Settings.RegisterCategory(c)
}

func (a *App) RegisterSettingDefinition(def setting.Definition) error {
	return a.Settings.RegisterDefinition(def)
}

func (a *App) RegisterStateMachine(machine statemachine.StateMachine) error {
	return a.StateMachines.Register(machine, a.CodeSets, a.Actions)
}

func (a *App) Run() error {
	// 1. 加载翻译文件
	dir := a.Config.I18n.Dir
	if dir == "" {
		dir = "i18n"
	}
	if err := i18n.LoadDir(dir); err != nil {
		return fmt.Errorf("i18n load: %w", err)
	}
	if a.Config.I18n.DefaultLanguage != "" {
		i18n.SetDefaultLang(a.Config.I18n.DefaultLanguage)
	}

	// 2. 启动 HTTP server
	engine := httpserver.New(httpserver.Options{
		Mode:            a.Config.Server.Mode,
		Installed:       &a.Installed,
		DevTokenEnabled: a.Config.Auth.DevTokenEnabled,
		Tokens:          a.Tokens,
		Authorization:   a.Authorization,
		Actions:         a.Actions,
		Menus:           a.Menus,
		CodeSets:        a.CodeSets,
		DB:              a.DB,
		OnHTTP:          a.OnHTTP,
	})
	return engine.Run(a.Config.Server.Addr)
}
