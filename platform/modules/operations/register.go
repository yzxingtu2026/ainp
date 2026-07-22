package operations

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/capability"
	"github.com/yzxingtu2026/ainp/platform/framework/menu"
	"github.com/yzxingtu2026/ainp/platform/framework/pagination"
	operationshelp "github.com/yzxingtu2026/ainp/platform/modules/operations/help"
)

const (
	ActionNotificationDeliveryList  = "operations.notification_delivery_list"
	ActionNotificationDeliveryGet   = "operations.notification_delivery_get"
	ActionNotificationDeliveryRetry = "operations.notification_delivery_retry"
)

type NotificationDeliveryService interface {
	List(context.Context, json.RawMessage) (any, error)
	Get(context.Context, json.RawMessage) (any, error)
	Retry(context.Context, json.RawMessage) (any, error)
}

type notificationDeliveryIDRequest struct {
	ID string `json:"id" validate:"required"`
}

type notificationDeliveryListRequest struct {
	pagination.Request
	Status  string `json:"status" validate:"omitempty,oneof=pending queued sending sent delivered failed dead cancelled skipped"`
	Channel string `json:"channel" validate:"omitempty,oneof=in_app email sms webhook im"`
}

type notificationDeliveryView struct {
	ID            string     `json:"id"`
	TenantID      string     `json:"tenant_id"`
	RequestID     string     `json:"request_id"`
	RecipientType string     `json:"recipient_type"`
	RecipientID   string     `json:"recipient_id"`
	RecipientName string     `json:"recipient_name"`
	Channel       string     `json:"channel"`
	Provider      string     `json:"provider"`
	Status        string     `json:"status"`
	Attempts      int        `json:"attempts"`
	MaxAttempts   int        `json:"max_attempts"`
	ExternalID    string     `json:"external_id"`
	LastError     string     `json:"last_error"`
	SentAt        *time.Time `json:"sent_at"`
	DeliveredAt   *time.Time `json:"delivered_at"`
	FailedAt      *time.Time `json:"failed_at"`
	ScheduledAt   *time.Time `json:"scheduled_at"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type notificationDeliveryContent struct {
	Title   string `json:"title"`
	Summary string `json:"summary"`
	Body    string `json:"body"`
}

type notificationDeliveryDetail struct {
	notificationDeliveryView
	Locale        string                      `json:"locale"`
	DefinitionKey string                      `json:"definition_key"`
	Variables     map[string]any              `json:"variables"`
	Content       notificationDeliveryContent `json:"content"`
}

type notificationDeliveryRetryResponse struct {
	Retried bool `json:"retried"`
}

func typedNotificationDeliveryID(handler func(context.Context, json.RawMessage) (any, error)) action.Handler {
	return action.JSON(func(ctx context.Context, request notificationDeliveryIDRequest) (any, error) {
		input, err := json.Marshal(request)
		if err != nil {
			return nil, err
		}
		return handler(ctx, input)
	})
}

func Register(a *app.App, deliveries NotificationDeliveryService) error {
	if err := a.RegisterCapability(capability.Capability{
		ID: "operations", Name: "运维", NameKey: "capability.operations.name",
		Description: "运行态任务、事件与投递运维", DescriptionKey: "capability.operations.description",
		Module: "operations", Order: 10,
	}); err != nil {
		return err
	}
	if err := registerJobActions(a); err != nil {
		return err
	}
	if err := registerOutboxActions(a); err != nil {
		return err
	}
	if err := registerAuditLogActions(a); err != nil {
		return err
	}
	if a.PageContexts == nil {
		return fmt.Errorf("page context registry is required")
	}
	if err := a.PageContexts.Register(PageTypeAuditLogs, auditPageContextResolver{db: a.DB}); err != nil {
		return err
	}
	if deliveries != nil {
		if err := registerNotificationDeliveryActions(a, deliveries); err != nil {
			return err
		}
	}
	return registerMenus(a, deliveries != nil)
}

func registerNotificationDeliveryActions(a *app.App, svc NotificationDeliveryService) error {
	auditOn, auditOff := true, false
	specs := []struct {
		id, path, name, nameKey, desc, descKey string
		audit                                  *bool
		handler                                func(context.Context, json.RawMessage) (any, error)
		ai                                     *action.AIMeta
	}{
		{ActionNotificationDeliveryList, "/api/v1/operations/notification-deliveries/list", "投递列表", "action.operations.notification_delivery_list.name", "查询通知投递记录", "action.operations.notification_delivery_list.description", &auditOff, svc.List, readAIMeta("查询当前租户通知投递记录，可按状态和渠道筛选", "查询通知投递列表", "已返回当前租户可见的通知投递记录", action.InputSchemaFor[notificationDeliveryListRequest](), action.SchemaFor[pagination.Result[notificationDeliveryView]](), "运维", "通知", "投递")},
		{ActionNotificationDeliveryGet, "/api/v1/operations/notification-deliveries/get", "投递详情", "action.operations.notification_delivery_get.name", "查询通知投递详情", "action.operations.notification_delivery_get.description", &auditOff, typedNotificationDeliveryID(svc.Get), readAIMeta("按投递 ID 查询当前租户通知投递详情", "查询通知投递详情", "已返回指定通知投递详情", action.InputSchemaFor[notificationDeliveryIDRequest](), action.SchemaFor[notificationDeliveryDetail](), "运维", "通知", "投递", "详情")},
		{ActionNotificationDeliveryRetry, "/api/v1/operations/notification-deliveries/retry", "重试投递", "action.operations.notification_delivery_retry.name", "重试失败通知投递", "action.operations.notification_delivery_retry.description", &auditOn, typedNotificationDeliveryID(svc.Retry), mutationAIMeta("重试当前租户中失败或终止的通知投递，仅接受投递 ID", "重试通知投递", "已重新提交指定通知投递", "high", "external", action.InputSchemaFor[notificationDeliveryIDRequest](), action.SchemaFor[notificationDeliveryRetryResponse](), "运维", "通知", "投递", "重试")},
	}
	for _, spec := range specs {
		if err := a.RegisterAction(action.Action{ID: spec.id, Handler: spec.handler, Meta: action.Metadata{
			Capability: "operations", Name: spec.name, NameKey: spec.nameKey,
			Description: spec.desc, DescriptionKey: spec.descKey,
			PermissionGroup: "operations.notification_delivery", PermissionGroupName: "通知投递运维",
			PermissionGroupNameKey: "permission_group.operations.notification_delivery",
			Audit:                  spec.audit, AI: spec.ai, HTTP: &action.HTTPMeta{Method: "POST", Path: spec.path},
		}}); err != nil {
			return err
		}
	}
	return nil
}

func registerMenus(a *app.App, includeDeliveries bool) error {
	if err := a.RegisterMenu(menu.Menu{ID: "operations.root", Title: "运维", TitleKey: "menu.operations.root.title", Icon: "mdi:monitor-dashboard", Type: menu.TypeDirectory, Module: "operations", Order: 20}); err != nil {
		return err
	}
	menus := []menu.Menu{
		{ID: "operations.jobs", ParentID: "operations.root", Title: "任务运行", TitleKey: "menu.operations.jobs.title", Icon: "mdi:briefcase-clock-outline", Type: menu.TypeMenu, Path: "/operations/jobs", Component: "operations/jobs", Permission: ActionJobList, BindAction: ActionJobList, Module: "operations", Order: 1},
		{ID: "operations.outbox", ParentID: "operations.root", Title: "Outbox 事件", TitleKey: "menu.operations.outbox.title", Icon: "mdi:tray-arrow-up", Type: menu.TypeMenu, Path: "/operations/outbox", Component: "operations/outbox", Permission: ActionOutboxList, BindAction: ActionOutboxList, Module: "operations", Order: 2},
	}
	if includeDeliveries {
		menus = append(menus, menu.Menu{ID: "operations.notification_deliveries", ParentID: "operations.root", Title: "通知投递", TitleKey: "menu.operations.notification_deliveries.title", Icon: "mdi:truck-delivery-outline", Type: menu.TypeMenu, Path: "/operations/notification-deliveries", Component: "operations/notification-deliveries", Permission: ActionNotificationDeliveryList, BindAction: ActionNotificationDeliveryList, Module: "operations", Order: 3})
	}
	menus = append(menus, menu.Menu{ID: "operations.audit_logs", ParentID: "operations.root", Title: "操作审计", TitleKey: "menu.operations.audit_logs.title", Icon: "mdi:text-box-search-outline", Type: menu.TypeMenu, Path: "/operations/audit-logs", Component: "operations/audit-logs", Permission: ActionAuditLogList, BindAction: ActionAuditLogList, Module: "operations", Order: 4})
	for _, item := range menus {
		if err := a.RegisterMenu(item); err != nil {
			return err
		}
	}
	if err := operationshelp.Register(a.PageHelp, includeDeliveries); err != nil {
		return fmt.Errorf("register operations page help: %w", err)
	}
	if err := a.PageHelp.ValidateMenus(a.Menus); err != nil {
		return err
	}
	return a.PageHelp.ValidateModuleCoverage(a.Menus, "operations", "zh-CN")
}
