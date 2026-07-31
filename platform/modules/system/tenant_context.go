package system

import (
	"context"
	"strings"

	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
)

func requireTenant(ctx context.Context) (*auth.Context, error) {
	ac := auth.MustFromContext(ctx)
	if !ac.Authenticated || strings.TrimSpace(ac.TenantID) == "" {
		return nil, errs.NewKey(errs.CodeUnauthorized, "common.unauthorized", "unauthorized")
	}
	return ac, nil
}

func tenantIdentity(ctx context.Context) (string, string, error) {
	ac := auth.MustFromContext(ctx)
	if !ac.Authenticated || ac.Realm != auth.RealmTenant || ac.TenantID == "" || ac.UserID == "" {
		return "", "", errs.NewKey(errs.CodeUnauthorized, "common.unauthorized", "unauthorized")
	}
	return ac.TenantID, ac.UserID, nil
}
