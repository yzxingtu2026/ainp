package idempotency

import (
	"time"
)

// Status represents the lifecycle state of an idempotency record.
type Status string

const (
	StatusProcessing Status = "processing"
	StatusCompleted  Status = "completed"
	StatusFailed     Status = "failed"
	StatusConflict   Status = "conflict"
)

// Record is the GORM model for idempotency_records.
type Record struct {
	ID             string     `gorm:"primaryKey;size:64" json:"id"`
	TenantID       string     `gorm:"size:64;uniqueIndex:ux_idempotency_key;not null" json:"tenant_id"`
	UserID         string     `gorm:"size:64;uniqueIndex:ux_idempotency_key;not null" json:"user_id"`
	ActionID       string     `gorm:"size:128;uniqueIndex:ux_idempotency_key;not null" json:"action_id"`
	IdempotencyKey string     `gorm:"size:191;uniqueIndex:ux_idempotency_key;not null" json:"idempotency_key"`
	RequestHash    string     `gorm:"size:64;not null" json:"request_hash"`
	Status         Status     `gorm:"size:32;not null" json:"status"`
	ResponseJSON   []byte     `gorm:"type:jsonb" json:"response_json,omitempty"`
	ErrorJSON      []byte     `gorm:"type:jsonb" json:"error_json,omitempty"`
	StartedAt      time.Time  `gorm:"not null" json:"started_at"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
	ExpiresAt      time.Time  `gorm:"index;not null" json:"expires_at"`
	CreatedAt      time.Time  `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt      time.Time  `gorm:"autoUpdateTime" json:"updated_at"`
}

func (Record) TableName() string { return "idempotency_records" }
