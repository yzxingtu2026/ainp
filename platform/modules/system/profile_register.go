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
		{ID: ActionProfileGet, Handler: users.SelfProfile, Meta: authMeta(action.AccessAuthenticated, "个人资料", "action.system.profile_get", &auditOff, "GET", "/api/v1/system/profile")},
		{ID: ActionProfileUpdate, Handler: action.JSON(func(ctx context.Context, req selfProfileUpdateRequest) (any, error) {
			raw, _ := json.Marshal(req)
			return users.UpdateSelfProfile(ctx, raw)
		}), Meta: authMeta(action.AccessAuthenticated, "更新个人资料", "action.system.profile_update", &auditOn, "POST", "/api/v1/system/profile/update")},
		{ID: ActionUserPreferencesGet, Handler: prefs.Get, Meta: authMeta(action.AccessAuthenticated, "个人运行偏好", "action.system.user_preferences_get", &auditOff, "GET", "/api/v1/system/profile/preferences")},
		{ID: ActionUserPreferencesUpdate, Handler: prefs.Update, Meta: authMeta(action.AccessAuthenticated, "更新个人运行偏好", "action.system.user_preferences_update", &auditOn, "POST", "/api/v1/system/profile/preferences/update")},
	}
	for _, item := range actions {
		if err := a.RegisterAction(item); err != nil {
			return err
		}
	}
	return nil
}
