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

const PageTypeSystemUsers = "system.users"

type UserPageContextResolver struct{ DB *gorm.DB }

func (r UserPageContextResolver) Resolve(ctx context.Context, envelope pagecontext.Envelope) (pagecontext.Snapshot, error) {
	ac := auth.MustFromContext(ctx)
	permission := ActionUserList
	if envelope.Mode == "detail" || envelope.Mode == "edit" {
		permission = ActionUserGet
	}
	if envelope.Mode == "create" {
		permission = ActionUserCreate
	}
	if !ac.Authenticated || !ac.HasPermission(permission) {
		return pagecontext.Snapshot{}, fmt.Errorf("page context unavailable")
	}
	snapshot := pagecontext.Snapshot{
		SafeSummary: "System user management " + envelope.Mode + " page",
		ToolHints:   pagecontext.ToolHints{Capabilities: []string{"system"}, Tags: []string{"user", "user-management"}, Queries: []string{"系统用户管理", "查询用户列表"}},
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
	if len(envelope.ResourceRefs) != 1 || envelope.ResourceRefs[0].ResourceType != "system.user" {
		return pagecontext.Snapshot{}, fmt.Errorf("page context unavailable")
	}
	tdb, err := tenant.TenantDB(ctx, r.DB)
	if err != nil {
		return pagecontext.Snapshot{}, err
	}
	users, err := tenant.TenantRaw[model.User](tdb, "SELECT * FROM users WHERE tenant_id = ? AND id = ? LIMIT 1", envelope.ResourceRefs[0].ResourceID)
	if err != nil || len(users) != 1 {
		return pagecontext.Snapshot{}, fmt.Errorf("page context unavailable")
	}
	user := users[0]
	revision := user.UpdatedAt.UTC().Format(time.RFC3339Nano)
	if supplied := envelope.ResourceRefs[0].Revision; supplied != "" {
		parsed, parseErr := time.Parse(time.RFC3339Nano, supplied)
		if parseErr != nil || !parsed.Equal(user.UpdatedAt) {
			return pagecontext.Snapshot{}, fmt.Errorf("page context unavailable")
		}
	}
	snapshot.TrustedRefs = []pagecontext.ResourceRef{{ResourceType: "system.user", ResourceID: user.ID, Revision: revision}}
	snapshot.SafeSummary = fmt.Sprintf("System user %q (%s), status %s; %s page", user.DisplayName, user.Username, user.Status, envelope.Mode)
	snapshot.ToolHints.Queries = []string{"查询系统用户详情", "管理用户"}
	return snapshot, nil
}
