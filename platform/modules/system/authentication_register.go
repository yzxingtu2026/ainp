package system

import (
	"context"
	"encoding/json"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
)

type RevokeSessionRequest struct {
	SessionID string `json:"session_id" validate:"required,max=64"`
}

func registerAuthenticationActions(a *app.App, service *UnifiedAuthService, auditOn, auditOff bool) error {
	actions := []action.Action{
		{ID: "system.auth_methods", Handler: func(context.Context, json.RawMessage) (any, error) { return service.Methods(), nil }, Meta: authMeta(action.AccessPublic, "认证方式", "action.system.auth_methods", &auditOff, "GET", "/api/v1/system/auth/methods")},
		{ID: "system.auth_challenge", Handler: action.JSON(func(ctx context.Context, req ChallengeRequest) (any, error) { return service.Begin(ctx, req) }), Meta: authMeta(action.AccessPublic, "创建认证挑战", "action.system.auth_challenge", &auditOn, "POST", "/api/v1/system/auth/challenges")},
		{ID: "system.auth_authenticate", Handler: action.JSON(func(ctx context.Context, req AuthenticateRequest) (any, error) { return service.Authenticate(ctx, req) }), Meta: authMeta(action.AccessPublic, "统一认证", "action.system.auth_authenticate", &auditOn, "POST", "/api/v1/system/auth/authenticate")},
		{ID: "system.auth_refresh", Handler: action.JSON(func(ctx context.Context, req RefreshRequest) (any, error) { return service.Refresh(ctx, req) }), Meta: authMeta(action.AccessPublic, "刷新会话", "action.system.auth_refresh", &auditOn, "POST", "/api/v1/system/auth/refresh")},
		{ID: "system.auth_logout", Handler: func(ctx context.Context, _ json.RawMessage) (any, error) { return service.RevokeCurrent(ctx) }, Meta: authMeta(action.AccessAuthenticated, "退出登录", "action.system.auth_logout", &auditOn, "POST", "/api/v1/system/auth/logout")},
		{ID: "system.auth_session_list", Handler: func(ctx context.Context, _ json.RawMessage) (any, error) { return service.ListSessions(ctx) }, Meta: authMeta(action.AccessAuthenticated, "会话列表", "action.system.auth_session_list", &auditOff, "GET", "/api/v1/system/auth/sessions")},
		{ID: "system.auth_session_revoke", Handler: action.JSON(func(ctx context.Context, req RevokeSessionRequest) (any, error) {
			return service.RevokeSession(ctx, req.SessionID)
		}), Meta: authMeta(action.AccessAuthenticated, "撤销会话", "action.system.auth_session_revoke", &auditOn, "POST", "/api/v1/system/auth/sessions/revoke")},
		{ID: "system.auth_session_revoke_all", Handler: func(ctx context.Context, _ json.RawMessage) (any, error) { return service.RevokeAll(ctx) }, Meta: authMeta(action.AccessAuthenticated, "撤销全部会话", "action.system.auth_session_revoke_all", &auditOn, "POST", "/api/v1/system/auth/sessions/revoke-all")},
	}
	for _, item := range actions {
		if err := a.RegisterAction(item); err != nil {
			return err
		}
	}
	return nil
}

func authMeta(access action.AccessMode, name, key string, audit *bool, method, path string) action.Metadata {
	reason := action.AIExcludePendingSafetyReview
	if access == action.AccessPublic {
		reason = action.AIExcludePublicAuthentication
	}
	return action.Metadata{Capability: "system", Access: access, Name: name, NameKey: key + ".name", Description: name, DescriptionKey: key + ".description", Audit: audit, AI: action.ExcludeFromAI(reason), HTTP: &action.HTTPMeta{Method: method, Path: path}}
}
