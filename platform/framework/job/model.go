package job

import (
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/status"
)

// Run is the PostgreSQL fact record for a background job (tenant-visible).
type Run struct {
	ID                string              `gorm:"primaryKey;size:64" json:"id"`
	TenantID          string              `gorm:"size:64;not null;index:idx_job_runs_tenant" json:"tenant_id"`
	Realm             string              `gorm:"size:32;not null" json:"realm"`
	JobType           string              `gorm:"size:128;not null;index:idx_job_runs_type" json:"job_type"`
	SchemaVersion     int                 `gorm:"not null" json:"schema_version"`
	Queue             string              `gorm:"size:32;not null" json:"queue"`
	PayloadJSON       []byte              `gorm:"type:jsonb;not null" json:"-"`
	Status            status.JobRunStatus `gorm:"size:32;not null;index:idx_job_runs_status" json:"status"`
	Progress          int                 `gorm:"not null;default:0" json:"progress"`
	ProgressMessage   string              `gorm:"size:256" json:"progress_message,omitempty"`
	Attempt           int                 `gorm:"not null;default:0" json:"attempt"`
	MaxAttempts       int                 `gorm:"not null;default:1" json:"max_attempts"`
	IdempotencyKey    string              `gorm:"size:191;index:idx_job_runs_idem,priority:3" json:"idempotency_key,omitempty"`
	CorrelationID     string              `gorm:"size:64" json:"correlation_id,omitempty"`
	CausationID       string              `gorm:"size:64" json:"causation_id,omitempty"`
	CreatedBy         string              `gorm:"size:64" json:"created_by,omitempty"`
	StartedAt         *time.Time          `json:"started_at,omitempty"`
	FinishedAt        *time.Time          `json:"finished_at,omitempty"`
	CancelRequestedAt *time.Time          `json:"cancel_requested_at,omitempty"`
	LastError         string              `gorm:"size:512" json:"last_error,omitempty"`
	ResultRef         string              `gorm:"size:512" json:"result_ref,omitempty"`
	CreatedAt         time.Time           `json:"created_at"`
	UpdatedAt         time.Time           `json:"updated_at"`
}

func (Run) TableName() string { return "job_runs" }
