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

const PageTypeSystemAccounts = "system.accounts"

type AccountPageContextResolver struct{ DB *gorm.DB }

func (r AccountPageContextResolver) Resolve(ctx context.Context, envelope pagecontext.Envelope) (pagecontext.Snapshot, error) {
	ac := auth.MustFromContext(ctx)
	permission := ActionAccountList
	if envelope.Mode == "detail" || envelope.Mode == "edit" {
		permission = ActionAccountGet
	}
	if envelope.Mode == "create" {
		permission = ActionAccountCreate
	}
	if !ac.Authenticated || !ac.HasPermission(permission) {
		return pagecontext.Snapshot{}, fmt.Errorf("page context unavailable")
	}
	snapshot := pagecontext.Snapshot{
		SafeSummary: "System account management " + envelope.Mode + " page",
		ToolHints:   pagecontext.ToolHints{Capabilities: []string{"system"}, Tags: []string{"account", "account-management"}, Queries: []string{"系统账号管理", "查询账号列表"}},
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
	if len(envelope.ResourceRefs) != 1 || (envelope.ResourceRefs[0].ResourceType != "system.account" && envelope.ResourceRefs[0].ResourceType != "system.user") {
		return pagecontext.Snapshot{}, fmt.Errorf("page context unavailable")
	}
	tdb, err := tenant.TenantDB(ctx, r.DB)
	if err != nil {
		return pagecontext.Snapshot{}, err
	}
	accounts, err := tenant.TenantRaw[model.TenantAccount](tdb, "SELECT * FROM tenant_accounts WHERE tenant_id = ? AND id = ? LIMIT 1", envelope.ResourceRefs[0].ResourceID)
	if err != nil || len(accounts) != 1 {
		return pagecontext.Snapshot{}, fmt.Errorf("page context unavailable")
	}
	account := accounts[0]
	revision := account.UpdatedAt.UTC().Format(time.RFC3339Nano)
	if supplied := envelope.ResourceRefs[0].Revision; supplied != "" {
		parsed, parseErr := time.Parse(time.RFC3339Nano, supplied)
		if parseErr != nil || !parsed.Equal(account.UpdatedAt) {
			return pagecontext.Snapshot{}, fmt.Errorf("page context unavailable")
		}
	}
	snapshot.TrustedRefs = []pagecontext.ResourceRef{{ResourceType: "system.account", ResourceID: account.ID, Revision: revision}}
	snapshot.SafeSummary = fmt.Sprintf("System account %q (%s), status %s; %s page", account.DisplayName, account.Username, account.Status, envelope.Mode)
	snapshot.ToolHints.Queries = []string{"查询系统账号详情", "管理账号"}
	return snapshot, nil
}
