package center

import (
	"context"
	"strings"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/audit"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/i18n"
	"github.com/yzxingtu2026/ainp/platform/framework/pagination"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"gorm.io/gorm"
)

const (
	ActionAuditLogList = "system.audit_log_list"
	ActionAuditLogGet  = "system.audit_log_get"

	auditDefaultRange = 24 * time.Hour
	auditMaxRange     = 31 * 24 * time.Hour
)

type auditLogListRequest struct {
	pagination.Request
	StartAt      string `json:"start_at" validate:"omitempty"`
	EndAt        string `json:"end_at" validate:"omitempty"`
	UserKeyword  string `json:"user_keyword" validate:"omitempty"`
	ActorType    string `json:"actor_type" validate:"omitempty"`
	ActionID     string `json:"action_id" validate:"omitempty"`
	Result       string `json:"result" validate:"omitempty,oneof=success failure"`
	ResourceType string `json:"resource_type" validate:"omitempty"`
	ResourceID   string `json:"resource_id" validate:"omitempty"`
	RequestID    string `json:"request_id" validate:"omitempty"`
	Client       string `json:"client" validate:"omitempty"`
	Sort         string `json:"sort" validate:"omitempty,oneof=asc desc"`
}

type auditLogGetRequest struct {
	ID string `json:"id" validate:"required"`
}

type AuditLogItem struct {
	audit.Record
	ActorName  string `json:"actor_name" gorm:"column:actor_name"`
	ActionName string `json:"action_name" gorm:"-"`
}

type AuditLogService struct {
	db      *gorm.DB
	actions *action.Registry
}

func NewAuditLogService(db *gorm.DB, actions ...*action.Registry) *AuditLogService {
	svc := &AuditLogService{db: db}
	if len(actions) > 0 {
		svc.actions = actions[0]
	}
	return svc
}

func (s *AuditLogService) list(ctx context.Context, req auditLogListRequest) (any, error) {
	_, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	end, start, err := auditRange(req.StartAt, req.EndAt)
	if err != nil {
		return nil, err
	}
	req.Normalize()

	tdb, err := tenant.TenantDB(ctx, s.db)
	if err != nil {
		return nil, err
	}
	fromSQL := " FROM audit_logs AS a LEFT JOIN tenant_accounts AS u ON u.id = a.user_id AND u.tenant_id = a.tenant_id" +
		" WHERE a.tenant_id = ? AND a.occurred_at >= ? AND a.occurred_at <= ?"
	args := []any{start, end}
	if keyword := strings.TrimSpace(req.UserKeyword); keyword != "" {
		like := "%" + strings.ToLower(keyword) + "%"
		fromSQL += " AND (LOWER(u.username) LIKE ? OR LOWER(u.display_name) LIKE ?)"
		args = append(args, like, like)
	}
	filters := map[string]string{"a.actor_type": req.ActorType, "a.action_id": req.ActionID, "a.result": req.Result, "a.resource_type": req.ResourceType, "a.resource_id": req.ResourceID, "a.request_id": req.RequestID, "a.client": req.Client}
	for column, value := range filters {
		if value != "" {
			fromSQL += " AND " + column + " = ?"
			args = append(args, value)
		}
	}
	counts, err := tenant.TenantRaw[struct{ Count int64 }](tdb, "SELECT COUNT(*) AS count"+fromSQL, args...)
	if err != nil {
		return nil, err
	}
	total := int64(0)
	if len(counts) > 0 {
		total = counts[0].Count
	}
	order := "DESC"
	if req.Sort == "asc" {
		order = "ASC"
	}
	listArgs := append(append([]any{}, args...), req.PageSize, req.Offset())
	rows, err := tenant.TenantRaw[AuditLogItem](tdb,
		"SELECT a.*, COALESCE(u.display_name, u.username, '') AS actor_name"+fromSQL+
			" ORDER BY a.occurred_at "+order+" LIMIT ? OFFSET ?", listArgs...)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		rows[i].ActionName = s.actionName(ctx, rows[i].ActionID)
	}
	return pagination.NewResult(rows, total, req.Request), nil
}

func (s *AuditLogService) get(ctx context.Context, req auditLogGetRequest) (any, error) {
	_, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	tdb, err := tenant.TenantDB(ctx, s.db)
	if err != nil {
		return nil, err
	}
	rows, err := tenant.TenantRaw[AuditLogItem](tdb,
		"SELECT a.*, COALESCE(u.display_name, u.username, '') AS actor_name FROM audit_logs AS a"+
			" LEFT JOIN tenant_accounts AS u ON u.id = a.user_id AND u.tenant_id = a.tenant_id"+
			" WHERE a.tenant_id = ? AND a.id = ? LIMIT 1", req.ID)
	if len(rows) == 0 && err == nil {
		return nil, errs.NewKey(errs.CodeNotFound, "audit.not_found", "audit log not found")
	}
	if err != nil {
		return nil, err
	}
	row := rows[0]
	row.ActionName = s.actionName(ctx, row.ActionID)
	return map[string]any{"audit_log": row}, nil
}

func (s *AuditLogService) actionName(ctx context.Context, id string) string {
	if s.actions == nil {
		return id
	}
	registered, ok := s.actions.Get(id)
	if !ok {
		return id
	}
	name := registered.Meta.Name
	if registered.Meta.NameKey != "" {
		lang := i18n.DefaultLang()
		if ac, ok := auth.FromContext(ctx); ok && ac.Language != "" {
			lang = ac.Language
		}
		if translated := i18n.T(lang, registered.Meta.NameKey); translated != registered.Meta.NameKey {
			name = translated
		}
	}
	if name == "" {
		return id
	}
	return name
}

func auditRange(startRaw, endRaw string) (time.Time, time.Time, error) {
	end := time.Now().UTC()
	var err error
	if endRaw != "" {
		end, err = time.Parse(time.RFC3339, endRaw)
		if err != nil {
			return time.Time{}, time.Time{}, errs.NewKey(errs.CodeValidation, "audit.invalid_time", "invalid end_at")
		}
	}
	start := end.Add(-auditDefaultRange)
	if startRaw != "" {
		start, err = time.Parse(time.RFC3339, startRaw)
		if err != nil {
			return time.Time{}, time.Time{}, errs.NewKey(errs.CodeValidation, "audit.invalid_time", "invalid start_at")
		}
	}
	if !start.Before(end) || end.Sub(start) > auditMaxRange {
		return time.Time{}, time.Time{}, errs.NewKey(errs.CodeValidation, "audit.invalid_range", "audit range must be positive and at most 31 days")
	}
	return end.UTC(), start.UTC(), nil
}

func registerAuditLogActions(a *app.App) error {
	svc := NewAuditLogService(a.DB, a.Actions)
	auditOn := true
	for _, spec := range []struct {
		id, path, name, nameKey, desc, descKey string
		handler                                action.Handler
		ai                                     *action.AIMeta
	}{
		{ActionAuditLogList, "/api/v1/operations/audit-logs/list", "审计列表", "action.system.audit_log_list.name", "查询当前租户操作审计", "action.system.audit_log_list.description", action.JSON(svc.list), readAIMeta("查询当前租户操作审计，可按时间、用户、操作、结果和资源筛选", "查询操作审计列表", "查询当前租户操作审计，可按时间、用户、操作、结果和资源筛选", "已返回当前租户可见的操作审计记录", action.InputSchemaFor[auditLogListRequest](), action.SchemaFor[pagination.Result[AuditLogItem]](), "运维", "审计", "操作记录")},
		{ActionAuditLogGet, "/api/v1/operations/audit-logs/get", "审计详情", "action.system.audit_log_get.name", "查询当前租户操作审计详情", "action.system.audit_log_get.description", action.JSON(svc.get), readAIMeta("按审计记录 ID 查询当前租户操作审计详情", "查询操作审计详情", "按审计记录 ID 查询当前租户操作审计详情", "已返回指定操作审计详情", action.InputSchemaFor[auditLogGetRequest](), action.SchemaFor[struct {
			AuditLog AuditLogItem `json:"audit_log"`
		}](), "运维", "审计", "详情")},
	} {
		if err := a.RegisterAction(action.Action{ID: spec.id, Handler: spec.handler, Meta: action.Metadata{Capability: "operations", Name: spec.name, NameKey: spec.nameKey, Description: spec.desc, DescriptionKey: spec.descKey, PermissionGroup: "system.operations.audit", PermissionGroupName: "操作审计", PermissionGroupNameKey: "permission_group.system.operations.audit", Audit: &auditOn, AI: spec.ai, HTTP: &action.HTTPMeta{Method: "POST", Path: spec.path}}}); err != nil {
			return err
		}
	}
	return nil
}
