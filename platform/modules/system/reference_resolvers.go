package system

import (
	"context"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
)

func registerReferenceResolvers(a *app.App) error {
	if err := a.Actions.RegisterReferenceResolver("system.assignable_permissions", action.ReferenceResolverFunc(func(context.Context) (action.ReferenceSnapshot, error) {
		return action.ReferenceSnapshot{Values: AssignablePermissions(a.Actions)}, nil
	})); err != nil {
		return err
	}
	return a.Actions.RegisterReferenceResolver("system.assignable_roles", action.ReferenceResolverFunc(func(ctx context.Context) (action.ReferenceSnapshot, error) {
		tdb, err := tenant.TenantDB(ctx, a.DB)
		if err != nil {
			return action.ReferenceSnapshot{}, err
		}
		rows, err := tenant.TenantRaw[struct{ ID string }](tdb, `SELECT id FROM roles WHERE tenant_id = ? AND status = ? ORDER BY id`, status.ActivationEnabled)
		if err != nil {
			return action.ReferenceSnapshot{}, err
		}
		values := make([]string, 0, len(rows))
		for _, row := range rows {
			values = append(values, row.ID)
		}
		return action.ReferenceSnapshot{Values: values}, nil
	}))
}
