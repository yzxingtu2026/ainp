package system

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/runtimecontext"
	"github.com/yzxingtu2026/ainp/platform/framework/setting"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"github.com/yzxingtu2026/ainp/platform/modules/system/model"
	"gorm.io/gorm"
)

type RuntimeContextProvider struct {
	db             *gorm.DB
	preferences    *UserSettingService
	tenantSettings *setting.Provider
	clock          runtimecontext.Clock
}

func NewRuntimeContextProvider(db *gorm.DB, preferences *UserSettingService, tenantSettings *setting.Provider, clock runtimecontext.Clock) *RuntimeContextProvider {
	if clock == nil {
		clock = runtimecontext.ClockFunc(func() time.Time { return time.Now().UTC() })
	}
	return &RuntimeContextProvider{db: db, preferences: preferences, tenantSettings: tenantSettings, clock: clock}
}

func (p *RuntimeContextProvider) Build(ctx context.Context, req runtimecontext.BuildRequest) (*runtimecontext.TrustedRuntimeContext, error) {
	ac, ok := auth.FromContext(ctx)
	if !ok || ac == nil || !ac.Authenticated || ac.TenantID == "" || ac.UserID == "" || ac.TenantID != req.TenantID || ac.UserID != req.UserID {
		return nil, tenant.ErrTenantConflict
	}
	tdb, err := tenant.TenantDB(ctx, p.db)
	if err != nil {
		return nil, err
	}
	users, err := tenant.TenantRaw[model.User](tdb, `SELECT * FROM users WHERE tenant_id=? AND id=? AND status=? LIMIT 1`, req.UserID, status.ActivationEnabled)
	if err != nil {
		return nil, err
	}
	if len(users) != 1 {
		return nil, auth.ErrAuthorizationInvalid
	}
	user := users[0]
	prefs, prefSource, err := p.preferences.PreferencesWithSource(ctx)
	if err != nil {
		return nil, err
	}
	tenantTimezone, tenantLocale, tenantSource := runtimecontext.SystemFallbackTimezone, "zh-CN", "system_default"
	if p.tenantSettings != nil {
		if eff, loadErr := p.tenantSettings.Get(ctx, req.TenantID, "general.runtime_defaults"); loadErr == nil {
			if value, ok := eff.Values["timezone"].(string); ok && value != "" {
				tenantTimezone = value
			}
			if value, ok := eff.Values["locale"].(string); ok && value != "" {
				tenantLocale = value
			}
			if eff.Source == "tenant" {
				tenantSource = "tenant_default"
			}
		}
	}
	timezone, timezoneSource := strings.TrimSpace(prefs.Timezone), "user_preference"
	if prefSource != "user" {
		timezone, timezoneSource = tenantTimezone, tenantSource
	}
	var snap runtimecontext.TimeSnapshot
	if req.TimeSnapshot != nil {
		snap = *req.TimeSnapshot
		if err := snap.Validate(); err != nil {
			return nil, err
		}
	} else {
		snap, err = runtimecontext.NewTimeSnapshot(p.clock.Now(), timezone, timezoneSource)
		if err != nil {
			return nil, err
		}
	}
	preferred, preferredSource := strings.TrimSpace(prefs.PreferredAddressOverride), "user_preference"
	if preferred == "" {
		rows, queryErr := tenant.TenantRaw[struct{ Profile json.RawMessage }](tdb, `SELECT profile_json AS profile FROM ai_memory_current_profiles WHERE tenant_id=? AND user_id=? AND agent_id IS NULL AND status='ready' ORDER BY source_revision DESC LIMIT 1`, req.UserID)
		if queryErr != nil {
			return nil, queryErr
		}
		if len(rows) == 1 {
			var profile struct {
				PreferredAddress string `json:"preferred_address"`
			}
			if json.Unmarshal(rows[0].Profile, &profile) == nil {
				preferred = strings.TrimSpace(profile.PreferredAddress)
			}
		}
		preferredSource = "memory_profile"
	}
	if preferred == "" {
		preferred, preferredSource = strings.TrimSpace(user.DisplayName), "display_name"
	}
	if preferred == "" {
		preferred, preferredSource = user.Username, "username"
	}
	locale, localeSource := strings.TrimSpace(prefs.Locale), "user_preference"
	if prefSource != "user" {
		locale, localeSource = tenantLocale, tenantSource
	}
	return &runtimecontext.TrustedRuntimeContext{TenantID: req.TenantID, UserID: req.UserID, Username: user.Username, DisplayName: user.DisplayName, PreferredAddress: preferred, PreferredAddressSource: preferredSource, Locale: locale, LocaleSource: localeSource, AutoMemoryEnabled: prefs.AutoMemoryEnabled, ConversationID: req.ConversationID, ConversationCreatedAt: req.ConversationCreatedAt, AgentID: req.AgentID, AgentVersionID: req.AgentVersionID, RunID: req.RunID, Time: snap, ContextVersion: runtimecontext.ContextVersion, PolicyVersion: runtimecontext.PolicyVersion}, nil
}

func registerRuntimeContextDefaultsSetting(a interface {
	RegisterSettingDefinition(setting.Definition) error
}) error {
	return a.RegisterSettingDefinition(setting.Definition{Key: "general.runtime_defaults", CategoryKey: "general", Title: "Runtime 默认值", Description: "用户未主动设置时使用的可信运行时默认值", Scope: setting.ScopeTenant, Surface: auth.RealmTenant, Permission: ActionSettingList, Order: 30, SchemaVersion: 1, Fields: []setting.Field{
		{Key: "timezone", Type: setting.FieldSelect, Label: "默认时区", Required: true, Default: runtimecontext.ProductDefaultTimezone, Options: []setting.FieldOption{{Value: "UTC", Label: "UTC"}, {Value: "Asia/Shanghai", Label: "Asia/Shanghai"}, {Value: "America/New_York", Label: "America/New_York"}}},
		{Key: "locale", Type: setting.FieldSelect, Label: "默认语言", Required: true, Default: "zh-CN", Options: []setting.FieldOption{{Value: "zh-CN", Label: "简体中文"}, {Value: "en-US", Label: "English"}}},
	}})
}
