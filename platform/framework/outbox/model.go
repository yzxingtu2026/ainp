package outbox

import (
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/status"
)

// Event is the PostgreSQL outbox row (transaction fact source).
type Event struct {
	ID               string              `gorm:"primaryKey;size:64" json:"id"`
	TenantID         string              `gorm:"size:64;not null;index:idx_outbox_tenant" json:"-"`
	Realm            string              `gorm:"size:32;not null" json:"-"`
	EventType        string              `gorm:"size:128;not null;index:idx_outbox_event_type" json:"event_type"`
	SchemaVersion    int                 `gorm:"not null" json:"schema_version"`
	AggregateType    string              `gorm:"size:128;index:idx_outbox_aggregate" json:"aggregate_type"`
	AggregateID      string              `gorm:"size:128;index:idx_outbox_aggregate" json:"aggregate_id"`
	AggregateVersion int64               `gorm:"not null;default:0" json:"aggregate_version"`
	PayloadJSON      []byte              `gorm:"type:jsonb;not null" json:"-"`
	MetadataJSON     []byte              `gorm:"type:jsonb" json:"-"`
	Status           status.OutboxStatus `gorm:"size:32;not null;index:idx_outbox_claim,priority:1" json:"status"`
	Attempts         int                 `gorm:"not null;default:0" json:"attempts"`
	AvailableAt      time.Time           `gorm:"not null;index:idx_outbox_claim,priority:2" json:"available_at"`
	LockedBy         string              `gorm:"size:128" json:"-"`
	LockedAt         *time.Time          `json:"-"`
	OccurredAt       time.Time           `gorm:"not null" json:"occurred_at"`
	PublishedAt      *time.Time          `json:"published_at,omitempty"`
	CorrelationID    string              `gorm:"size:64" json:"correlation_id,omitempty"`
	CausationID      string              `gorm:"size:64" json:"causation_id,omitempty"`
	ActorID          string              `gorm:"size:64" json:"actor_id,omitempty"`
	LastError        string              `gorm:"size:512" json:"last_error,omitempty"`
	CreatedAt        time.Time           `json:"created_at"`
	UpdatedAt        time.Time           `json:"updated_at"`
}

func (Event) TableName() string { return "outbox_events" }

// Delivery records per-handler idempotent processing state.
type Delivery struct {
	ID          string                `gorm:"primaryKey;size:64" json:"id"`
	EventID     string                `gorm:"size:64;not null;uniqueIndex:ux_event_handler" json:"event_id"`
	HandlerName string                `gorm:"size:128;not null;uniqueIndex:ux_event_handler" json:"handler_name"`
	Status      status.DeliveryStatus `gorm:"size:32;not null" json:"status"`
	Attempts    int                   `gorm:"not null;default:0" json:"attempts"`
	ProcessedAt *time.Time            `json:"processed_at,omitempty"`
	LastError   string                `gorm:"size:512" json:"last_error,omitempty"`
	CreatedAt   time.Time             `json:"created_at"`
	UpdatedAt   time.Time             `json:"updated_at"`
}

func (Delivery) TableName() string { return "event_deliveries" }
