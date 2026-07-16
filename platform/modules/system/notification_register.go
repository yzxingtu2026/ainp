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
	"github.com/yzxingtu2026/ainp/platform/framework/menu"
	"github.com/yzxingtu2026/ainp/platform/framework/notification"
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
	ActionNotificationDeliveryList    = "system.notification_delivery_list"
	ActionNotificationDeliveryRetry   = "system.notification_delivery_retry"
)

// InAppNotificationService provides user-facing notification actions.
type InAppNotificationService struct {
	repo *NotificationRepository
}

func NewInAppNotificationService(repo *NotificationRepository) *InAppNotificationService {
	return &InAppNotificationService{repo: repo}
}

func (s *InAppNotificationService) List(ctx context.Context, input json.RawMessage) (any, error) {
	ac, err := requireTenant(ctx)
	if err != nil {
		return nil, err
	}
	var req struct {
		UnreadOnly      bool `json:"unread_only"`
		ExcludeArchived bool `json:"exclude_archived"`
		Current         int  `json:"current"`
		Size            int  `json:"size"`
	}
	_ = json.Unmarshal(input, &req)
	if req.Current < 1 {
		req.Current = 1
	}
	if req.Size < 1 || req.Size > 100 {
		req.Size = 20
	}

	items, total, err := s.repo.ListUserNotifications(ctx, ac.UserID, req.UnreadOnly, req.ExcludeArchived, req.Current, req.Size)
	if err != nil {
		return nil, err
	}

	// Map to API response
	notifications := make([]map[string]any, len(items))
	for i, n := range items {
		notifications[i] = map[string]any{
			"id":          n.ID,
			"title":       n.Title,
			"summary":     n.Summary,
			"body":        n.Body,
			"icon":        n.Icon,
			"local_icon":  n.LocalIcon,
			"action_json": json.RawMessage(n.ActionJSON),
			"read_at":     n.ReadAt,
			"archived_at": n.ArchivedAt,
			"created_at":  n.CreatedAt,
		}
	}
	return map[string]any{
		"items":   notifications,
		"total":   total,
		"current": req.Current,
		"size":    req.Size,
	}, nil
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
	overrides, err := s.repo.ListTemplateOverrides(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]any{"items": overrides}, nil
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
	resolved, err := resolver.Resolve(ac.TenantID, def, notification.Channel(req.Channel), req.Locale)
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
	repo *NotificationRepository
}

func NewNotificationDeliveryService(repo *NotificationRepository) *NotificationDeliveryService {
	return &NotificationDeliveryService{repo: repo}
}

func (s *NotificationDeliveryService) List(ctx context.Context, input json.RawMessage) (any, error) {
	if _, err := requireTenant(ctx); err != nil {
		return nil, err
	}
	var req struct {
		Current int `json:"current"`
		Size    int `json:"size"`
	}
	_ = json.Unmarshal(input, &req)
	items, total, err := s.repo.ListFailedDeliveries(ctx, req.Current, req.Size)
	if err != nil {
		return nil, err
	}
	return map[string]any{"items": items, "total": total}, nil
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
	deliverySvc := NewNotificationDeliveryService(repo)

	auditOn := true
	auditOff := false

	// User notification actions (permission: system.notification_user)
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

	// Admin notification management actions (permission: system.notification_management)
	adminSpecs := []struct {
		id, path, name, nameKey, desc, descKey string
		audit                                  *bool
		handler                                func(context.Context, json.RawMessage) (any, error)
	}{
		{ActionNotificationTemplateList, "/api/v1/system/notification/templates", "模板覆盖列表", "action.system.notification_template_list.name", "列出租户模板覆盖", "action.system.notification_template_list.description", &auditOff, tplSvc.List},
		{ActionNotificationTemplateUpdate, "/api/v1/system/notification/templates/update", "更新模板覆盖", "action.system.notification_template_update.name", "更新模板覆盖内容", "action.system.notification_template_update.description", &auditOn, tplSvc.Update},
		{ActionNotificationTemplateReset, "/api/v1/system/notification/templates/reset", "重置模板覆盖", "action.system.notification_template_reset.name", "重置为代码默认模板", "action.system.notification_template_reset.description", &auditOn, tplSvc.Reset},
		{ActionNotificationDeliveryList, "/api/v1/system/notification/deliveries", "Delivery 列表", "action.system.notification_delivery_list.name", "查询失败投递记录", "action.system.notification_delivery_list.description", &auditOff, deliverySvc.List},
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
				PermissionGroup:        "system.notification_management",
				PermissionGroupName:    "通知管理",
				PermissionGroupNameKey: "permission_group.system.notification_management",
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

	return nil
}

// Ensure auth is used
var _ = auth.MustFromContext

// Ensure gorm is used
var _ = gorm.ErrRecordNotFound
