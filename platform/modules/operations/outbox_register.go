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
	"github.com/yzxingtu2026/ainp/platform/framework/pagination"
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

type outboxListRequest struct {
	pagination.Request
	EventType string `json:"event_type" validate:"omitempty"`
	Status    string `json:"status" validate:"omitempty,oneof=pending processing published failed dead"`
}

type outboxSummaryResponse struct {
	Counts          map[string]int64 `json:"counts"`
	OldestPendingAt *time.Time       `json:"oldest_pending_at"`
}

type outboxEventDetail struct {
	ID            string              `json:"id"`
	EventType     string              `json:"event_type"`
	Status        status.OutboxStatus `json:"status"`
	Attempts      int                 `json:"attempts"`
	LastError     string              `json:"last_error"`
	OccurredAt    time.Time           `json:"occurred_at"`
	AggregateType string              `json:"aggregate_type"`
	AggregateID   string              `json:"aggregate_id"`
	Payload       json.RawMessage     `json:"payload"`
	Metadata      json.RawMessage     `json:"metadata"`
	AvailableAt   time.Time           `json:"available_at"`
	PublishedAt   *time.Time          `json:"published_at"`
}

type outboxDeliveriesResponse struct {
	Event      outboxEventDetail `json:"event"`
	Deliveries []outbox.Delivery `json:"deliveries"`
}

type outboxRetryResponse struct {
	ID     string              `json:"id"`
	Status status.OutboxStatus `json:"status"`
}

func (s *OutboxService) List(ctx context.Context, input json.RawMessage) (any, error) {
	return action.JSON(s.list)(ctx, input)
}

func (s *OutboxService) list(ctx context.Context, req outboxListRequest) (any, error) {
	if _, err := requireTenant(ctx); err != nil {
		return nil, err
	}
	tdb, err := tenant.TenantDB(ctx, s.db)
	if err != nil {
		return nil, err
	}
	whereSQL := " WHERE tenant_id = ?"
	args := make([]any, 0, 2)
	if value := strings.TrimSpace(req.EventType); value != "" {
		whereSQL += " AND event_type = ?"
		args = append(args, value)
	}
	if value := strings.TrimSpace(req.Status); value != "" {
		whereSQL += " AND status = ?"
		args = append(args, value)
	}
	counts, err := tenant.TenantRaw[struct{ Count int64 }](tdb, "SELECT COUNT(*) AS count FROM outbox_events"+whereSQL, args...)
	if err != nil {
		return nil, err
	}
	total := int64(0)
	if len(counts) > 0 {
		total = counts[0].Count
	}
	listArgs := append(append([]any{}, args...), req.PageSize, req.Offset())
	rows, err := tenant.TenantRaw[outbox.Event](tdb, "SELECT * FROM outbox_events"+whereSQL+" ORDER BY occurred_at DESC LIMIT ? OFFSET ?", listArgs...)
	if err != nil {
		return nil, err
	}
	return pagination.NewResult(rows, total, req.Request), nil
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
	tdb, err := tenant.TenantDB(ctx, s.db)
	if err != nil {
		return nil, err
	}
	type row struct {
		Status status.OutboxStatus
		Count  int64
	}
	rows, err := tenant.TenantRaw[row](tdb, "SELECT status, COUNT(*) AS count FROM outbox_events WHERE tenant_id = ? GROUP BY status")
	if err != nil {
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
	pending, err := tenant.TenantRaw[outbox.Event](tdb, "SELECT * FROM outbox_events WHERE tenant_id = ? AND status IN (?, ?, ?) ORDER BY occurred_at ASC LIMIT 1", status.OutboxPending, status.OutboxFailed, status.OutboxProcessing)
	if err != nil {
		return nil, err
	}
	if len(pending) > 0 {
		t := pending[0].OccurredAt
		oldest = &t
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
	tdb, err := tenant.TenantDB(ctx, s.db)
	if err != nil {
		return nil, err
	}
	events, err := tenant.TenantRaw[outbox.Event](tdb, "SELECT * FROM outbox_events WHERE tenant_id = ? AND id = ? LIMIT 1", req.EventID)
	if err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return nil, errs.NewKey(errs.CodeNotFound, "outbox.event_not_found", "outbox event not found")
	}
	ev := events[0]
	deliveries, err := tenant.TenantRaw[outbox.Delivery](tdb, `SELECT d.* FROM event_deliveries AS d
JOIN outbox_events AS e ON e.id = d.event_id
WHERE e.tenant_id = ? AND e.id = ? ORDER BY d.handler_name`, ev.ID)
	if err != nil {
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
	tdb, err := tenant.TenantDB(ctx, s.db)
	if err != nil {
		return nil, err
	}
	events, err := tenant.TenantRaw[outbox.Event](tdb, "SELECT * FROM outbox_events WHERE tenant_id = ? AND id = ? LIMIT 1", req.EventID)
	if err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return nil, errs.NewKey(errs.CodeNotFound, "outbox.event_not_found", "outbox event not found")
	}
	ev := events[0]
	if err := outbox.RetryTenantEvent(ctx, s.db, ev.ID); err != nil {
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
		ai                                     *action.AIMeta
	}{
		{ActionOutboxList, "/api/v1/operations/outbox/list", "Outbox 事件列表", "action.operations.outbox_list.name", "分页查询租户 Outbox 事件", "action.operations.outbox_list.description", &auditOff, action.JSON(svc.list), readAIMeta("查询当前租户 Outbox 事件，可按事件类型和状态筛选", "查询 Outbox 事件列表", "已返回当前租户可见的 Outbox 事件", action.InputSchemaFor[outboxListRequest](), action.SchemaFor[pagination.Result[outbox.Event]](), "运维", "Outbox", "事件")},
		{ActionOutboxSummary, "/api/v1/operations/outbox/summary", "Outbox 摘要", "action.operations.outbox_summary.name", "查询租户 Outbox 状态计数", "action.operations.outbox_summary.description", &auditOff, svc.Summary, readAIMeta("查询当前租户 Outbox 各状态数量和最早待处理时间", "查询 Outbox 运行摘要", "已返回当前租户 Outbox 运行摘要", action.InputSchemaFor[struct{}](), action.SchemaFor[outboxSummaryResponse](), "运维", "Outbox", "摘要")},
		{ActionOutboxListDeliveries, "/api/v1/operations/outbox/deliveries", "Outbox 投递记录", "action.operations.outbox_list_deliveries.name", "查看事件 Handler 投递状态", "action.operations.outbox_list_deliveries.description", &auditOff, action.JSON(svc.listDeliveries), readAIMeta("按事件 ID 查询当前租户 Outbox Handler 投递状态", "查询 Outbox 事件投递详情", "已返回指定 Outbox 事件及其 Handler 投递状态", action.InputSchemaFor[outboxEventIDRequest](), action.SchemaFor[outboxDeliveriesResponse](), "运维", "Outbox", "投递")},
		{ActionOutboxRetry, "/api/v1/operations/outbox/retry", "重试 Outbox 事件", "action.operations.outbox_retry.name", "将 failed/dead 事件重置为 pending（不可改 payload）", "action.operations.outbox_retry.description", &auditOn, action.JSON(svc.retry), mutationAIMeta("将失败或终止的 Outbox 事件重置为待处理，仅接受事件 ID 且不可修改原始载荷", "重试 Outbox 事件", "已将指定 Outbox 事件重置为待处理", "high", "write", action.InputSchemaFor[outboxEventIDRequest](), action.SchemaFor[outboxRetryResponse](), "运维", "Outbox", "重试")},
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
				AI:                     spec.ai,
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
