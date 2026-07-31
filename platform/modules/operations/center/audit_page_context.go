package center

import (
	"context"
	"fmt"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/audit"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/pagecontext"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"gorm.io/gorm"
)

const PageTypeAuditLogs = "operations.audit-logs"

type auditPageContextResolver struct{ db *gorm.DB }

func (r auditPageContextResolver) Resolve(ctx context.Context, envelope pagecontext.Envelope) (pagecontext.Snapshot, error) {
	ac := auth.MustFromContext(ctx)
	permission := ActionAuditLogList
	if envelope.Mode == "detail" {
		permission = ActionAuditLogGet
	}
	if !ac.Authenticated || !ac.HasPermission(permission) || (envelope.Mode != "list" && envelope.Mode != "detail") {
		return pagecontext.Snapshot{}, fmt.Errorf("page context unavailable")
	}
	snapshot := pagecontext.Snapshot{
		SafeSummary: "Audit log " + envelope.Mode + " page",
		ToolHints:   pagecontext.ToolHints{Capabilities: []string{"operations"}, Tags: []string{"audit", "audit-log"}, Queries: []string{"查询审计日志", "审计详情"}},
	}
	if envelope.Mode == "list" {
		if len(envelope.ResourceRefs) != 0 {
			return pagecontext.Snapshot{}, fmt.Errorf("page context unavailable")
		}
		return snapshot, nil
	}
	if len(envelope.ResourceRefs) != 1 || envelope.ResourceRefs[0].ResourceType != "operations.audit-log" {
		return pagecontext.Snapshot{}, fmt.Errorf("page context unavailable")
	}
	tdb, err := tenant.TenantDB(ctx, r.db)
	if err != nil {
		return pagecontext.Snapshot{}, err
	}
	rows, err := tenant.TenantRaw[audit.Record](tdb, "SELECT * FROM audit_logs WHERE tenant_id = ? AND id = ? LIMIT 1", envelope.ResourceRefs[0].ResourceID)
	if err != nil || len(rows) != 1 {
		return pagecontext.Snapshot{}, fmt.Errorf("page context unavailable")
	}
	record := rows[0]
	revision := record.OccurredAt.UTC().Format(time.RFC3339Nano)
	if supplied := envelope.ResourceRefs[0].Revision; supplied != "" {
		parsed, parseErr := time.Parse(time.RFC3339Nano, supplied)
		if parseErr != nil || !parsed.Equal(record.OccurredAt) {
			return pagecontext.Snapshot{}, fmt.Errorf("page context unavailable")
		}
	}
	snapshot.TrustedRefs = []pagecontext.ResourceRef{{ResourceType: "operations.audit-log", ResourceID: record.ID, Revision: revision}}
	snapshot.SafeSummary = fmt.Sprintf("Audit record for action %s with result %s at %s", record.ActionID, record.Result, revision)
	return snapshot, nil
}
