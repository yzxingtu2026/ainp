package system

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/notification"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"gorm.io/gorm"
)

// NotificationRepository handles notification data persistence.
type NotificationRepository struct {
	db *gorm.DB
}

var notificationOverrideRegistration sync.Once

func NewNotificationRepository(db *gorm.DB) *NotificationRepository {
	notificationOverrideRegistration.Do(func() {
		if err := tenant.DefaultRegistry.Register(NotificationTemplateOverride{}, tenant.ModelOptions{Kind: tenant.TenantOwned, InjectOnCreate: true, SystemAccess: true}); err != nil {
			panic(err)
		}
	})
	return &NotificationRepository{db: db}
}

func (r *NotificationRepository) overrideRepository(ctx context.Context) (*tenant.Repository[NotificationTemplateOverride], error) {
	tdb, err := tenant.TenantDB(ctx, r.db)
	if err != nil {
		return nil, err
	}
	return tenant.NewTenantRepository[NotificationTemplateOverride](tdb)
}

// AutoMigrate runs GORM auto-migration for notification tables.
func (r *NotificationRepository) AutoMigrate() error {
	return r.db.AutoMigrate(
		&NotificationRequest{},
		&NotificationDelivery{},
		&NotificationTemplateOverride{},
		&UserNotification{},
	)
}

// --- Request operations ---

// CreateRequest creates a new notification request.
func (r *NotificationRepository) CreateRequest(ctx context.Context, req *NotificationRequest) error {
	if req.ID == "" {
		req.ID = uuid.NewString()
	}
	if req.CreatedAt.IsZero() {
		req.CreatedAt = time.Now().UTC()
	}
	return r.db.WithContext(ctx).Create(req).Error
}

// GetRequest retrieves a request by ID within a tenant.
func (r *NotificationRepository) GetRequest(ctx context.Context, id string) (*NotificationRequest, error) {
	var req NotificationRequest
	err := r.db.WithContext(ctx).Scopes(tenant.ScopeByTenant(ctx)).
		Where("id = ?", id).First(&req).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &req, nil
}

// GetRequestByIdempotencyKey looks up a request by tenant+definition+idempotency_key.
func (r *NotificationRepository) GetRequestByIdempotencyKey(ctx context.Context, definitionKey, idempotencyKey string) (*NotificationRequest, error) {
	if idempotencyKey == "" {
		return nil, nil
	}
	var req NotificationRequest
	err := r.db.WithContext(ctx).Scopes(tenant.ScopeByTenant(ctx)).
		Where("definition_key = ? AND idempotency_key = ?", definitionKey, idempotencyKey).
		First(&req).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &req, nil
}

// --- Delivery operations ---

// CreateDelivery creates a new notification delivery.
func (r *NotificationRepository) CreateDelivery(ctx context.Context, del *NotificationDelivery) error {
	if del.ID == "" {
		del.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	if del.CreatedAt.IsZero() {
		del.CreatedAt = now
	}
	if del.UpdatedAt.IsZero() {
		del.UpdatedAt = now
	}
	return r.db.WithContext(ctx).Create(del).Error
}

// GetDelivery retrieves a delivery by ID within a tenant.
func (r *NotificationRepository) GetDelivery(ctx context.Context, id string) (*NotificationDelivery, error) {
	var del NotificationDelivery
	err := r.db.WithContext(ctx).Scopes(tenant.ScopeByTenant(ctx)).
		Where("id = ?", id).First(&del).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &del, nil
}

// ListDeliveriesByRequest returns all deliveries for a given request.
func (r *NotificationRepository) ListDeliveriesByRequest(ctx context.Context, requestID string) ([]NotificationDelivery, error) {
	var deliveries []NotificationDelivery
	err := r.db.WithContext(ctx).Scopes(tenant.ScopeByTenant(ctx)).
		Where("request_id = ?", requestID).Order("created_at ASC").Find(&deliveries).Error
	return deliveries, err
}

// UpdateDeliveryStatus updates the delivery status and related fields.
func (r *NotificationRepository) UpdateDeliveryStatus(ctx context.Context, id string, status notification.NotificationDeliveryStatus, externalID, lastError string) error {
	updates := map[string]any{
		"status":     string(status),
		"updated_at": time.Now().UTC(),
		"last_error": lastError,
	}
	if externalID != "" {
		updates["external_id"] = externalID
	}
	switch status {
	case notification.DeliverySending:
		updates["attempts"] = gorm.Expr("attempts + 1")
	case notification.DeliverySent, notification.DeliveryDelivered:
		updates["sent_at"] = time.Now().UTC()
		if status == notification.DeliveryDelivered {
			updates["delivered_at"] = time.Now().UTC()
		}
	case notification.DeliveryFailed, notification.DeliveryDead:
		updates["failed_at"] = time.Now().UTC()
	}
	return r.db.WithContext(ctx).Model(&NotificationDelivery{}).
		Where("id = ?", id).Updates(updates).Error
}

// ListFailedDeliveries returns failed/dead deliveries for admin retry.
func (r *NotificationRepository) ListFailedDeliveries(ctx context.Context, page, pageSize int) ([]NotificationDelivery, int64, error) {
	var total int64
	q := r.db.WithContext(ctx).Scopes(tenant.ScopeByTenant(ctx)).
		Where("status IN ?", []string{string(notification.DeliveryFailed), string(notification.DeliveryDead)})
	q.Model(&NotificationDelivery{}).Count(&total)

	var deliveries []NotificationDelivery
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	err := q.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&deliveries).Error
	return deliveries, total, err
}

// ListDeliveries returns filtered, paginated delivery records for the tenant.
func (r *NotificationRepository) ListDeliveries(ctx context.Context, page, pageSize int, status, channel string) ([]NotificationDelivery, int64, error) {
	var total int64
	q := r.db.WithContext(ctx).Scopes(tenant.ScopeByTenant(ctx))
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if channel != "" {
		q = q.Where("channel = ?", channel)
	}
	q.Model(&NotificationDelivery{}).Count(&total)

	var deliveries []NotificationDelivery
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	err := q.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&deliveries).Error
	return deliveries, total, err
}

// BatchLookupUserNames returns a userID → displayName map for the given IDs.
// Falls back to Username when DisplayName is empty.
func (r *NotificationRepository) BatchLookupUserNames(ctx context.Context, tenantID string, userIDs []string) (map[string]string, error) {
	if len(userIDs) == 0 {
		return map[string]string{}, nil
	}
	var rows []struct {
		ID          string `gorm:"column:id"`
		Username    string `gorm:"column:username"`
		DisplayName string `gorm:"column:display_name"`
	}
	if err := r.db.WithContext(ctx).Table("users").
		Select("id, username, display_name").
		Where("tenant_id = ? AND id IN ?", tenantID, userIDs).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make(map[string]string, len(rows))
	for _, r := range rows {
		name := r.DisplayName
		if name == "" {
			name = r.Username
		}
		result[r.ID] = name
	}
	return result, nil
}

// DeliveryDetailRow is the aggregated result for a single delivery detail query.
type DeliveryDetailRow struct {
	// Delivery fields
	ID            string
	TenantID      string
	RequestID     string
	RecipientType string
	RecipientID   string
	Channel       string
	Provider      string
	Locale        string
	Status        string
	Attempts      int
	MaxAttempts   int
	ExternalID    string
	LastError     string
	SentAt        *time.Time
	DeliveredAt   *time.Time
	FailedAt      *time.Time
	ScheduledAt   *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
	// Request fields
	DefinitionKey string
	VariablesJSON []byte
	// UserNotification rendered content (in_app channel)
	Title   string
	Summary string
	Body    string
	// User display name
	RecipientName string
}

// GetDeliveryDetail returns a single delivery enriched with request content and user name.
func (r *NotificationRepository) GetDeliveryDetail(ctx context.Context, deliveryID string) (*DeliveryDetailRow, error) {
	ac := auth.MustFromContext(ctx)
	if ac == nil || ac.TenantID == "" {
		return nil, fmt.Errorf("tenant context required")
	}
	var row DeliveryDetailRow
	err := r.db.WithContext(ctx).
		Table("notification_deliveries AS d").
		Select(`d.id, d.tenant_id, d.request_id, d.recipient_type, d.recipient_id,
			d.channel, d.provider, d.locale, d.status, d.attempts, d.max_attempts,
			d.external_id, d.last_error, d.sent_at, d.delivered_at, d.failed_at, d.scheduled_at,
			d.created_at, d.updated_at,
			COALESCE(nr.definition_key, '') AS definition_key,
			COALESCE(nr.variables_json, '{}') AS variables_json,
			COALESCE(un.title, '') AS title,
			COALESCE(un.summary, '') AS summary,
			COALESCE(un.body, '') AS body,
			COALESCE(NULLIF(u.display_name, ''), u.username, '') AS recipient_name`).
		Joins("LEFT JOIN notification_requests nr ON nr.id = d.request_id AND nr.tenant_id = d.tenant_id").
		Joins("LEFT JOIN user_notifications un ON un.delivery_id = d.id AND un.tenant_id = d.tenant_id").
		Joins("LEFT JOIN users u ON u.id = d.recipient_id AND u.tenant_id = d.tenant_id").
		Where("d.id = ? AND d.tenant_id = ?", deliveryID, ac.TenantID).
		Scan(&row).Error
	if err != nil {
		return nil, err
	}
	if row.ID == "" {
		return nil, nil
	}
	return &row, nil
}

// --- Template override operations ---

// ListTemplateOverrides returns all overrides for a tenant.
func (r *NotificationRepository) ListTemplateOverrides(ctx context.Context) ([]NotificationTemplateOverride, error) {
	repo, err := r.overrideRepository(ctx)
	if err != nil {
		return nil, err
	}
	overrides, err := repo.Order("definition_key, channel, locale").Find("1=1")
	return overrides, err
}

// GetTemplateOverride returns a specific override.
func (r *NotificationRepository) GetTemplateOverride(ctx context.Context, definitionKey, channel, locale string) (*NotificationTemplateOverride, error) {
	repo, err := r.overrideRepository(ctx)
	if err != nil {
		return nil, err
	}
	o, err := repo.First("definition_key = ? AND channel = ? AND locale = ?", definitionKey, channel, locale)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &o, nil
}

// UpsertTemplateOverride creates or updates a template override.
func (r *NotificationRepository) UpsertTemplateOverride(ctx context.Context, o *NotificationTemplateOverride) error {
	if o.ID == "" {
		o.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	o.UpdatedAt = now
	if o.CreatedAt.IsZero() {
		o.CreatedAt = now
	}

	// Try to find existing override
	existing, err := r.GetTemplateOverride(ctx, o.DefinitionKey, o.Channel, o.Locale)
	if err != nil {
		return err
	}
	if existing != nil {
		repo, repoErr := r.overrideRepository(ctx)
		if repoErr != nil {
			return repoErr
		}
		result := repo.Where("id = ?", existing.ID).Updates(map[string]any{
			"version":     o.Version,
			"subject":     o.Subject,
			"body":        o.Body,
			"config_json": o.ConfigJSON,
			"enabled":     o.Enabled,
			"updated_by":  o.UpdatedBy,
			"updated_at":  now,
		})
		return result.Error
	}
	repo, err := r.overrideRepository(ctx)
	if err != nil {
		return err
	}
	return repo.Create(o)
}

// DeleteTemplateOverride removes a template override (reset to code default).
func (r *NotificationRepository) DeleteTemplateOverride(ctx context.Context, definitionKey, channel, locale string) error {
	repo, err := r.overrideRepository(ctx)
	if err != nil {
		return err
	}
	result := repo.DeleteWhere("definition_key = ? AND channel = ? AND locale = ?", definitionKey, channel, locale)
	return result.Error
}

// --- User notification operations ---

// CreateUserNotification creates an in-app notification for a user.
func (r *NotificationRepository) CreateUserNotification(ctx context.Context, n *UserNotification) error {
	if n.ID == "" {
		n.ID = uuid.NewString()
	}
	if n.CreatedAt.IsZero() {
		n.CreatedAt = time.Now().UTC()
	}
	return r.db.WithContext(ctx).Create(n).Error
}

// ListUserNotifications returns paginated notifications for a user.
func (r *NotificationRepository) ListUserNotifications(ctx context.Context, userID string, unreadOnly, excludeArchived bool, page, pageSize int) ([]UserNotification, int64, error) {
	var total int64
	q := r.db.WithContext(ctx).Scopes(tenant.ScopeByTenant(ctx)).
		Where("user_id = ?", userID)
	if unreadOnly {
		q = q.Where("read_at IS NULL")
	}
	if excludeArchived {
		q = q.Where("archived_at IS NULL")
	}
	q.Model(&UserNotification{}).Count(&total)

	var notifications []UserNotification
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	err := q.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&notifications).Error
	return notifications, total, err
}

// CountUnread returns the unread notification count for a user.
func (r *NotificationRepository) CountUnread(ctx context.Context, userID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Scopes(tenant.ScopeByTenant(ctx)).
		Model(&UserNotification{}).
		Where("user_id = ? AND read_at IS NULL AND archived_at IS NULL", userID).
		Count(&count).Error
	return count, err
}

// MarkRead marks a single notification as read (only if currently unread).
func (r *NotificationRepository) MarkRead(ctx context.Context, userID, notificationID string) error {
	result := r.db.WithContext(ctx).
		Scopes(tenant.ScopeByTenant(ctx)).
		Model(&UserNotification{}).
		Where("id = ? AND user_id = ? AND read_at IS NULL", notificationID, userID).
		Update("read_at", time.Now().UTC())
	if result.Error != nil {
		return result.Error
	}
	return nil
}

// MarkAllRead marks all unread notifications as read for a user.
func (r *NotificationRepository) MarkAllRead(ctx context.Context, userID string) (int64, error) {
	result := r.db.WithContext(ctx).
		Model(&UserNotification{}).
		Scopes(tenant.ScopeByTenant(ctx)).
		Where("user_id = ? AND read_at IS NULL", userID).
		Update("read_at", time.Now().UTC())
	return result.RowsAffected, result.Error
}

// Archive marks a notification as archived.
func (r *NotificationRepository) Archive(ctx context.Context, userID, notificationID string) error {
	return r.db.WithContext(ctx).
		Scopes(tenant.ScopeByTenant(ctx)).
		Model(&UserNotification{}).
		Where("id = ? AND user_id = ? AND archived_at IS NULL", notificationID, userID).
		Update("archived_at", time.Now().UTC()).Error
}

// Unarchive removes the archive mark from a notification.
func (r *NotificationRepository) Unarchive(ctx context.Context, userID, notificationID string) error {
	return r.db.WithContext(ctx).
		Scopes(tenant.ScopeByTenant(ctx)).
		Model(&UserNotification{}).
		Where("id = ? AND user_id = ? AND archived_at IS NOT NULL", notificationID, userID).
		Update("archived_at", sql.NullTime{}).Error
}

// RetryDelivery resets a failed/dead delivery back to pending for retry.
func (r *NotificationRepository) RetryDelivery(ctx context.Context, deliveryID string) error {
	result := r.db.WithContext(ctx).
		Scopes(tenant.ScopeByTenant(ctx)).
		Model(&NotificationDelivery{}).
		Where("id = ? AND status IN ?", deliveryID, []string{string(notification.DeliveryFailed), string(notification.DeliveryDead)}).
		Updates(map[string]any{
			"status":     string(notification.DeliveryPending),
			"attempts":   0,
			"last_error": "",
			"failed_at":  sql.NullTime{},
			"updated_at": time.Now().UTC(),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// --- Helper ---

func tenantIDFromCtx(ctx context.Context) string {
	ac, ok := auth.FromContext(ctx)
	if !ok || ac == nil {
		return ""
	}
	return ac.TenantID
}
