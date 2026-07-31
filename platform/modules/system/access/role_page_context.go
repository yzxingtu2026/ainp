package access

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/pagecontext"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"github.com/yzxingtu2026/ainp/platform/modules/system/access/model"
	"gorm.io/gorm"
)

const PageTypeSystemRoles = "system.roles"

type RolePageContextResolver struct{ DB *gorm.DB }

func (r RolePageContextResolver) Resolve(ctx context.Context, envelope pagecontext.Envelope) (pagecontext.Snapshot, error) {
	ac := auth.MustFromContext(ctx)
	permission := ActionRoleList
	if envelope.Mode == "detail" || envelope.Mode == "edit" {
		permission = ActionRoleGet
	}
	if envelope.Mode == "create" {
		permission = ActionRoleCreate
	}
	if !ac.Authenticated || !ac.HasPermission(permission) {
		return pagecontext.Snapshot{}, fmt.Errorf("page context unavailable")
	}
	snapshot := pagecontext.Snapshot{
		SafeSummary: "System role and permission management " + envelope.Mode + " page",
		ToolHints: pagecontext.ToolHints{
			Capabilities: []string{"system"},
			Tags:         []string{"role", "permission-management"},
			Queries:      []string{"系统角色管理", "查询角色与权限"},
		},
	}
	if envelope.Mode == "list" || envelope.Mode == "create" {
		if len(envelope.ResourceRefs) != 0 {
			return pagecontext.Snapshot{}, fmt.Errorf("page context unavailable")
		}
		if envelope.Mode == "list" {
			filters := make([]string, 0, 2)
			for _, key := range []string{"keyword", "status"} {
				if value, ok := envelope.UIState[key].(string); ok && strings.TrimSpace(value) != "" {
					filters = append(filters, key+"="+strings.TrimSpace(value))
				}
			}
			if len(filters) > 0 {
				snapshot.SafeSummary += "; active filters: " + strings.Join(filters, ", ")
			}
		}
		return snapshot, nil
	}
	if len(envelope.ResourceRefs) != 1 || envelope.ResourceRefs[0].ResourceType != "system.role" {
		return pagecontext.Snapshot{}, fmt.Errorf("page context unavailable")
	}
	tdb, err := tenant.TenantDB(ctx, r.DB)
	if err != nil {
		return pagecontext.Snapshot{}, err
	}
	roles, err := tenant.TenantRaw[model.Role](tdb, "SELECT * FROM roles WHERE tenant_id = ? AND id = ? LIMIT 1", envelope.ResourceRefs[0].ResourceID)
	if err != nil || len(roles) != 1 {
		return pagecontext.Snapshot{}, fmt.Errorf("page context unavailable")
	}
	role := roles[0]
	revision := role.UpdatedAt.UTC().Format(time.RFC3339Nano)
	if supplied := envelope.ResourceRefs[0].Revision; supplied != "" {
		parsed, parseErr := time.Parse(time.RFC3339Nano, supplied)
		if parseErr != nil || !parsed.Equal(role.UpdatedAt) {
			return pagecontext.Snapshot{}, fmt.Errorf("page context unavailable")
		}
	}
	snapshot.TrustedRefs = []pagecontext.ResourceRef{{ResourceType: "system.role", ResourceID: role.ID, Revision: revision}}
	snapshot.SafeSummary = fmt.Sprintf("System role %q (%s), status %s; %s page", role.Name, role.Code, role.Status, envelope.Mode)
	snapshot.ToolHints.Queries = []string{"查询系统角色详情", "管理角色权限"}
	return snapshot, nil
}
