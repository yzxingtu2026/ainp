package ai

import (
	"encoding/json"
	"time"
)

type AgentRun struct {
	ID                   string          `gorm:"primaryKey;size:64" json:"id"`
	TenantID             string          `gorm:"size:64;not null;uniqueIndex:ux_ai_run_idem,priority:1;index:idx_ai_run_list,priority:1" json:"-"`
	UserID               string          `gorm:"size:64;not null;uniqueIndex:ux_ai_run_idem,priority:2;index:idx_ai_run_list,priority:2" json:"user_id"`
	ConversationID       string          `gorm:"size:64;not null;index" json:"conversation_id"`
	ConversationRevision uint64          `gorm:"not null" json:"conversation_revision"`
	TriggerMessageID     string          `gorm:"size:64;not null" json:"trigger_message_id"`
	AgentVersionID       string          `gorm:"size:64;not null" json:"agent_version_id"`
	SessionID            string          `gorm:"size:64" json:"session_id"`
	AuthVersion          uint64          `gorm:"not null" json:"auth_version"`
	OriginRequestID      string          `gorm:"size:64;not null;default:'';index" json:"request_id,omitempty"`
	TraceID              string          `gorm:"size:64;not null;default:'';index" json:"trace_id,omitempty"`
	ContextSnapshotID    *string         `gorm:"size:64" json:"context_snapshot_id,omitempty"`
	EvaluatedAtUTC       *time.Time      `json:"evaluated_at_utc,omitempty"`
	Timezone             string          `gorm:"size:64;not null;default:''" json:"timezone,omitempty"`
	TimeSnapshotJSON     json.RawMessage `gorm:"type:jsonb;not null;default:'{}'" json:"-"`
	Status               string          `gorm:"size:32;not null;index:idx_ai_run_list,priority:3" json:"status"`
	BudgetJSON           json.RawMessage `gorm:"type:jsonb;not null" json:"budget"`
	CurrentStep          int             `gorm:"not null;default:0" json:"current_step"`
	ModelCalls           int             `gorm:"not null;default:0" json:"model_calls"`
	ToolCalls            int             `gorm:"not null;default:0" json:"tool_calls"`
	Compactions          int             `gorm:"not null;default:0" json:"compactions"`
	InputTokens          int64           `gorm:"not null;default:0" json:"input_tokens"`
	OutputTokens         int64           `gorm:"not null;default:0" json:"output_tokens"`
	IdempotencyKey       string          `gorm:"size:191;not null;uniqueIndex:ux_ai_run_idem,priority:3" json:"-"`
	LeaseOwner           string          `gorm:"size:128" json:"-"`
	LeaseExpiresAt       *time.Time      `json:"-"`
	ErrorCategory        string          `gorm:"size:64" json:"error_category,omitempty"`
	Revision             uint64          `gorm:"not null;default:1" json:"revision"`
	CreatedAt            time.Time       `json:"created_at"`
	UpdatedAt            time.Time       `json:"updated_at"`
	StartedAt            *time.Time      `json:"started_at,omitempty"`
	FinishedAt           *time.Time      `json:"finished_at,omitempty"`
}

func (AgentRun) TableName() string { return "ai_agent_runs" }

type AgentStep struct {
	ID                string          `gorm:"primaryKey;size:64" json:"id"`
	TenantID          string          `gorm:"size:64;not null;uniqueIndex:ux_ai_step_index,priority:1" json:"-"`
	RunID             string          `gorm:"size:64;not null;uniqueIndex:ux_ai_step_index,priority:2;index" json:"run_id"`
	StepIndex         int             `gorm:"not null;uniqueIndex:ux_ai_step_index,priority:3" json:"step_index"`
	Type              string          `gorm:"size:32;not null" json:"type"`
	Status            string          `gorm:"size:32;not null" json:"status"`
	ParentStepID      *string         `gorm:"size:64" json:"parent_step_id,omitempty"`
	ModelInvocationID *string         `gorm:"size:64" json:"model_invocation_id,omitempty"`
	ToolInvocationID  *string         `gorm:"size:64" json:"tool_invocation_id,omitempty"`
	ConfirmationID    *string         `gorm:"size:64" json:"confirmation_id,omitempty"`
	ContextSnapshotID *string         `gorm:"size:64" json:"context_snapshot_id,omitempty"`
	InputSummary      json.RawMessage `gorm:"type:jsonb;not null;default:'{}'" json:"input_summary"`
	OutputSummary     json.RawMessage `gorm:"type:jsonb;not null;default:'{}'" json:"output_summary"`
	ErrorCategory     string          `gorm:"size:64" json:"error_category,omitempty"`
	CreatedAt         time.Time       `json:"created_at"`
	StartedAt         *time.Time      `json:"started_at,omitempty"`
	FinishedAt        *time.Time      `json:"finished_at,omitempty"`
}

func (AgentStep) TableName() string { return "ai_agent_steps" }

type AgentRunEvent struct {
	ID        string          `gorm:"primaryKey;size:64" json:"id"`
	TenantID  string          `gorm:"size:64;not null;uniqueIndex:ux_ai_run_event_cursor,priority:1" json:"-"`
	RunID     string          `gorm:"size:64;not null;uniqueIndex:ux_ai_run_event_cursor,priority:2;index" json:"run_id"`
	Sequence  uint64          `gorm:"not null;uniqueIndex:ux_ai_run_event_cursor,priority:3" json:"sequence"`
	Type      string          `gorm:"size:64;not null" json:"type"`
	StepID    *string         `gorm:"size:64" json:"step_id,omitempty"`
	Payload   json.RawMessage `gorm:"type:jsonb;not null;default:'{}'" json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
}

func (AgentRunEvent) TableName() string { return "ai_agent_run_events" }
