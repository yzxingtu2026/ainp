package operations

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/audit"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/i18n"
	"github.com/yzxingtu2026/ainp/platform/framework/pagination"
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
	StartAt      string `json:"start_at"`
	EndAt        string `json:"end_at"`
	UserKeyword  string `json:"user_keyword"`
	ActorType    string `json:"actor_type"`
	ActionID     string `json:"action_id"`
	Result       string `json:"result" validate:"omitempty,oneof=success failure"`
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
	RequestID    string `json:"request_id"`
	Client       string `json:"client"`
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
	ac, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	end, start, err := auditRange(req.StartAt, req.EndAt)
	if err != nil {
		return nil, err
	}
	req.Normalize()

	q := s.db.WithContext(ctx).Table("audit_logs AS a").
		Select("a.*, COALESCE(u.display_name, u.username, '') AS actor_name").
		Joins("LEFT JOIN users AS u ON u.id = a.user_id AND u.tenant_id = a.tenant_id").
		Where("a.tenant_id = ? AND a.occurred_at >= ? AND a.occurred_at <= ?", ac.TenantID, start, end)
	if keyword := strings.TrimSpace(req.UserKeyword); keyword != "" {
		like := "%" + strings.ToLower(keyword) + "%"
		q = q.Where("LOWER(u.username) LIKE ? OR LOWER(u.display_name) LIKE ?", like, like)
	}
	filters := map[string]string{"a.actor_type": req.ActorType, "a.action_id": req.ActionID, "a.result": req.Result, "a.resource_type": req.ResourceType, "a.resource_id": req.ResourceID, "a.request_id": req.RequestID, "a.client": req.Client}
	for column, value := range filters {
		if value != "" {
			q = q.Where(column+" = ?", value)
		}
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, err
	}
	order := "DESC"
	if req.Sort == "asc" {
		order = "ASC"
	}
	var rows []AuditLogItem
	if err := q.Order("a.occurred_at " + order).Offset(req.Offset()).Limit(req.PageSize).Scan(&rows).Error; err != nil {
		return nil, err
	}
	for i := range rows {
		rows[i].ActionName = s.actionName(ctx, rows[i].ActionID)
	}
	return pagination.NewResult(rows, total, req.Request), nil
}

func (s *AuditLogService) get(ctx context.Context, req auditLogGetRequest) (any, error) {
	ac, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	var row AuditLogItem
	err = s.db.WithContext(ctx).Table("audit_logs AS a").
		Select("a.*, COALESCE(u.display_name, u.username, '') AS actor_name").
		Joins("LEFT JOIN users AS u ON u.id = a.user_id AND u.tenant_id = a.tenant_id").
		Where("a.tenant_id = ? AND a.id = ?", ac.TenantID, req.ID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errs.NewKey(errs.CodeNotFound, "audit.not_found", "audit log not found")
	}
	if err != nil {
		return nil, err
	}
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
	}{
		{ActionAuditLogList, "/api/v1/operations/audit-logs/list", "审计列表", "action.system.audit_log_list.name", "查询当前租户操作审计", "action.system.audit_log_list.description", action.JSON(svc.list)},
		{ActionAuditLogGet, "/api/v1/operations/audit-logs/get", "审计详情", "action.system.audit_log_get.name", "查询当前租户操作审计详情", "action.system.audit_log_get.description", action.JSON(svc.get)},
	} {
		if err := a.RegisterAction(action.Action{ID: spec.id, Handler: spec.handler, Meta: action.Metadata{Capability: "operations", Name: spec.name, NameKey: spec.nameKey, Description: spec.desc, DescriptionKey: spec.descKey, PermissionGroup: "system.operations.audit", PermissionGroupName: "操作审计", PermissionGroupNameKey: "permission_group.system.operations.audit", Audit: &auditOn, HTTP: &action.HTTPMeta{Method: "POST", Path: spec.path}}}); err != nil {
			return err
		}
	}
	return nil
}
