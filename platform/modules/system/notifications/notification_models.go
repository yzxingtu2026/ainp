package notifications

import (
	"database/sql"
	"time"
)

// NotificationRequest represents a notification request row.
type NotificationRequest struct {
	ID             string `gorm:"primaryKey;size:64"`
	TenantID       string `gorm:"size:64;not null;index:idx_notif_req_tenant"`
	DefinitionKey  string `gorm:"size:256;not null"`
	Category       string `gorm:"size:128;not null;default:''"`
	Priority       string `gorm:"size:32;not null;default:'normal'"`
	IdempotencyKey string `gorm:"size:256;not null;default:'';uniqueIndex:uq_notif_req_idem,where:idempotency_key != ''"`
	VariablesJSON  []byte `gorm:"type:jsonb;not null;default:'{}'"`
	ActorID        string `gorm:"size:64;not null;default:''"`
	CorrelationID  string `gorm:"size:64;not null;default:''"`
	CausationID    string `gorm:"size:64;not null;default:''"`
	CreatedAt      time.Time
}

func (NotificationRequest) TableName() string { return "notification_requests" }

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
	TenantID   string `gorm:"size:64;not null;index:idx_user_notif_user"`
	UserID     string `gorm:"size:64;not null;index:idx_user_notif_user"`
	RequestID  string `gorm:"size:64;not null"`
	DeliveryID string `gorm:"size:64;not null;uniqueIndex:uq_user_notif_delivery_user"`
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
