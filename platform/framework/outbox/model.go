package outbox

import (
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/status"
)

// Event is the PostgreSQL outbox row (transaction fact source).
type Event struct {
	ID               string              `gorm:"primaryKey;size:64"`
	TenantID         string              `gorm:"size:64;not null;index:idx_outbox_tenant"`
	Realm            string              `gorm:"size:32;not null"`
	EventType        string              `gorm:"size:128;not null;index:idx_outbox_event_type"`
	SchemaVersion    int                 `gorm:"not null"`
	AggregateType    string              `gorm:"size:128;index:idx_outbox_aggregate"`
	AggregateID      string              `gorm:"size:128;index:idx_outbox_aggregate"`
	AggregateVersion int64               `gorm:"not null;default:0"`
	PayloadJSON      []byte              `gorm:"type:jsonb;not null"`
	MetadataJSON     []byte              `gorm:"type:jsonb"`
	Status           status.OutboxStatus `gorm:"size:32;not null;index:idx_outbox_claim,priority:1"`
	Attempts         int                 `gorm:"not null;default:0"`
	AvailableAt      time.Time           `gorm:"not null;index:idx_outbox_claim,priority:2"`
	LockedBy         string              `gorm:"size:128"`
	LockedAt         *time.Time
	OccurredAt       time.Time `gorm:"not null"`
	PublishedAt      *time.Time
	CorrelationID    string `gorm:"size:64"`
	CausationID      string `gorm:"size:64"`
	ActorID          string `gorm:"size:64"`
	LastError        string `gorm:"size:512"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

func (Event) TableName() string { return "outbox_events" }

// Delivery records per-handler idempotent processing state.
type Delivery struct {
	ID          string                `gorm:"primaryKey;size:64"`
	EventID     string                `gorm:"size:64;not null;uniqueIndex:ux_event_handler"`
	HandlerName string                `gorm:"size:128;not null;uniqueIndex:ux_event_handler"`
	Status      status.DeliveryStatus `gorm:"size:32;not null"`
	Attempts    int                   `gorm:"not null;default:0"`
	ProcessedAt *time.Time
	LastError   string `gorm:"size:512"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (Delivery) TableName() string { return "event_deliveries" }
