package system

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/i18n"
	"github.com/yzxingtu2026/ainp/platform/framework/job"
	"github.com/yzxingtu2026/ainp/platform/framework/logging"
	"github.com/yzxingtu2026/ainp/platform/framework/menu"
	"github.com/yzxingtu2026/ainp/platform/framework/notification"
	"github.com/yzxingtu2026/ainp/platform/framework/outbox"
	"github.com/yzxingtu2026/ainp/platform/framework/pagination"
	"github.com/yzxingtu2026/ainp/platform/modules/system/jobs"
	"gorm.io/gorm"
)

const (
	ActionNotificationList            = "system.notification_list"
	ActionNotificationUnreadCount     = "system.notification_unread_count"
	ActionNotificationMarkRead        = "system.notification_mark_read"
	ActionNotificationMarkAllRead     = "system.notification_mark_all_read"
	ActionNotificationArchive         = "system.notification_archive"
	ActionNotificationUnarchive       = "system.notification_unarchive"
	ActionNotificationTemplateList    = "system.notification_template_list"
	ActionNotificationTemplateUpdate  = "system.notification_template_update"
	ActionNotificationTemplateReset   = "system.notification_template_reset"
	ActionNotificationTemplatePreview = "system.notification_template_preview"
)

// InAppNotificationService provides user-facing notification actions.
type InAppNotificationService struct {
	repo *NotificationRepository
}

type inAppNotificationListRequest struct {
	pagination.Request
	UnreadOnly      bool `json:"unread_only"`
	ExcludeArchived bool `json:"exclude_archived"`
}

func NewInAppNotificationService(repo *NotificationRepository) *InAppNotificationService {
	return &InAppNotificationService{repo: repo}
}

func (s *InAppNotificationService) List(ctx context.Context, input json.RawMessage) (any, error) {
	return action.JSON(s.list)(ctx, input)
}

func (s *InAppNotificationService) list(ctx context.Context, req inAppNotificationListRequest) (any, error) {
	ac, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	items, total, err := s.repo.ListUserNotifications(ctx, ac.UserID, req.UnreadOnly, req.ExcludeArchived, req.Page, req.PageSize)
	if err != nil {
		return nil, err
	}

	// Map to API response
	notifications := make([]map[string]any, len(items))
	for i, n := range items {
		var readAt any
		if n.ReadAt.Valid {
			readAt = n.ReadAt.Time
		}
		var archivedAt any
		if n.ArchivedAt.Valid {
			archivedAt = n.ArchivedAt.Time
		}
		notifications[i] = map[string]any{
			"id":          n.ID,
			"title":       n.Title,
			"summary":     n.Summary,
			"body":        n.Body,
			"icon":        n.Icon,
			"local_icon":  n.LocalIcon,
			"action_json": json.RawMessage(n.ActionJSON),
			"read_at":     readAt,
			"archived_at": archivedAt,
			"created_at":  n.CreatedAt,
		}
	}
	return pagination.NewResult(notifications, total, req.Request), nil
}

func (s *InAppNotificationService) UnreadCount(ctx context.Context, _ json.RawMessage) (any, error) {
	ac, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	count, err := s.repo.CountUnread(ctx, ac.UserID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"unread_count": count}, nil
}

func (s *InAppNotificationService) MarkRead(ctx context.Context, input json.RawMessage) (any, error) {
	ac, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(input, &req); err != nil || strings.TrimSpace(req.ID) == "" {
		return nil, errs.ValidationKey("common.validation_failed", "id required")
	}
	if err := s.repo.MarkRead(ctx, ac.UserID, req.ID); err != nil {
		return nil, err
	}
	return map[string]any{"id": req.ID, "read_at": time.Now().UTC()}, nil
}

func (s *InAppNotificationService) MarkAllRead(ctx context.Context, _ json.RawMessage) (any, error) {
	ac, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	affected, err := s.repo.MarkAllRead(ctx, ac.UserID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"affected": affected}, nil
}

func (s *InAppNotificationService) Archive(ctx context.Context, input json.RawMessage) (any, error) {
	ac, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(input, &req); err != nil || strings.TrimSpace(req.ID) == "" {
		return nil, errs.ValidationKey("common.validation_failed", "id required")
	}
	if err := s.repo.Archive(ctx, ac.UserID, req.ID); err != nil {
		return nil, err
	}
	return map[string]any{"id": req.ID, "archived": true}, nil
}

func (s *InAppNotificationService) Unarchive(ctx context.Context, input json.RawMessage) (any, error) {
	ac, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(input, &req); err != nil || strings.TrimSpace(req.ID) == "" {
		return nil, errs.ValidationKey("common.validation_failed", "id required")
	}
	if err := s.repo.Unarchive(ctx, ac.UserID, req.ID); err != nil {
		return nil, err
	}
	return map[string]any{"id": req.ID, "archived": false}, nil
}

// --- Template management actions ---

type NotificationTemplateService struct {
	repo     *NotificationRepository
	registry *notification.DefinitionRegistry
	renderer *notification.Renderer
	override notification.OverrideProvider
}

func NewNotificationTemplateService(repo *NotificationRepository, registry *notification.DefinitionRegistry, renderer *notification.Renderer, override notification.OverrideProvider) *NotificationTemplateService {
	return &NotificationTemplateService{repo: repo, registry: registry, renderer: renderer, override: override}
}

func (s *NotificationTemplateService) List(ctx context.Context, _ json.RawMessage) (any, error) {
	if _, err := requireTenant(ctx); err != nil {
		return nil, err
	}

	// 获取请求语言，用于翻译 name/description
	lang := i18n.FromCtx(ctx)

	// 1. 获取代码注册的所有通知定义
	defs := s.registry.List()

	// 2. 获取数据库中的租户覆盖
	overrides, err := s.repo.ListTemplateOverrides(ctx)
	if err != nil {
		return nil, err
	}

	// 3. 构建覆盖索引 map[definition_key:channel:locale] -> override
	overrideMap := make(map[string]*NotificationTemplateOverride)
	for i := range overrides {
		key := overrides[i].DefinitionKey + ":" + overrides[i].Channel + ":" + overrides[i].Locale
		overrideMap[key] = &overrides[i]
	}

	// 4. 合并：以定义为主体，附加覆盖信息
	items := make([]map[string]any, 0, len(defs))
	for _, def := range defs {
		// 收集变量字段
		varFields := make([]map[string]any, len(def.Variables.Fields))
		for i, f := range def.Variables.Fields {
			varFields[i] = map[string]any{
				"name":      f.Name,
				"type":      f.Type,
				"required":  f.Required,
				"sensitive": f.Sensitive,
			}
		}

		// 收集模板变体（默认值 + 覆盖）
		templates := make([]map[string]any, 0, len(def.Templates.Variants))
		for _, ct := range def.Templates.Variants {
			key := def.Key + ":" + string(ct.Channel) + ":" + ct.Locale
			ov := overrideMap[key]

			// 默认 subject/body 取决于渠道
			defaultSubject := ct.Subject
			defaultBody := ct.Body
			if ct.InApp != nil {
				if defaultSubject == "" {
					defaultSubject = ct.InApp.Title
				}
				if defaultBody == "" {
					defaultBody = ct.InApp.Body
				}
			}
			if ct.Email != nil && defaultSubject == "" {
				defaultSubject = ct.Email.Subject
			}

			entry := map[string]any{
				"channel":         string(ct.Channel),
				"locale":          ct.Locale,
				"default_subject": defaultSubject,
				"default_body":    defaultBody,
				"has_override":    ov != nil,
			}

			if ov != nil {
				entry["override"] = map[string]any{
					"subject":    ov.Subject,
					"body":       ov.Body,
					"enabled":    ov.Enabled,
					"updated_at": ov.UpdatedAt,
				}
			} else {
				entry["override"] = nil
			}

			templates = append(templates, entry)
		}

		// 转换 channel slice 为 string slice
		allowedChannels := make([]string, len(def.AllowedChannels))
		for i, ch := range def.AllowedChannels {
			allowedChannels[i] = string(ch)
		}
		defaultChannels := make([]string, len(def.DefaultChannels))
		for i, ch := range def.DefaultChannels {
			defaultChannels[i] = string(ch)
		}

		items = append(items, map[string]any{
			"definition_key":   def.Key,
			"name":             i18n.T(lang, def.NameKey),
			"description":      i18n.T(lang, def.DescriptionKey),
			"category":         def.Category,
			"name_key":         def.NameKey,
			"description_key":  def.DescriptionKey,
			"priority":         string(def.Priority),
			"allowed_channels": allowedChannels,
			"default_channels": defaultChannels,
			"variables":        varFields,
			"templates":        templates,
		})
	}

	return map[string]any{"items": items}, nil
}

func (s *NotificationTemplateService) Update(ctx context.Context, input json.RawMessage) (any, error) {
	ac, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	var req struct {
		DefinitionKey string `json:"definition_key"`
		Channel       string `json:"channel"`
		Locale        string `json:"locale"`
		Subject       string `json:"subject"`
		Body          string `json:"body"`
		Enabled       *bool  `json:"enabled"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, errs.ValidationKey("common.validation_failed", "invalid input")
	}
	if req.DefinitionKey == "" || req.Channel == "" || req.Locale == "" {
		return nil, errs.ValidationKey("common.validation_failed", "definition_key, channel, locale are required")
	}
	if !notification.Channel(req.Channel).Valid() {
		return nil, errs.ValidationKey("common.validation_failed", "invalid channel")
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	override := &NotificationTemplateOverride{
		DefinitionKey: req.DefinitionKey,
		Channel:       req.Channel,
		Locale:        req.Locale,
		Subject:       req.Subject,
		Body:          req.Body,
		Enabled:       enabled,
		UpdatedBy:     ac.UserID,
	}
	if err := s.repo.UpsertTemplateOverride(ctx, override); err != nil {
		return nil, err
	}
	ac.AuditResourceType = "notification_template_override"
	ac.AuditResourceID = req.DefinitionKey + ":" + req.Channel + ":" + req.Locale
	ac.AuditSummary = "update template override"
	return map[string]any{"updated": true}, nil
}

func (s *NotificationTemplateService) Reset(ctx context.Context, input json.RawMessage) (any, error) {
	ac, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	var req struct {
		DefinitionKey string `json:"definition_key"`
		Channel       string `json:"channel"`
		Locale        string `json:"locale"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, errs.ValidationKey("common.validation_failed", "invalid input")
	}
	if req.DefinitionKey == "" || req.Channel == "" || req.Locale == "" {
		return nil, errs.ValidationKey("common.validation_failed", "definition_key, channel, locale are required")
	}
	if err := s.repo.DeleteTemplateOverride(ctx, req.DefinitionKey, req.Channel, req.Locale); err != nil {
		return nil, err
	}
	ac.AuditResourceType = "notification_template_override"
	ac.AuditResourceID = req.DefinitionKey + ":" + req.Channel + ":" + req.Locale
	ac.AuditSummary = "reset template override to code default"
	return map[string]any{"reset": true}, nil
}

// Preview renders a template with variables without triggering delivery.
func (s *NotificationTemplateService) Preview(ctx context.Context, input json.RawMessage) (any, error) {
	ac, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	var req struct {
		DefinitionKey string         `json:"definition_key"`
		Channel       string         `json:"channel"`
		Locale        string         `json:"locale"`
		Variables     map[string]any `json:"variables"`
	}
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, errs.ValidationKey("common.validation_failed", "invalid input")
	}
	if req.DefinitionKey == "" || req.Channel == "" || req.Locale == "" {
		return nil, errs.ValidationKey("common.validation_failed", "definition_key, channel, locale are required")
	}

	def, ok := s.registry.Get(req.DefinitionKey)
	if !ok {
		return nil, errs.NewKey(errs.CodeNotificationDefinitionNotFound, "notification.definition_not_found", req.DefinitionKey)
	}

	resolver := notification.NewTemplateResolver(s.renderer, s.override)
	resolved, err := resolver.Resolve(ctx, ac.TenantID, def, notification.Channel(req.Channel), req.Locale)
	if err != nil {
		return nil, errs.NewKey(errs.CodeNotificationTemplateMissing, "notification.template_missing", err.Error())
	}

	rendered, err := resolver.Render(resolved, req.Variables)
	if err != nil {
		return nil, errs.ValidationKey("notification.render_failed", err.Error())
	}

	return map[string]any{
		"source":    resolved.Source,
		"version":   resolved.Version,
		"channel":   string(resolved.Channel),
		"locale":    resolved.Locale,
		"subject":   rendered.Subject,
		"body":      rendered.Body,
		"title":     rendered.Title,
		"summary":   rendered.Summary,
		"text_body": rendered.TextBody,
		"html_body": rendered.HTMLBody,
		"sms_text":  rendered.SMSText,
		"im_text":   rendered.IMText,
	}, nil
}

// --- Delivery management actions ---

type NotificationDeliveryService struct {
	repo     *NotificationRepository
	enqueuer job.Enqueuer
}

type notificationDeliveryListRequest struct {
	pagination.Request
	Status  string `json:"status" validate:"omitempty,oneof=pending queued sending sent delivered failed dead cancelled skipped"`
	Channel string `json:"channel" validate:"omitempty,oneof=in_app email sms webhook im"`
}

func NewNotificationDeliveryService(repo *NotificationRepository, enqueuer ...job.Enqueuer) *NotificationDeliveryService {
	service := &NotificationDeliveryService{repo: repo}
	if len(enqueuer) > 0 {
		service.enqueuer = enqueuer[0]
	}
	return service
}

func NewOperationsNotificationDeliveryService(a *app.App) *NotificationDeliveryService {
	if a == nil || a.DB == nil {
		return nil
	}
	var enqueuer job.Enqueuer
	if a.Jobs != nil && a.Jobs.Enabled {
		enqueuer = a.Jobs.Enqueuer
	}
	return NewNotificationDeliveryService(NewNotificationRepository(a.DB), enqueuer)
}

func (s *NotificationDeliveryService) List(ctx context.Context, input json.RawMessage) (any, error) {
	return action.JSON(s.list)(ctx, input)
}

func (s *NotificationDeliveryService) list(ctx context.Context, req notificationDeliveryListRequest) (any, error) {
	ac, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	items, total, err := s.repo.ListDeliveries(ctx, req.Page, req.PageSize, req.Status, req.Channel)
	if err != nil {
		return nil, err
	}
	// Batch lookup recipient display names (user type only)
	var userIDs []string
	seen := make(map[string]struct{})
	for _, d := range items {
		if d.RecipientType == "user" || d.RecipientType == "" {
			if _, ok := seen[d.RecipientID]; !ok {
				seen[d.RecipientID] = struct{}{}
				userIDs = append(userIDs, d.RecipientID)
			}
		}
	}
	nameMap, _ := s.repo.BatchLookupUserNames(ctx, ac.TenantID, userIDs)
	// Map to API response with proper null serialization
	result := make([]map[string]any, len(items))
	for i, d := range items {
		var sentAt, deliveredAt, failedAt, scheduledAt any
		if d.SentAt != nil {
			sentAt = d.SentAt
		}
		if d.DeliveredAt != nil {
			deliveredAt = d.DeliveredAt
		}
		if d.FailedAt != nil {
			failedAt = d.FailedAt
		}
		if d.ScheduledAt != nil {
			scheduledAt = d.ScheduledAt
		}
		recipientName := nameMap[d.RecipientID]
		result[i] = map[string]any{
			"id":             d.ID,
			"tenant_id":      d.TenantID,
			"request_id":     d.RequestID,
			"recipient_type": d.RecipientType,
			"recipient_id":   d.RecipientID,
			"recipient_name": recipientName,
			"channel":        d.Channel,
			"provider":       d.Provider,
			"status":         d.Status,
			"attempts":       d.Attempts,
			"max_attempts":   d.MaxAttempts,
			"external_id":    d.ExternalID,
			"last_error":     d.LastError,
			"sent_at":        sentAt,
			"delivered_at":   deliveredAt,
			"failed_at":      failedAt,
			"scheduled_at":   scheduledAt,
			"created_at":     d.CreatedAt,
			"updated_at":     d.UpdatedAt,
		}
	}
	return pagination.NewResult(result, total, req.Request), nil
}

func (s *NotificationDeliveryService) Retry(ctx context.Context, input json.RawMessage) (any, error) {
	ac, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(input, &req); err != nil || strings.TrimSpace(req.ID) == "" {
		return nil, errs.ValidationKey("common.validation_failed", "id required")
	}
	if s.enqueuer == nil {
		return nil, errs.NewKey(errs.CodeNotificationDeliveryFailed, "notification.delivery_failed", "notification jobs unavailable")
	}
	delivery, err := s.repo.GetDelivery(ctx, req.ID)
	if err != nil {
		return nil, err
	}
	if delivery == nil {
		return nil, errs.NewKey(errs.CodeNotFound, "notification.delivery_not_found", "delivery not found")
	}
	if delivery.Status != string(notification.DeliveryFailed) && delivery.Status != string(notification.DeliveryDead) {
		return nil, errs.ValidationKey("notification.delivery_not_retryable", "delivery is not failed or dead")
	}
	if _, err := s.enqueuer.Enqueue(ctx, job.EnqueueInput{
		Type:           jobs.NotificationDeliverJobType,
		IdempotencyKey: "notification-retry:" + req.ID + ":" + time.Now().UTC().Format(time.RFC3339Nano),
		Payload:        jobs.NotificationDeliverPayload{DeliveryID: req.ID},
	}); err != nil {
		return nil, errs.Wrap(errs.CodeNotificationDeliveryFailed, "enqueue notification retry failed", err)
	}
	if err := s.repo.RetryDelivery(ctx, req.ID); err != nil {
		return nil, err
	}
	ac.AuditResourceType = "notification_delivery"
	ac.AuditResourceID = req.ID
	ac.AuditSummary = "retry failed delivery"
	return map[string]any{"retried": true}, nil
}

// Get returns a single delivery detail with rendered message content.
func (s *NotificationDeliveryService) Get(ctx context.Context, input json.RawMessage) (any, error) {
	if _, err := requireTenant(ctx); err != nil {
		return nil, err
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(input, &req); err != nil || strings.TrimSpace(req.ID) == "" {
		return nil, errs.ValidationKey("common.validation_failed", "id required")
	}
	row, err := s.repo.GetDeliveryDetail(ctx, req.ID)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, errs.NewKey(errs.CodeNotFound, "notification.delivery_not_found", "delivery not found")
	}
	var sentAt, deliveredAt, failedAt, scheduledAt any
	if row.SentAt != nil {
		sentAt = row.SentAt
	}
	if row.DeliveredAt != nil {
		deliveredAt = row.DeliveredAt
	}
	if row.FailedAt != nil {
		failedAt = row.FailedAt
	}
	if row.ScheduledAt != nil {
		scheduledAt = row.ScheduledAt
	}
	var variables map[string]any
	_ = json.Unmarshal(row.VariablesJSON, &variables)
	return map[string]any{
		"id":             row.ID,
		"tenant_id":      row.TenantID,
		"request_id":     row.RequestID,
		"recipient_type": row.RecipientType,
		"recipient_id":   row.RecipientID,
		"recipient_name": row.RecipientName,
		"channel":        row.Channel,
		"provider":       row.Provider,
		"locale":         row.Locale,
		"status":         row.Status,
		"attempts":       row.Attempts,
		"max_attempts":   row.MaxAttempts,
		"external_id":    row.ExternalID,
		"last_error":     row.LastError,
		"definition_key": row.DefinitionKey,
		"variables":      variables,
		"content": map[string]any{
			"title":   row.Title,
			"summary": row.Summary,
			"body":    row.Body,
		},
		"sent_at":      sentAt,
		"delivered_at": deliveredAt,
		"failed_at":    failedAt,
		"scheduled_at": scheduledAt,
		"created_at":   row.CreatedAt,
		"updated_at":   row.UpdatedAt,
	}, nil
}

// --- Registration ---

func registerNotificationActions(a *app.App) error {
	if a.DB == nil {
		return nil
	}
	repo := NewNotificationRepository(a.DB)

	// Auto-migrate notification tables
	if err := repo.AutoMigrate(); err != nil {
		return err
	}

	inAppSvc := NewInAppNotificationService(repo)
	overrideProvider := NewGormOverrideProvider(repo)
	tplSvc := NewNotificationTemplateService(repo, a.Notifications, a.NotificationRenderer, overrideProvider)
	// ── Build Orchestrator + NotificationService ──
	inAppProvider := NewInAppProvider(repo)
	providers := map[notification.Channel]notification.ChannelProvider{
		notification.ChannelInApp: inAppProvider,
	}
	recipientResolver := notification.NewTenantUserResolver(UserExistsFn(a.DB))

	var notifSvc *NotificationService
	var enqueuer *NotificationJobEnqueuer
	if a.Jobs != nil && a.Jobs.Enabled {
		enqueuer = NewNotificationJobEnqueuer(func(ctx context.Context, deliveryID string) error {
			_, err := a.Jobs.Enqueuer.Enqueue(ctx, job.EnqueueInput{
				Type:           jobs.NotificationDeliverJobType,
				IdempotencyKey: "notif-delivery:" + deliveryID,
				Payload:        jobs.NotificationDeliverPayload{DeliveryID: deliveryID},
			})
			return err
		})
	} else {
		// No-op enqueuer when Jobs disabled
		enqueuer = NewNotificationJobEnqueuer(func(ctx context.Context, deliveryID string) error {
			logging.Warn(ctx, "notification jobs disabled; delivery not enqueued", "delivery_id", deliveryID)
			return nil
		})
	}
	notifSvc = NewNotificationService(a.DB, repo, a.Notifications, a.NotificationRenderer,
		notification.NewTemplateResolver(a.NotificationRenderer, overrideProvider),
		recipientResolver, enqueuer, providers)
	_ = notifSvc // used for Outbox handler below

	// ── Register notification.deliver Job ──
	if a.Jobs != nil {
		deliverStore := NewGormDeliverStore(repo)
		deliverHandler := jobs.NewNotificationDeliverHandler(
			deliverStore, providers, a.NotificationRenderer,
			notification.NewTemplateResolver(a.NotificationRenderer, overrideProvider),
			a.Notifications,
		)
		if err := a.Jobs.Register(jobs.NotificationDeliverJobDefinition(), deliverHandler); err != nil {
			return err
		}
	}

	// ── Register Outbox event definition + handler ──
	if a.Outbox != nil {
		if err := a.Outbox.RegisterDefinition(outbox.Definition{
			EventType: "notification.requested.v1", SchemaVersion: 1,
			Description: "Business requests a notification delivery via Orchestrator",
		}); err != nil {
			return err
		}
		outboxHandler := NewNotificationOutboxHandler(notifSvc)
		if err := a.Outbox.RegisterHandler(outboxHandler); err != nil {
			return err
		}
	}

	auditOn := true
	auditOff := false

	// User notification actions are authenticated base capabilities scoped to the current user.
	userSpecs := []struct {
		id, path, name, nameKey, desc, descKey string
		audit                                  *bool
		handler                                func(context.Context, json.RawMessage) (any, error)
	}{
		{ActionNotificationList, "/api/v1/notifications/list", "通知列表", "action.system.notification_list.name", "分页查询当前用户站内通知", "action.system.notification_list.description", &auditOff, inAppSvc.List},
		{ActionNotificationUnreadCount, "/api/v1/notifications/unread-count", "未读数量", "action.system.notification_unread_count.name", "查询当前用户未读通知数", "action.system.notification_unread_count.description", &auditOff, inAppSvc.UnreadCount},
		{ActionNotificationMarkRead, "/api/v1/notifications/mark-read", "标记已读", "action.system.notification_mark_read.name", "标记单条通知为已读", "action.system.notification_mark_read.description", &auditOff, inAppSvc.MarkRead},
		{ActionNotificationMarkAllRead, "/api/v1/notifications/mark-all-read", "全部已读", "action.system.notification_mark_all_read.name", "标记所有通知为已读", "action.system.notification_mark_all_read.description", &auditOff, inAppSvc.MarkAllRead},
		{ActionNotificationArchive, "/api/v1/notifications/archive", "归档通知", "action.system.notification_archive.name", "归档单条通知", "action.system.notification_archive.description", &auditOff, inAppSvc.Archive},
		{ActionNotificationUnarchive, "/api/v1/notifications/unarchive", "取消归档", "action.system.notification_unarchive.name", "取消归档单条通知", "action.system.notification_unarchive.description", &auditOff, inAppSvc.Unarchive},
	}
	for _, spec := range userSpecs {
		if err := a.RegisterAction(action.Action{
			ID:      spec.id,
			Handler: spec.handler,
			Meta: action.Metadata{
				Capability:             "system",
				Access:                 action.AccessAuthenticated,
				Name:                   spec.name,
				NameKey:                spec.nameKey,
				Description:            spec.desc,
				DescriptionKey:         spec.descKey,
				PermissionGroup:        "system.notification_user",
				PermissionGroupName:    "通知操作",
				PermissionGroupNameKey: "permission_group.system.notification_user",
				Audit:                  spec.audit,
				HTTP:                   &action.HTTPMeta{Method: "POST", Path: spec.path},
			},
		}); err != nil {
			return err
		}
	}

	// Notification configuration remains in the system capability.
	adminSpecs := []struct {
		id, path, name, nameKey, desc, descKey string
		audit                                  *bool
		handler                                func(context.Context, json.RawMessage) (any, error)
	}{
		{ActionNotificationTemplateList, "/api/v1/system/notification/templates", "模板覆盖列表", "action.system.notification_template_list.name", "列出租户模板覆盖", "action.system.notification_template_list.description", &auditOff, tplSvc.List},
		{ActionNotificationTemplateUpdate, "/api/v1/system/notification/templates/update", "更新模板覆盖", "action.system.notification_template_update.name", "更新模板覆盖内容", "action.system.notification_template_update.description", &auditOn, tplSvc.Update},
		{ActionNotificationTemplateReset, "/api/v1/system/notification/templates/reset", "重置模板覆盖", "action.system.notification_template_reset.name", "重置为代码默认模板", "action.system.notification_template_reset.description", &auditOn, tplSvc.Reset},
		{ActionNotificationTemplatePreview, "/api/v1/system/notification/templates/preview", "模板预览", "action.system.notification_template_preview.name", "预览模板渲染结果（不触发投递）", "action.system.notification_template_preview.description", &auditOff, tplSvc.Preview},
	}
	for _, spec := range adminSpecs {
		if err := a.RegisterAction(action.Action{
			ID:      spec.id,
			Handler: spec.handler,
			Meta: action.Metadata{
				Capability:             "system",
				Name:                   spec.name,
				NameKey:                spec.nameKey,
				Description:            spec.desc,
				DescriptionKey:         spec.descKey,
				PermissionGroup:        "system.notification_configuration",
				PermissionGroupName:    "通知配置",
				PermissionGroupNameKey: "permission_group.system.notification_configuration",
				Audit:                  spec.audit,
				HTTP:                   &action.HTTPMeta{Method: "POST", Path: spec.path},
			},
		}); err != nil {
			return err
		}
	}

	// Register notification menu (hidden from sidebar; entry via header bell icon → "view all")
	if err := a.RegisterMenu(menu.Menu{
		ID:         "notification",
		ParentID:   "",
		Type:       "directory",
		Title:      "通知",
		TitleKey:   "menu.notification.title",
		Icon:       "mdi:bell-outline",
		Order:      50,
		HideInMenu: true,
	}); err != nil {
		return err
	}
	if err := a.RegisterMenu(menu.Menu{
		ID:         "notification.list",
		ParentID:   "notification",
		Type:       "menu",
		Title:      "我的通知",
		TitleKey:   "menu.notification.list.title",
		Icon:       "mdi:bell-outline",
		Order:      1,
		Path:       "/notification/list",
		Component:  "notification/list",
		HideInMenu: true,
	}); err != nil {
		return err
	}

	// Admin management menus (visible in sidebar under system)
	if err := a.RegisterMenu(menu.Menu{
		ID:         "system.notification_templates",
		ParentID:   "system.root",
		Title:      "通知模板",
		TitleKey:   "menu.system.notification_templates.title",
		Icon:       "mdi:email-edit-outline",
		Type:       menu.TypeMenu,
		Path:       "/system/notification-templates",
		Component:  "system/notification-templates",
		Permission: ActionNotificationTemplateList,
		BindAction: ActionNotificationTemplateList,
		Module:     "system",
		Order:      6,
	}); err != nil {
		return err
	}

	// ── Register notification definitions (code-level defaults) ──

	// 1. 用户欢迎通知
	if err := a.RegisterNotification(notification.Definition{
		Key:             "user.welcome",
		Category:        "user",
		Surface:         notification.SurfaceTenant,
		NameKey:         "notification.user.welcome.name",
		DescriptionKey:  "notification.user.welcome.description",
		Priority:        notification.PriorityNormal,
		AllowedChannels: []notification.Channel{notification.ChannelInApp},
		DefaultChannels: []notification.Channel{notification.ChannelInApp},
		Variables: notification.VariableSchema{
			Fields: []notification.VariableField{
				{Name: "username", Type: "string", Required: true},
				{Name: "tenant_name", Type: "string", Required: false},
			},
		},
		Templates: &notification.TemplateDefinition{
			Version: 1,
			Variants: map[string]notification.ChannelTemplate{
				"in_app:zh-CN": {
					Channel: notification.ChannelInApp, Locale: "zh-CN",
					InApp: &notification.InAppTemplate{
						Title:   "欢迎，{{.username}}",
						Body:    "您已成功加入{{.tenant_name}}，欢迎开始使用！",
						Icon:    "mdi:account-check",
						Summary: "新用户欢迎通知",
					},
				},
				"in_app:en": {
					Channel: notification.ChannelInApp, Locale: "en",
					InApp: &notification.InAppTemplate{
						Title:   "Welcome, {{.username}}",
						Body:    "You have joined {{.tenant_name}}. Welcome aboard!",
						Icon:    "mdi:account-check",
						Summary: "New user welcome notification",
					},
				},
			},
		},
	}); err != nil {
		return err
	}

	// 2. 密码重置通知
	if err := a.RegisterNotification(notification.Definition{
		Key:             "user.password_reset",
		Category:        "security",
		Surface:         notification.SurfaceTenant,
		NameKey:         "notification.user.password_reset.name",
		DescriptionKey:  "notification.user.password_reset.description",
		Priority:        notification.PriorityHigh,
		AllowedChannels: []notification.Channel{notification.ChannelInApp, notification.ChannelEmail},
		DefaultChannels: []notification.Channel{notification.ChannelInApp},
		Variables: notification.VariableSchema{
			Fields: []notification.VariableField{
				{Name: "username", Type: "string", Required: true},
				{Name: "reset_link", Type: "string", Required: true, Ephemeral: true},
				{Name: "expires_in", Type: "string", Required: false},
			},
		},
		Templates: &notification.TemplateDefinition{
			Version: 1,
			Variants: map[string]notification.ChannelTemplate{
				"in_app:zh-CN": {
					Channel: notification.ChannelInApp, Locale: "zh-CN",
					InApp: &notification.InAppTemplate{
						Title:   "密码重置请求",
						Body:    "{{.username}}，您请求重置密码，请在{{.expires_in}}内完成操作。",
						Icon:    "mdi:lock-reset",
						Summary: "安全验证通知",
					},
				},
				"in_app:en": {
					Channel: notification.ChannelInApp, Locale: "en",
					InApp: &notification.InAppTemplate{
						Title:   "Password Reset Request",
						Body:    "{{.username}}, you requested a password reset. Please complete within {{.expires_in}}.",
						Icon:    "mdi:lock-reset",
						Summary: "Security notification",
					},
				},
			},
		},
	}); err != nil {
		return err
	}

	// 3. 角色变更通知
	if err := a.RegisterNotification(notification.Definition{
		Key:             "system.role_changed",
		Category:        "system",
		Surface:         notification.SurfaceTenant,
		NameKey:         "notification.system.role_changed.name",
		DescriptionKey:  "notification.system.role_changed.description",
		Priority:        notification.PriorityNormal,
		AllowedChannels: []notification.Channel{notification.ChannelInApp},
		DefaultChannels: []notification.Channel{notification.ChannelInApp},
		Variables: notification.VariableSchema{
			Fields: []notification.VariableField{
				{Name: "username", Type: "string", Required: true},
				{Name: "role_name", Type: "string", Required: true},
				{Name: "action", Type: "string", Required: true}, // "assigned" or "removed"
			},
		},
		Templates: &notification.TemplateDefinition{
			Version: 1,
			Variants: map[string]notification.ChannelTemplate{
				"in_app:zh-CN": {
					Channel: notification.ChannelInApp, Locale: "zh-CN",
					InApp: &notification.InAppTemplate{
						Title:   "角色变更通知",
						Body:    "{{.username}}，您的角色已{{.action}}：{{.role_name}}",
						Icon:    "mdi:shield-account",
						Summary: "角色权限变更",
					},
				},
				"in_app:en": {
					Channel: notification.ChannelInApp, Locale: "en",
					InApp: &notification.InAppTemplate{
						Title:   "Role Changed",
						Body:    "{{.username}}, your role has been {{.action}}: {{.role_name}}",
						Icon:    "mdi:shield-account",
						Summary: "Role permission change",
					},
				},
			},
		},
	}); err != nil {
		return err
	}

	// 4. 系统公告
	if err := a.RegisterNotification(notification.Definition{
		Key:             "system.announcement",
		Category:        "system",
		Surface:         notification.SurfaceTenant,
		NameKey:         "notification.system.announcement.name",
		DescriptionKey:  "notification.system.announcement.description",
		Priority:        notification.PriorityNormal,
		AllowedChannels: []notification.Channel{notification.ChannelInApp},
		DefaultChannels: []notification.Channel{notification.ChannelInApp},
		Variables: notification.VariableSchema{
			Fields: []notification.VariableField{
				{Name: "title", Type: "string", Required: true},
				{Name: "content", Type: "string", Required: true},
				{Name: "importance", Type: "string", Required: false}, // "info", "warning"
			},
		},
		Templates: &notification.TemplateDefinition{
			Version: 1,
			Variants: map[string]notification.ChannelTemplate{
				"in_app:zh-CN": {
					Channel: notification.ChannelInApp, Locale: "zh-CN",
					InApp: &notification.InAppTemplate{
						Title:   "{{.title}}",
						Body:    "{{.content}}",
						Icon:    "mdi:bullhorn",
						Summary: "系统公告",
					},
				},
				"in_app:en": {
					Channel: notification.ChannelInApp, Locale: "en",
					InApp: &notification.InAppTemplate{
						Title:   "{{.title}}",
						Body:    "{{.content}}",
						Icon:    "mdi:bullhorn",
						Summary: "System Announcement",
					},
				},
			},
		},
	}); err != nil {
		return err
	}

	// 5. 测试通知（用于验证通知系统是否正常工作）
	if err := a.RegisterNotification(notification.Definition{
		Key:             "system.notification_test",
		Category:        "system",
		Surface:         notification.SurfaceTenant,
		NameKey:         "notification.system.notification_test.name",
		DescriptionKey:  "notification.system.notification_test.description",
		Priority:        notification.PriorityLow,
		AllowedChannels: []notification.Channel{notification.ChannelInApp},
		DefaultChannels: []notification.Channel{notification.ChannelInApp},
		Variables: notification.VariableSchema{
			Fields: []notification.VariableField{
				{Name: "message", Type: "string", Required: false},
			},
		},
		Templates: &notification.TemplateDefinition{
			Version: 1,
			Variants: map[string]notification.ChannelTemplate{
				"in_app:zh-CN": {
					Channel: notification.ChannelInApp, Locale: "zh-CN",
					InApp: &notification.InAppTemplate{
						Title:   "测试通知",
						Body:    "这是一条测试通知。{{if .message}}{{.message}}{{end}}",
						Icon:    "mdi:test-tube",
						Summary: "通知系统连通性测试",
					},
				},
				"in_app:en": {
					Channel: notification.ChannelInApp, Locale: "en",
					InApp: &notification.InAppTemplate{
						Title:   "Test Notification",
						Body:    "This is a test notification.{{if .message}} {{.message}}{{end}}",
						Icon:    "mdi:test-tube",
						Summary: "Notification system connectivity test",
					},
				},
			},
		},
	}); err != nil {
		return err
	}

	return nil
}

// Ensure auth is used
var _ = auth.MustFromContext

// Ensure gorm is used
var _ = gorm.ErrRecordNotFound
