package system

import (
	"context"
	"errors"

	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
	"github.com/yzxingtu2026/ainp/platform/modules/system/model"
	"gorm.io/gorm"
)

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
	if u.Status != status.ActivationEnabled {
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
	return map[string]any{
		"roles":       append([]string(nil), ac.Roles...),
		"permissions": append([]string(nil), ac.Permissions...),
	}, nil
}
