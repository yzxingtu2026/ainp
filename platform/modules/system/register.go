package system

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/attachment"
	"github.com/yzxingtu2026/ainp/platform/framework/capability"
	"github.com/yzxingtu2026/ainp/platform/framework/i18n"
	"github.com/yzxingtu2026/ainp/platform/framework/menu"
	"github.com/yzxingtu2026/ainp/platform/modules/system/jobs"
)

func Register(a *app.App) error {
	if err := a.RegisterCapability(capability.Capability{
		ID:             "system",
		Name:           "系统",
		NameKey:        "capability.system.name",
		Description:    "系统域：鉴权与基础能力",
		DescriptionKey: "capability.system.description",
		Module:         "system",
		Order:          0,
	}); err != nil {
		return err
	}

	svc := NewAuthService(a.DB)
	a.Authorization = svc
	if a.Authenticators == nil {
		return fmt.Errorf("authenticator registry is required")
	}
	if err := a.Authenticators.Register(NewPasswordAuthenticator(svc)); err != nil {
		return err
	}
	sessionSvc := NewSessionService(a.DB, a.Tokens, svc, a.Config.Auth.RefreshDuration())
	unifiedAuth := NewUnifiedAuthService(a.DB, a.Authenticators, svc, sessionSvc, a.AuthRateLimiter)
	auditOn := true
	auditOff := false

	if err := a.RegisterAction(action.Action{
		ID: "system.ping",
		Handler: func(ctx context.Context, input json.RawMessage) (any, error) {
			return map[string]any{"pong": true}, nil
		},
		Meta: action.Metadata{
			Capability: "system",
			Access:     action.AccessPublic,
			Name:       "Ping",
			NameKey:    "action.system.ping.name",
			Audit:      &auditOff,
			HTTP: &action.HTTPMeta{
				Method: "GET",
				Path:   "/api/v1/system/ping",
			},
		},
	}); err != nil {
		return err
	}

	roleSvc := NewRoleService(a.DB, svc, a.Actions, a.Capabilities)
	if err := registerRoleActions(a, roleSvc); err != nil {
		return err
	}

	userSvc := NewUserService(a.DB, svc)
	if a.Outbox != nil {
		userSvc.SetOutboxWriter(a.Outbox.Writer)
	}
	if err := registerUserActions(a, userSvc); err != nil {
		return err
	}
	if a.PageContexts == nil {
		return fmt.Errorf("page context registry is required")
	}
	if err := a.PageContexts.Register(PageTypeSystemUsers, userPageContextResolver{db: a.DB}); err != nil {
		return err
	}
	if err := registerRuntimePreferencesSetting(a); err != nil {
		return err
	}
	if err := registerExampleSettings(a); err != nil {
		return err
	}
	if err := registerRuntimeContextDefaultsSetting(a); err != nil {
		return err
	}
	settingSvc := NewSettingService(a.DB, a.Settings)
	a.SettingValues = settingSvc.Provider()
	if err := registerSettingActions(a, settingSvc); err != nil {
		return err
	}
	userSettingSvc := NewUserSettingService(a.DB, a.Settings)
	a.RuntimeContextProvider = NewRuntimeContextProvider(a.DB, userSettingSvc, a.SettingValues, nil)
	if err := registerProfileActions(a, userSvc, userSettingSvc); err != nil {
		return err
	}

	if a.TransferTokens == nil {
		return fmt.Errorf("attachment transfer token service is required")
	}
	if a.Storage == nil {
		return fmt.Errorf("object store is required for attachments")
	}
	if a.AttachmentPolicies == nil {
		a.AttachmentPolicies = attachment.NewPolicyRegistry()
	}
	policyProvider := NewSettingUploadPolicyProvider(settingSvc.Provider())
	attachSvc := NewAttachmentService(a.DB, a.Storage, a.TransferTokens, a.AttachmentPolicies, policyProvider)
	a.AIAssetResolver = attachSvc
	a.AttachmentResourceLinker = attachSvc
	if a.Outbox != nil {
		attachSvc.SetOutboxWriter(a.Outbox.Writer)
	}
	if err := registerAttachmentActions(a, attachSvc); err != nil {
		return err
	}
	if err := jobs.Register(a); err != nil {
		return err
	}
	a.OnHTTP = func(engine *gin.Engine) {
		RegisterAttachmentTransferRoutes(engine, attachSvc)
	}

	if err := a.RegisterAction(action.Action{
		ID: "system.auth_login",
		Handler: action.JSON(func(ctx context.Context, input LegacyLoginRequest) (any, error) {
			return unifiedAuth.LegacyLogin(ctx, input)
		}),
		Meta: action.Metadata{
			Capability:     "system",
			Access:         action.AccessPublic,
			Name:           "登录",
			NameKey:        "action.system.auth_login.name",
			Description:    "通过统一认证与会话链路完成密码登录",
			DescriptionKey: "action.system.auth_login.description",
			Audit:          &auditOn,
			HTTP: &action.HTTPMeta{
				Method: "POST",
				Path:   "/api/v1/system/auth/login",
			},
		},
	}); err != nil {
		return err
	}

	if err := registerAuthenticationActions(a, unifiedAuth, auditOn, auditOff); err != nil {
		return err
	}

	if err := a.RegisterAction(action.Action{
		ID: "system.auth_me",
		Handler: func(ctx context.Context, input json.RawMessage) (any, error) {
			return svc.Me(ctx)
		},
		Meta: action.Metadata{
			Capability:     "system",
			Access:         action.AccessAuthenticated,
			Name:           "当前用户",
			NameKey:        "action.system.auth_me.name",
			Description:    "返回当前登录用户摘要与权限",
			DescriptionKey: "action.system.auth_me.description",
			Audit:          &auditOff,
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
			Capability:     "system",
			Access:         action.AccessAuthenticated,
			Name:           "权限列表",
			NameKey:        "action.system.auth_permissions.name",
			Description:    "返回当前用户角色与 permission 列表",
			DescriptionKey: "action.system.auth_permissions.description",
			Audit:          &auditOff,
			HTTP: &action.HTTPMeta{
				Method: "GET",
				Path:   "/api/v1/system/auth/permissions",
			},
		},
	}); err != nil {
		return err
	}

	// i18n reload：AI Agent 新增语言后触发重载
	if err := a.RegisterAction(action.Action{
		ID: "system.i18n_reload",
		Handler: func(ctx context.Context, input json.RawMessage) (any, error) {
			if err := i18n.Reload(); err != nil {
				return nil, err
			}
			return map[string]any{
				"langs":     i18n.AvailableLangs(),
				"key_count": i18n.KeyCount(),
			}, nil
		},
		Meta: action.Metadata{
			Capability:     "system",
			Name:           "重载翻译",
			NameKey:        "action.system.i18n_reload.name",
			Description:    "重新加载 i18n 翻译文件",
			DescriptionKey: "action.system.i18n_reload.description",
			Audit:          &auditOn,
			HTTP: &action.HTTPMeta{
				Method: "POST",
				Path:   "/api/v1/system/i18n/reload",
			},
		},
	}); err != nil {
		return err
	}

	if err := a.RegisterMenu(menu.Menu{
		ID:       "system.root",
		Title:    "系统",
		TitleKey: "menu.system.root.title",
		Icon:     "mdi:cog-outline",
		Type:     menu.TypeDirectory,
		Module:   "system",
		Order:    10,
	}); err != nil {
		return err
	}

	if err := a.RegisterMenu(menu.Menu{
		ID:         "system.profile",
		ParentID:   "system.root",
		Title:      "个人中心",
		TitleKey:   "menu.system.profile.title",
		Icon:       "mdi:account-circle-outline",
		Type:       menu.TypeMenu,
		Path:       "/system/profile",
		Component:  "system/profile",
		BindAction: "system.auth_me",
		HideInMenu: true,
		Module:     "system",
		Order:      1,
	}); err != nil {
		return err
	}

	if err := a.RegisterMenu(menu.Menu{
		ID:         "system.profile_refresh",
		ParentID:   "system.profile",
		Title:      "刷新资料",
		TitleKey:   "menu.system.profile_refresh.title",
		Type:       menu.TypeButton,
		BindAction: "system.auth_me",
		HideInMenu: true,
		Module:     "system",
		Order:      1,
	}); err != nil {
		return err
	}

	if err := a.RegisterMenu(menu.Menu{
		ID: MenuUserManagement, ParentID: "system.root", Title: "用户管理", TitleKey: "menu.system.users.title",
		Icon: "mdi:account-outline", Type: menu.TypeMenu, Path: "/system/users", Component: "system/users",
		Permission: ActionUserList, BindAction: ActionUserList, Module: "system", Order: 2,
	}); err != nil {
		return err
	}

	if err := a.RegisterMenu(menu.Menu{
		ID: MenuRoleManagement, ParentID: "system.root", Title: "角色管理", TitleKey: "menu.system.roles.title",
		Icon: "mdi:shield-account-outline", Type: menu.TypeMenu, Path: "/system/roles", Component: "system/roles",
		Permission: ActionRoleList, BindAction: ActionRoleList, Module: "system", Order: 3,
	}); err != nil {
		return err
	}

	if err := a.RegisterMenu(menu.Menu{
		ID: MenuSettingManagement, ParentID: "system.root", Title: "系统设置", TitleKey: "menu.system.settings.title",
		Icon: "mdi:tune-vertical", Type: menu.TypeMenu, Path: "/system/settings", Component: "system/settings",
		Permission: ActionSettingList, BindAction: ActionSettingList, Module: "system", Order: 4,
	}); err != nil {
		return err
	}

	if err := a.RegisterMenu(menu.Menu{
		ID: MenuAttachmentCenter, ParentID: "system.root", Title: "附件中心", TitleKey: "menu.system.attachments.title",
		Icon: "mdi:paperclip", Type: menu.TypeMenu, Path: "/system/attachments", Component: "system/attachments",
		Permission: ActionAttachmentList, BindAction: ActionAttachmentList, Module: "system", Order: 5,
	}); err != nil {
		return err
	}

	if err := registerNotificationActions(a); err != nil {
		return err
	}

	return nil
}
