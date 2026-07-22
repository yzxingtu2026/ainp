package system

import (
	"context"
	"encoding/json"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
)

func registerProfileActions(a *app.App, users *UserService, prefs *UserSettingService) error {
	auditOn, auditOff := true, false
	actions := []action.Action{
		{ID: ActionProfileGet, Handler: users.SelfProfile, Meta: withAIMeta(authMeta(action.AccessAuthenticated, "个人资料", "action.system.profile_get", &auditOff, "GET", "/api/v1/system/profile"), readAIMeta("查询当前登录用户的个人资料、角色和有效称呼", "查询个人资料", "已返回当前用户个人资料", action.SchemaFor[struct{}](), action.SchemaFor[SelfProfileView](), "个人资料", "当前用户", "查询"))},
		{ID: ActionProfileUpdate, Handler: action.JSON(func(ctx context.Context, req selfProfileUpdateRequest) (any, error) {
			raw, _ := json.Marshal(req)
			return users.UpdateSelfProfile(ctx, raw)
		}), Meta: withAIMeta(authMeta(action.AccessAuthenticated, "更新个人资料", "action.system.profile_update", &auditOn, "POST", "/api/v1/system/profile/update"), writeAIMeta("更新当前登录用户的显示名称", "更新个人资料", "个人资料已更新", "medium", action.SchemaFor[selfProfileUpdateRequest](), action.SchemaFor[SelfProfileView](), "个人资料", "更新"))},
		{ID: ActionUserPreferencesGet, Handler: prefs.Get, Meta: withAIMeta(authMeta(action.AccessAuthenticated, "个人运行偏好", "action.system.user_preferences_get", &auditOff, "GET", "/api/v1/system/profile/preferences"), readAIMeta("查询当前登录用户的时区、语言和 AI 响应偏好", "查询个人运行偏好", "已返回当前用户运行偏好", action.SchemaFor[struct{}](), action.SchemaFor[SettingDefinitionView](), "个人偏好", "时区", "语言", "查询"))},
		{ID: ActionUserPreferencesUpdate, Handler: prefs.Update, Meta: withAIMeta(authMeta(action.AccessAuthenticated, "更新个人运行偏好", "action.system.user_preferences_update", &auditOn, "POST", "/api/v1/system/profile/preferences/update"), writeAIMeta("按当前 revision 更新当前登录用户的运行偏好", "更新个人运行偏好", "个人运行偏好已更新", "medium", action.SchemaFor[userSettingUpdateRequest](), action.SchemaFor[SettingDefinitionView](), "个人偏好", "更新", "时区", "语言"))},
	}
	for _, item := range actions {
		if err := a.RegisterAction(item); err != nil {
			return err
		}
	}
	return nil
}
