package system

import (
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/runtimecontext"
	"github.com/yzxingtu2026/ainp/platform/framework/setting"
	"github.com/yzxingtu2026/ainp/platform/modules/system/settings"
	"gorm.io/gorm"
)

type (
	SettingService         = settings.SettingService
	SettingFieldView       = settings.SettingFieldView
	SettingDefinitionView  = settings.SettingDefinitionView
	SettingCategoryView    = settings.SettingCategoryView
	Preferences            = settings.Preferences
	RuntimePreferences     = settings.RuntimePreferences
	UserSettingService     = settings.UserSettingService
	RuntimeContextProvider = settings.RuntimeContextProvider
	MCPConnectionInfoView  = settings.MCPConnectionInfoView
)

func NewSettingService(db *gorm.DB, registry *setting.Registry) *SettingService {
	return settings.NewSettingService(db, registry)
}
func NewSettingValueStore(db *gorm.DB) setting.ValueStore { return settings.NewSettingValueStore(db) }
func NewUserSettingService(db *gorm.DB, registry *setting.Registry) *UserSettingService {
	return settings.NewUserSettingService(db, registry)
}
func NewRuntimeContextProvider(db *gorm.DB, preferences *UserSettingService, tenantSettings *setting.Provider, clock runtimecontext.Clock) *RuntimeContextProvider {
	return settings.NewRuntimeContextProvider(db, preferences, tenantSettings, clock)
}

func registerRuntimePreferencesSetting(a *app.App) error {
	return settings.RegisterRuntimePreferencesSetting(a)
}
func registerExampleSettings(a *app.App) error { return settings.RegisterExampleSettings(a) }
func registerRuntimeContextDefaultsSetting(a *app.App) error {
	return settings.RegisterRuntimeContextDefaultsSetting(a)
}
func registerSettingActions(a *app.App, svc *SettingService) error {
	return settings.RegisterSettingActions(a, svc)
}
func registerProfileActions(a *app.App, users *UserService, prefs *UserSettingService) error {
	return settings.RegisterProfileActions(a, users, prefs)
}
