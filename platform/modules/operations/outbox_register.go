package operations

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/outbox"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"gorm.io/gorm"
)

const (
	ActionOutboxList           = "operations.outbox_list"
	ActionOutboxSummary        = "operations.outbox_summary"
	ActionOutboxListDeliveries = "operations.outbox_list_deliveries"
	ActionOutboxRetry          = "operations.outbox_retry"
)

type outboxEventIDRequest struct {
	EventID string `json:"event_id" validate:"required"`
}

func (s *OutboxService) List(ctx context.Context, input json.RawMessage) (any, error) {
	if _, err := requireTenant(ctx); err != nil {
		return nil, err
	}
	var req struct {
		EventType string `json:"event_type"`
		Status    string `json:"status"`
		Current   int    `json:"current"`
		Size      int    `json:"size"`
	}
	_ = json.Unmarshal(input, &req)
	if req.Current < 1 {
		req.Current = 1
	}
	if req.Size < 1 || req.Size > 100 {
		req.Size = 20
	}
	db := s.db.WithContext(ctx).Scopes(tenant.ScopeByTenant(ctx)).Model(&outbox.Event{})
	if value := strings.TrimSpace(req.EventType); value != "" {
		db = db.Where("event_type = ?", value)
	}
	if value := strings.TrimSpace(req.Status); value != "" {
		db = db.Where("status = ?", value)
	}
	var total int64
	if err := db.Count(&total).Error; err != nil {
		return nil, err
	}
	var rows []outbox.Event
	if err := db.Order("occurred_at DESC").Offset((req.Current - 1) * req.Size).Limit(req.Size).Find(&rows).Error; err != nil {
		return nil, err
	}
	return map[string]any{"items": rows, "total": total, "current": req.Current, "size": req.Size}, nil
}

type OutboxService struct {
	db *gorm.DB
}

func NewOutboxService(db *gorm.DB) *OutboxService {
	return &OutboxService{db: db}
}

func (s *OutboxService) Summary(ctx context.Context, _ json.RawMessage) (any, error) {
	if _, err := requireTenant(ctx); err != nil {
		return nil, err
	}
	type row struct {
		Status status.OutboxStatus
		Count  int64
	}
	var rows []row
	q := s.db.WithContext(ctx).Scopes(tenant.ScopeByTenant(ctx)).Model(&outbox.Event{}).
		Select("status, count(*) as count").Group("status")
	if err := q.Scan(&rows).Error; err != nil {
		return nil, err
	}
	counts := map[string]int64{
		string(status.OutboxPending):    0,
		string(status.OutboxProcessing): 0,
		string(status.OutboxPublished):  0,
		string(status.OutboxFailed):     0,
		string(status.OutboxDead):       0,
	}
	for _, r := range rows {
		counts[string(r.Status)] = r.Count
	}
	var oldest *time.Time
	var pending outbox.Event
	err := s.db.WithContext(ctx).Scopes(tenant.ScopeByTenant(ctx)).Model(&outbox.Event{}).
		Where("status IN ?", []status.OutboxStatus{status.OutboxPending, status.OutboxFailed, status.OutboxProcessing}).
		Order("occurred_at ASC").
		First(&pending).Error
	if err == nil {
		t := pending.OccurredAt
		oldest = &t
	} else if err != gorm.ErrRecordNotFound {
		return nil, err
	}
	return map[string]any{
		"counts":            counts,
		"oldest_pending_at": oldest,
	}, nil
}

func (s *OutboxService) listDeliveries(ctx context.Context, req outboxEventIDRequest) (any, error) {
	if _, err := requireTenant(ctx); err != nil {
		return nil, err
	}
	var ev outbox.Event
	if err := s.db.WithContext(ctx).Scopes(tenant.ScopeByTenant(ctx)).
		Where("id = ?", req.EventID).First(&ev).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, errs.NewKey(errs.CodeNotFound, "outbox.event_not_found", "outbox event not found")
		}
		return nil, err
	}
	var deliveries []outbox.Delivery
	if err := s.db.WithContext(ctx).Where("event_id = ?", ev.ID).Order("handler_name").Find(&deliveries).Error; err != nil {
		return nil, err
	}
	return map[string]any{
		"event": map[string]any{
			"id":             ev.ID,
			"event_type":     ev.EventType,
			"status":         ev.Status,
			"attempts":       ev.Attempts,
			"last_error":     ev.LastError,
			"occurred_at":    ev.OccurredAt,
			"aggregate_type": ev.AggregateType,
			"aggregate_id":   ev.AggregateID,
			"payload":        json.RawMessage(ev.PayloadJSON),
			"metadata":       json.RawMessage(ev.MetadataJSON),
			"available_at":   ev.AvailableAt,
			"published_at":   ev.PublishedAt,
		},
		"deliveries": deliveries,
	}, nil
}

func (s *OutboxService) retry(ctx context.Context, req outboxEventIDRequest) (any, error) {
	ac, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	var ev outbox.Event
	if err := s.db.WithContext(ctx).Scopes(tenant.ScopeByTenant(ctx)).
		Where("id = ?", req.EventID).First(&ev).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, errs.NewKey(errs.CodeNotFound, "outbox.event_not_found", "outbox event not found")
		}
		return nil, err
	}
	if err := outbox.RetryEvent(ctx, s.db, ev.ID); err != nil {
		return nil, errs.NewKey(errs.CodeConflict, "outbox.retry_not_allowed", err.Error())
	}
	ac.AuditResourceType = "outbox_event"
	ac.AuditResourceID = ev.ID
	ac.AuditSummary = "retry failed/dead outbox event"
	return map[string]any{"id": ev.ID, "status": status.OutboxPending}, nil
}

func requireTenant(ctx context.Context) (*auth.Context, error) {
	ac := auth.MustFromContext(ctx)
	if !ac.Authenticated || strings.TrimSpace(ac.TenantID) == "" {
		return nil, errs.NewKey(errs.CodeUnauthorized, "common.unauthorized", "unauthorized")
	}
	return ac, nil
}

func registerOutboxActions(a *app.App) error {
	svc := NewOutboxService(a.DB)
	auditOn := true
	auditOff := false
	specs := []struct {
		id, path, name, nameKey, desc, descKey string
		audit                                  *bool
		handler                                func(context.Context, json.RawMessage) (any, error)
	}{
		{ActionOutboxList, "/api/v1/operations/outbox/list", "Outbox 事件列表", "action.operations.outbox_list.name", "分页查询租户 Outbox 事件", "action.operations.outbox_list.description", &auditOff, svc.List},
		{ActionOutboxSummary, "/api/v1/operations/outbox/summary", "Outbox 摘要", "action.operations.outbox_summary.name", "查询租户 Outbox 状态计数", "action.operations.outbox_summary.description", &auditOff, svc.Summary},
		{ActionOutboxListDeliveries, "/api/v1/operations/outbox/deliveries", "Outbox 投递记录", "action.operations.outbox_list_deliveries.name", "查看事件 Handler 投递状态", "action.operations.outbox_list_deliveries.description", &auditOff, action.JSON(svc.listDeliveries)},
		{ActionOutboxRetry, "/api/v1/operations/outbox/retry", "重试 Outbox 事件", "action.operations.outbox_retry.name", "将 failed/dead 事件重置为 pending（不可改 payload）", "action.operations.outbox_retry.description", &auditOn, action.JSON(svc.retry)},
	}
	for _, spec := range specs {
		if err := a.RegisterAction(action.Action{
			ID:      spec.id,
			Handler: spec.handler,
			Meta: action.Metadata{
				Capability:             "operations",
				Name:                   spec.name,
				NameKey:                spec.nameKey,
				Description:            spec.desc,
				DescriptionKey:         spec.descKey,
				PermissionGroup:        "operations.outbox",
				PermissionGroupName:    "Outbox 运维",
				PermissionGroupNameKey: "permission_group.operations.outbox",
				Audit:                  spec.audit,
				HTTP: &action.HTTPMeta{
					Method: "POST",
					Path:   spec.path,
				},
			},
		}); err != nil {
			return err
		}
	}
	return nil
}
