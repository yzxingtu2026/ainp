package system

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
)

type MCPConnectionInfoView struct {
	Name         string   `json:"name"`
	Transport    string   `json:"transport"`
	URL          string   `json:"url"`
	OAuthEnabled bool     `json:"oauth_enabled"`
	Scopes       []string `json:"scopes"`
}

func registerProfileActions(a *app.App, users *UserService, prefs *UserSettingService) error {
	auditOn, auditOff := true, false
	actions := []action.Action{
		{ID: ActionProfileGet, Handler: users.SelfProfile, Meta: withAIMeta(authMeta(action.AccessAuthenticated, "个人资料", "action.system.profile_get", &auditOff, "GET", "/api/v1/system/profile"), readAIMeta("查询当前登录用户的个人资料、角色和有效称呼", "查询个人资料", "查询当前登录用户的个人资料、角色和有效称呼", "已返回当前用户个人资料", action.InputSchemaFor[struct{}](), action.SchemaFor[SelfProfileView](), "个人资料", "当前用户", "查询"))},
		{ID: ActionProfileUpdate, Handler: action.JSON(func(ctx context.Context, req selfProfileUpdateRequest) (any, error) {
			raw, _ := json.Marshal(req)
			return users.UpdateSelfProfile(ctx, raw)
		}), Meta: withAIMeta(authMeta(action.AccessAuthenticated, "更新个人资料", "action.system.profile_update", &auditOn, "POST", "/api/v1/system/profile/update"), writeAIMeta("更新当前登录用户的显示名称和头像", "更新个人资料", "更新当前登录用户的显示名称和头像", "个人资料已更新", "medium", action.InputSchemaFor[selfProfileUpdateRequest](), action.SchemaFor[SelfProfileView](), "个人资料", "更新", "头像"))},
		{ID: ActionProfilePasswordUpdate, Handler: action.JSON(func(ctx context.Context, req selfPasswordUpdateRequest) (any, error) {
			raw, _ := json.Marshal(req)
			return users.UpdateSelfPassword(ctx, raw)
		}), Meta: action.Metadata{Capability: "system", Access: action.AccessAuthenticated, Name: "修改当前用户密码", NameKey: "action.system.profile_password_update.name", Description: "校验当前密码并更新当前登录用户密码，使全部旧会话失效", DescriptionKey: "action.system.profile_password_update.description", Audit: &auditOn, AI: action.ExcludeFromAI(action.AIExcludeSensitiveCredential), HTTP: &action.HTTPMeta{Method: "POST", Path: "/api/v1/system/profile/password/update"}}},
		{ID: ActionUserPreferencesGet, Handler: prefs.Get, Meta: withAIMeta(authMeta(action.AccessAuthenticated, "个人运行偏好", "action.system.user_preferences_get", &auditOff, "GET", "/api/v1/system/profile/preferences"), readAIMeta("查询当前登录用户的时区、语言和 AI 响应偏好", "查询个人运行偏好", "查询当前登录用户的时区、语言和 AI 响应偏好", "已返回当前用户运行偏好", action.InputSchemaFor[struct{}](), action.SchemaFor[SettingDefinitionView](), "个人偏好", "时区", "语言", "查询"))},
		{ID: ActionUserPreferencesUpdate, Handler: prefs.Update, Meta: withAIMeta(authMeta(action.AccessAuthenticated, "更新个人运行偏好", "action.system.user_preferences_update", &auditOn, "POST", "/api/v1/system/profile/preferences/update"), writeAIMeta("按当前 revision 更新当前登录用户的运行偏好", "更新个人运行偏好", "按当前 revision 更新当前登录用户的运行偏好", "个人运行偏好已更新", "medium", action.InputSchemaFor[userSettingUpdateRequest](), action.SchemaFor[SettingDefinitionView](), "个人偏好", "更新", "时区", "语言"))},
		{ID: ActionMCPConnectionInfo, Handler: func(context.Context, json.RawMessage) (any, error) {
			return MCPConnectionInfoView{
				Name: "AINP", Transport: "streamable_http", URL: strings.TrimRight(a.Config.MCP.BaseURL, "/") + "/mcp",
				OAuthEnabled: true, Scopes: []string{"actions:read", "actions:write", "actions:sensitive"},
			}, nil
		}, Meta: action.Metadata{Capability: "system", Access: action.AccessAuthenticated, Name: "MCP 连接信息", Description: "返回当前部署的远程 MCP 连接地址和 OAuth 能力", Audit: &auditOff, AI: action.ExcludeFromAI(action.AIExcludeInternalRuntime), HTTP: &action.HTTPMeta{Method: "GET", Path: "/api/v1/system/profile/mcp-connection"}}},
	}
	for _, item := range actions {
		if err := a.RegisterAction(item); err != nil {
			return err
		}
	}
	return nil
}
