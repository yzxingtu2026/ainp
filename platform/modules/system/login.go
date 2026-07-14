package system

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/logging"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"github.com/yzxingtu2026/ainp/platform/modules/system/model"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *AuthService) Login(ctx context.Context, tokens *auth.TokenService, input json.RawMessage) (any, error) {
	var req loginRequest
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, errs.ValidationKey("common.validation_failed", "invalid json")
	}
	if req.Username == "" || req.Password == "" {
		return nil, errs.ValidationKey("common.validation_failed", "username and password required",
			errs.FieldError{Field: "username", Message: "required", MessageKey: "common.field_required"},
			errs.FieldError{Field: "password", Message: "required", MessageKey: "common.field_required"},
		)
	}

	t, err := s.FindDefaultTenant(ctx)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errs.NewKey(errs.CodeInvalidCredentials, "auth.invalid_credentials", "invalid username or password")
		}
		return nil, errs.Wrap(errs.CodeInternal, "resolve default tenant failed", err)
	}
	u, err := s.FindUserByUsername(ctx, t.ID, req.Username)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			logging.Warn(ctx, "login failed: user not found", "username", req.Username)
			return nil, errs.NewKey(errs.CodeInvalidCredentials, "auth.invalid_credentials", "invalid username or password")
		}
		return nil, errs.Wrap(errs.CodeInternal, "lookup user failed", err)
	}

	if u.Status != model.StatusEnabled {
		logging.Warn(ctx, "login failed: user disabled", "user_id", u.ID)
		return nil, errs.NewKey(errs.CodeUserDisabled, "auth.user_disabled", "user disabled")
	}

	t, err = s.FindTenant(ctx, u.TenantID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errs.NewKey(errs.CodeInvalidCredentials, "auth.invalid_credentials", "invalid username or password")
		}
		return nil, errs.Wrap(errs.CodeInternal, "lookup tenant failed", err)
	}
	if t.Status != tenant.StatusEnabled {
		logging.Warn(ctx, "login failed: tenant disabled", "tenant_id", t.ID)
		return nil, errs.NewKey(errs.CodeTenantDisabled, "auth.tenant_disabled", "tenant disabled")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.Password)); err != nil {
		logging.Warn(ctx, "login failed: bad password", "user_id", u.ID, "tenant_id", u.TenantID)
		return nil, errs.NewKey(errs.CodeInvalidCredentials, "auth.invalid_credentials", "invalid username or password")
	}

	profile, err := s.BuildProfile(ctx, u)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "load permissions failed", err)
	}

	token, exp, err := tokens.Issue(u.TenantID, u.ID, profile.Roles, profile.Permissions)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "issue token failed", err)
	}

	// 写入上下文，供执行链审计落 tenant_id / user_id（登录为 Public Action）
	if ac := auth.MustFromContext(ctx); ac != nil {
		ac.TenantID = u.TenantID
		ac.Realm = auth.RealmTenant
		ac.UserID = u.ID
		ac.Roles = append([]string(nil), profile.Roles...)
		ac.Permissions = append([]string(nil), profile.Permissions...)
	}

	logging.Info(ctx, "login success", "user_id", u.ID, "tenant_id", u.TenantID)

	return map[string]any{
		"access_token": token,
		"token_type":   "Bearer",
		"expires_in":   int(time.Until(exp).Seconds()),
		"expires_at":   exp.UTC().Format(time.RFC3339),
		"user":         profile,
	}, nil
}

func (s *AuthService) Me(ctx context.Context) (any, error) {
	ac := auth.MustFromContext(ctx)
	if ac == nil || !ac.Authenticated || ac.UserID == "" {
		return nil, errs.NewKey(errs.CodeUnauthorized, "common.unauthorized", "unauthorized")
	}

	var u model.User
	if err := s.db.WithContext(ctx).Where("id = ? AND tenant_id = ?", ac.UserID, ac.TenantID).First(&u).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errs.NewKey(errs.CodeNotFound, "common.not_found", "user not found")
		}
		return nil, errs.Wrap(errs.CodeInternal, "lookup user failed", err)
	}
	if u.Status != model.StatusEnabled {
		return nil, errs.NewKey(errs.CodeUserDisabled, "auth.user_disabled", "user disabled")
	}

	profile, err := s.BuildProfile(ctx, &u)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "load profile failed", err)
	}
	return profile, nil
}

func (s *AuthService) Permissions(ctx context.Context) (any, error) {
	ac := auth.MustFromContext(ctx)
	if ac == nil || !ac.Authenticated || ac.UserID == "" {
		return nil, errs.NewKey(errs.CodeUnauthorized, "common.unauthorized", "unauthorized")
	}
	roles, perms, err := s.LoadRolesAndPermissions(ctx, ac.UserID, ac.TenantID)
	if err != nil {
		return nil, errs.Wrap(errs.CodeInternal, "load permissions failed", err)
	}
	return map[string]any{
		"roles":       roles,
		"permissions": perms,
	}, nil
}
