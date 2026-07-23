package audit

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Record struct {
	ID               string    `gorm:"primaryKey;size:64" json:"id"`
	OccurredAt       time.Time `gorm:"index" json:"occurred_at"`
	TenantID         string    `gorm:"size:64;index" json:"tenant_id"`
	Realm            string    `gorm:"size:32;index" json:"realm"`
	UserID           string    `gorm:"size:64;index" json:"user_id"`
	ActorType        string    `gorm:"size:32" json:"actor_type"`
	ActionID         string    `gorm:"size:128;index" json:"action_id"`
	Result           string    `gorm:"size:16" json:"result"`
	RequestID        string    `gorm:"size:64;index" json:"request_id"`
	TraceID          *string   `gorm:"size:64;index" json:"trace_id,omitempty"`
	AgentRunID       *string   `gorm:"size:64;index" json:"agent_run_id,omitempty"`
	ToolInvocationID *string   `gorm:"size:64;index" json:"tool_invocation_id,omitempty"`
	Permission       string    `gorm:"size:128" json:"permission"`
	ErrorCode        int       `json:"error_code"`
	ResourceType     string    `gorm:"size:64" json:"resource_type"`
	ResourceID       string    `gorm:"size:64" json:"resource_id"`
	Client           string    `gorm:"size:32" json:"client"`
	MCPClientID      string    `gorm:"size:64" json:"mcp_client_id,omitempty"`
	MCPGrantID       string    `gorm:"size:64" json:"mcp_grant_id,omitempty"`
	Summary          string    `gorm:"size:512" json:"summary"`
	Replay           bool      `gorm:"default:false" json:"replay"`
}

func (Record) TableName() string { return "audit_logs" }

type Recorder interface {
	Write(ctx context.Context, rec Record) error
}

type GormRecorder struct {
	db *gorm.DB
}

func NewGormRecorder(db *gorm.DB) *GormRecorder {
	return &GormRecorder{db: db}
}

func (r *GormRecorder) AutoMigrate() error {
	return r.db.AutoMigrate(&Record{})
}

func (r *GormRecorder) Write(ctx context.Context, rec Record) error {
	if rec.ID == "" {
		rec.ID = uuid.NewString()
	}
	if rec.OccurredAt.IsZero() {
		rec.OccurredAt = time.Now().UTC()
	}
	return r.db.WithContext(ctx).Create(&rec).Error
}
