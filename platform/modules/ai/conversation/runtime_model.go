package conversation

import (
	"encoding/json"
	"time"
)

type AgentRun struct {
	ID                             string          `gorm:"primaryKey;size:64;uniqueIndex:ux_ai_agent_runs_tenant_id,priority:2" json:"id"`
	TenantID                       string          `gorm:"size:64;not null;uniqueIndex:ux_ai_agent_runs_tenant_id,priority:1;uniqueIndex:ux_ai_run_idem,priority:1;index:idx_ai_run_list,priority:1" json:"-"`
	UserID                         string          `gorm:"size:64;not null;uniqueIndex:ux_ai_run_idem,priority:2;index:idx_ai_run_list,priority:2" json:"user_id"`
	PrincipalKind                  string          `gorm:"size:128;not null;default:'ainp.account';uniqueIndex:ux_ai_run_idem,priority:4" json:"-"`
	ClientSurface                  string          `gorm:"size:128;not null;default:''" json:"-"`
	Audience                       string          `gorm:"size:255;not null;default:''" json:"-"`
	ConversationID                 string          `gorm:"size:64;not null;index" json:"conversation_id"`
	ConversationRevision           uint64          `gorm:"not null" json:"conversation_revision"`
	TriggerMessageID               string          `gorm:"size:64;not null" json:"trigger_message_id"`
	AgentVersionID                 string          `gorm:"size:64;not null" json:"agent_version_id"`
	SessionID                      string          `gorm:"size:64" json:"session_id"`
	AuthVersion                    uint64          `gorm:"not null" json:"auth_version"`
	OriginRequestID                string          `gorm:"size:64;not null;default:'';index" json:"request_id,omitempty"`
	TraceID                        string          `gorm:"size:64;not null;default:'';index" json:"trace_id,omitempty"`
	PageContextJSON                json.RawMessage `gorm:"type:jsonb;not null;default:'{}'" json:"-"`
	PageContextType                string          `gorm:"size:64;not null;default:'';index" json:"page_context_type,omitempty"`
	PageContextStatus              string          `gorm:"size:16;not null;default:'none'" json:"page_context_status"`
	PageContextBytes               int             `gorm:"not null;default:0" json:"page_context_bytes"`
	ContextSnapshotID              *string         `gorm:"size:64" json:"context_snapshot_id,omitempty"`
	EvaluatedAtUTC                 *time.Time      `json:"evaluated_at_utc,omitempty"`
	Timezone                       string          `gorm:"size:64;not null;default:''" json:"timezone,omitempty"`
	TimeSnapshotJSON               json.RawMessage `gorm:"type:jsonb;not null;default:'{}'" json:"-"`
	Status                         string          `gorm:"size:32;not null;index:idx_ai_run_list,priority:3" json:"status"`
	ExecutionMode                  string          `gorm:"size:32;not null;default:'normal'" json:"execution_mode"`
	CompletionReason               string          `gorm:"size:64;not null;default:''" json:"completion_reason,omitempty"`
	CompletionAttempted            bool            `gorm:"not null;default:false" json:"completion_attempted"`
	TerminalProtocolVersion        string          `gorm:"size:16;not null;default:'v1'" json:"terminal_protocol_version,omitempty"`
	TerminalReason                 string          `gorm:"size:64;not null;default:''" json:"terminal_reason,omitempty"`
	FinalizationMode               string          `gorm:"size:16;not null;default:'none'" json:"finalization_mode,omitempty"`
	FinalizationAttempted          bool            `gorm:"not null;default:false" json:"finalization_attempted"`
	Retryable                      bool            `gorm:"not null;default:false" json:"retryable"`
	UserMessageKey                 string          `gorm:"size:128;not null;default:''" json:"user_message_key,omitempty"`
	SafeArgsJSON                   json.RawMessage `gorm:"column:safe_args;type:jsonb;not null;default:'{}'" json:"safe_args,omitempty"`
	SafeSummary                    string          `gorm:"size:255;not null;default:''" json:"safe_summary,omitempty"`
	TerminalMessageID              *string         `gorm:"size:64" json:"terminal_message_id,omitempty"`
	CompletedOperationCount        int             `gorm:"not null;default:0" json:"completed_operation_count"`
	FailedOperationCount           int             `gorm:"not null;default:0" json:"failed_operation_count"`
	UnfinishedOperationCount       int             `gorm:"not null;default:0" json:"unfinished_operation_count"`
	CompletionReservedInputTokens  int64           `gorm:"not null;default:0" json:"completion_reserved_input_tokens"`
	CompletionReservedOutputTokens int64           `gorm:"not null;default:0" json:"completion_reserved_output_tokens"`
	CompletionSafetyMarginTokens   int64           `gorm:"not null;default:0" json:"completion_safety_margin_tokens"`
	BudgetJSON                     json.RawMessage `gorm:"type:jsonb;not null" json:"budget"`
	CurrentStep                    int             `gorm:"not null;default:0" json:"current_step"`
	ModelCalls                     int             `gorm:"not null;default:0" json:"model_calls"`
	ToolCalls                      int             `gorm:"not null;default:0" json:"tool_calls"`
	Compactions                    int             `gorm:"not null;default:0" json:"compactions"`
	InputTokens                    int64           `gorm:"not null;default:0" json:"input_tokens"`
	OutputTokens                   int64           `gorm:"not null;default:0" json:"output_tokens"`
	IdempotencyKey                 string          `gorm:"size:191;not null;uniqueIndex:ux_ai_run_idem,priority:3" json:"-"`
	LeaseOwner                     string          `gorm:"size:128" json:"-"`
	LeaseExpiresAt                 *time.Time      `json:"-"`
	ErrorCategory                  string          `gorm:"size:64" json:"error_category,omitempty"`
	Revision                       uint64          `gorm:"not null;default:1" json:"revision"`
	CreatedAt                      time.Time       `json:"created_at"`
	UpdatedAt                      time.Time       `json:"updated_at"`
	StartedAt                      *time.Time      `json:"started_at,omitempty"`
	FinishedAt                     *time.Time      `json:"finished_at,omitempty"`
}

func (AgentRun) TableName() string { return "ai_agent_runs" }

type AgentStep struct {
	ID                string          `gorm:"primaryKey;size:64;uniqueIndex:ux_ai_agent_steps_tenant_id,priority:2" json:"id"`
	TenantID          string          `gorm:"size:64;not null;uniqueIndex:ux_ai_agent_steps_tenant_id,priority:1;uniqueIndex:ux_ai_step_index,priority:1" json:"-"`
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

const (
	PageCapturePending   = "pending"
	PageCaptureSubmitted = "submitted"
	PageCaptureConsumed  = "consumed"
	PageCaptureFailed    = "failed"
	PageCaptureExpired   = "expired"
	PageCaptureCancelled = "cancelled"
)

type PageCaptureRequest struct {
	ID                 string          `gorm:"primaryKey;size:64;uniqueIndex:ux_ai_page_capture_requests_tenant_id,priority:2" json:"id"`
	TenantID           string          `gorm:"size:64;not null;uniqueIndex:ux_ai_page_capture_requests_tenant_id,priority:1;uniqueIndex:ux_ai_page_capture_requests_invocation,priority:1;index:idx_ai_page_capture_requests_run_status,priority:1" json:"-"`
	PrincipalID        string          `gorm:"size:64;not null;index" json:"-"`
	PrincipalKind      string          `gorm:"size:128;not null;default:'ainp.account'" json:"-"`
	ClientSurface      string          `gorm:"size:128;not null;default:''" json:"-"`
	Audience           string          `gorm:"size:255;not null;default:''" json:"-"`
	SessionID          string          `gorm:"size:64;not null;default:''" json:"-"`
	AuthVersion        uint64          `gorm:"not null" json:"-"`
	RunID              string          `gorm:"size:64;not null;index:idx_ai_page_capture_requests_run_status,priority:2" json:"run_id"`
	StepID             string          `gorm:"size:64;not null" json:"step_id"`
	ScopeID            string          `gorm:"size:64;not null" json:"scope_id"`
	InvocationID       string          `gorm:"size:64;not null;uniqueIndex:ux_ai_page_capture_requests_invocation,priority:2" json:"invocation_id"`
	ToolCallID         string          `gorm:"size:128;not null" json:"tool_call_id"`
	ToolID             string          `gorm:"size:255;not null" json:"tool_id"`
	ParametersJSON     json.RawMessage `gorm:"type:jsonb;not null;default:'{}'" json:"parameters,omitempty"`
	Status             string          `gorm:"size:24;not null;index:idx_ai_page_capture_requests_run_status,priority:3;check:ck_ai_page_capture_requests_status,status IN ('pending','submitted','consumed','failed','expired','cancelled')" json:"status"`
	AttachmentID       *string         `gorm:"size:64" json:"attachment_id,omitempty"`
	AttachmentRevision string          `gorm:"size:128;not null;default:''" json:"attachment_revision,omitempty"`
	ErrorCategory      string          `gorm:"size:64;not null;default:''" json:"error_category,omitempty"`
	ExpiresAt          time.Time       `gorm:"not null" json:"expires_at"`
	SubmittedAt        *time.Time      `json:"submitted_at,omitempty"`
	ConsumedAt         *time.Time      `json:"consumed_at,omitempty"`
	CreatedAt          time.Time       `gorm:"index:idx_ai_page_capture_requests_run_status,priority:4" json:"created_at"`
	UpdatedAt          time.Time       `json:"updated_at"`
}

func (PageCaptureRequest) TableName() string { return "ai_page_capture_requests" }
