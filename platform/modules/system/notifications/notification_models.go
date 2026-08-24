package notifications

import (
	"database/sql"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/auth"
)

// NotificationRequest represents a notification request row.
type NotificationRequest struct {
	ID             string    `gorm:"primaryKey;size:64"`
	TenantID       string    `gorm:"size:64;not null;index:idx_notif_req_tenant;index:idx_notification_requests_principal,priority:1;uniqueIndex:uq_notif_req_idem,priority:1,where:idempotency_key != ''"`
	DefinitionKey  string    `gorm:"size:256;not null;uniqueIndex:uq_notif_req_idem,priority:2,where:idempotency_key != ''"`
	Category       string    `gorm:"size:128;not null;default:''"`
	Priority       string    `gorm:"size:32;not null;default:'normal'"`
	IdempotencyKey string    `gorm:"size:256;not null;default:'';uniqueIndex:uq_notif_req_idem,priority:7,where:idempotency_key != ''"`
	VariablesJSON  []byte    `gorm:"type:jsonb;not null;default:'{}'"`
	ActorID        string    `gorm:"size:64;not null;default:'';index:idx_notification_requests_principal,priority:3;uniqueIndex:uq_notif_req_idem,priority:4,where:idempotency_key != ''"`
	PrincipalKind  string    `gorm:"size:128;not null;default:'ainp.account';index:idx_notification_requests_principal,priority:2;uniqueIndex:uq_notif_req_idem,priority:3,where:idempotency_key != ''"`
	ClientSurface  string    `gorm:"size:128;not null;default:'ainp.tenant-console';index:idx_notification_requests_principal,priority:4;uniqueIndex:uq_notif_req_idem,priority:5,where:idempotency_key != ''"`
	Audience       string    `gorm:"size:255;not null;default:'ainp:tenant-console';uniqueIndex:uq_notif_req_idem,priority:6,where:idempotency_key != ''"`
	CorrelationID  string    `gorm:"size:64;not null;default:''"`
	CausationID    string    `gorm:"size:64;not null;default:''"`
	CreatedAt      time.Time `gorm:"index:idx_notification_requests_principal,priority:5,sort:desc"`
}

func (NotificationRequest) TableName() string { return "notification_requests" }

func (r NotificationRequest) PrincipalRef() auth.PrincipalRef {
	return auth.PrincipalRef{Kind: auth.PrincipalKind(r.PrincipalKind), ID: r.ActorID}
}

// NotificationDelivery represents a notification delivery row.
type NotificationDelivery struct {
	ID              string `gorm:"primaryKey;size:64"`
	TenantID        string `gorm:"size:64;not null;index:idx_notif_del_tenant"`
	RequestID       string `gorm:"size:64;not null;index:idx_notif_del_request"`
	RecipientType   string `gorm:"size:32;not null;default:'user'"`
	RecipientID     string `gorm:"size:128;not null"`
	Channel         string `gorm:"size:32;not null"`
	Provider        string `gorm:"size:64;not null;default:''"`
	TemplateVersion int    `gorm:"not null;default:0"`
	Locale          string `gorm:"size:16;not null;default:'en'"`
	Status          string `gorm:"size:32;not null;default:'pending'"`
	Attempts        int    `gorm:"not null;default:0"`
	MaxAttempts     int    `gorm:"not null;default:3"`
	ExternalID      string `gorm:"size:256;not null;default:''"`
	ScheduledAt     *time.Time
	SentAt          *time.Time
	DeliveredAt     *time.Time
	FailedAt        *time.Time
	LastError       string `gorm:"size:1024;not null;default:''"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (NotificationDelivery) TableName() string { return "notification_deliveries" }

// NotificationTemplateOverride represents a tenant-level template override row.
type NotificationTemplateOverride struct {
	ID            string `gorm:"primaryKey;size:64"`
	TenantID      string `gorm:"size:64;not null"`
	DefinitionKey string `gorm:"size:256;not null;uniqueIndex:uq_notif_tpl_override"`
	Channel       string `gorm:"size:32;not null;uniqueIndex:uq_notif_tpl_override"`
	Locale        string `gorm:"size:16;not null;uniqueIndex:uq_notif_tpl_override"`
	Version       int    `gorm:"not null;default:1"`
	Subject       string `gorm:"type:text;not null;default:''"`
	Body          string `gorm:"type:text;not null;default:''"`
	ConfigJSON    []byte `gorm:"type:jsonb;not null;default:'{}'"`
	Enabled       bool   `gorm:"not null;default:true"`
	UpdatedBy     string `gorm:"size:64;not null;default:''"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (NotificationTemplateOverride) TableName() string { return "notification_template_overrides" }

// UserNotification represents a user's in-app notification row.
type UserNotification struct {
	ID         string `gorm:"primaryKey;size:64"`
	TenantID   string `gorm:"size:64;not null;index:idx_user_notif_account"`
	AccountID  string `gorm:"size:64;not null;index:idx_user_notif_account"`
	RequestID  string `gorm:"size:64;not null"`
	DeliveryID string `gorm:"size:64;not null;uniqueIndex:uq_user_notif_delivery_account"`
	Title      string `gorm:"type:text;not null;default:''"`
	Summary    string `gorm:"type:text;not null;default:''"`
	Body       string `gorm:"type:text;not null;default:''"`
	Icon       string `gorm:"size:128;not null;default:''"`
	LocalIcon  string `gorm:"size:128;not null;default:''"`
	ActionJSON []byte `gorm:"type:jsonb;not null;default:'{}'"`
	ReadAt     sql.NullTime
	ArchivedAt sql.NullTime
	CreatedAt  time.Time
}

func (UserNotification) TableName() string { return "user_notifications" }
