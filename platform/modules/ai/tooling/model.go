package tooling

import (
	"encoding/json"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/aitool"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	"gorm.io/gorm"
)

type PGVector = database.PGVector

type ToolCatalog struct {
	ID            string          `gorm:"primaryKey;size:64"`
	TenantID      string          `gorm:"size:64;not null;uniqueIndex:ux_ai_tool_catalog_tenant_tool"`
	ToolID        string          `gorm:"size:255;not null;uniqueIndex:ux_ai_tool_catalog_tenant_tool"`
	Source        string          `gorm:"size:32;not null"`
	Name          string          `gorm:"size:128;not null"`
	Capability    string          `gorm:"size:64;not null"`
	Summary       string          `gorm:"type:text;not null"`
	TagsJSON      json.RawMessage `gorm:"type:jsonb;not null;default:'[]'"`
	Risk          string          `gorm:"size:16;not null"`
	SideEffect    string          `gorm:"size:16;not null"`
	SchemaVersion int             `gorm:"not null"`
	SchemaHash    string          `gorm:"size:64;not null"`
	Status        string          `gorm:"size:16;not null"`
	Revision      uint64          `gorm:"not null;default:1"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (ToolCatalog) TableName() string { return "ai_tool_catalog" }

type ToolSearchDocument struct {
	ID                     string             `gorm:"primaryKey;size:64"`
	TenantID               string             `gorm:"size:64;not null;uniqueIndex:ux_ai_tool_search_tenant_tool"`
	ToolID                 string             `gorm:"size:255;not null;uniqueIndex:ux_ai_tool_search_tenant_tool"`
	SearchText             string             `gorm:"type:text;not null"`
	InputHintsJSON         json.RawMessage    `gorm:"type:jsonb;not null;default:'[]'"`
	IndexStatus            string             `gorm:"size:16;not null"`
	ErrorCategory          string             `gorm:"size:64;not null;default:''"`
	ErrorStage             string             `gorm:"size:64;not null;default:''"`
	CatalogRevision        uint64             `gorm:"not null;default:1"`
	EmbeddingModel         string             `gorm:"size:128"`
	EmbeddingAlias         string             `gorm:"size:64"`
	EmbeddingDeploymentID  string             `gorm:"size:64"`
	EmbeddingDimensions    int                `gorm:"not null;default:0"`
	EmbeddingPreprocessing string             `gorm:"size:32"`
	EmbeddingSchemaVersion int                `gorm:"not null;default:0"`
	EmbeddingVector        *database.PGVector `gorm:"type:public.vector" json:"-"`
	VectorUpdatedAt        *time.Time
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

func (ToolSearchDocument) TableName() string { return "ai_tool_search_documents" }

type ToolScope struct {
	ID                      string          `gorm:"primaryKey;size:64" json:"id"`
	TenantID                string          `gorm:"size:64;not null;index" json:"-"`
	PrincipalID             string          `gorm:"size:64;not null;index" json:"-"`
	Realm                   string          `gorm:"size:16;not null" json:"realm"`
	SessionID               string          `gorm:"size:64;not null" json:"-"`
	AuthVersion             uint64          `gorm:"not null" json:"-"`
	AgentRunID              *string         `gorm:"size:64" json:"agent_run_id,omitempty"`
	ConversationID          *string         `gorm:"size:64;index" json:"conversation_id,omitempty"`
	AllowedSourcesJSON      json.RawMessage `gorm:"type:jsonb;not null;default:'[]'" json:"-"`
	AllowedCapabilitiesJSON json.RawMessage `gorm:"type:jsonb;not null;default:'[]'" json:"-"`
	AllowedToolsJSON        json.RawMessage `gorm:"type:jsonb;not null;default:'[]'" json:"-"`
	LoadedToolsJSON         json.RawMessage `gorm:"type:jsonb;not null;default:'[]'" json:"loaded_tool_ids"`
	ReferenceEvidenceJSON   json.RawMessage `gorm:"type:jsonb;not null;default:'[]'" json:"-"`
	SchemaBytes             int             `gorm:"not null;default:0" json:"schema_bytes"`
	ExpiresAt               time.Time       `gorm:"not null;index" json:"expires_at"`
	CreatedAt               time.Time       `json:"created_at"`
	UpdatedAt               time.Time       `json:"updated_at"`
}

func (ToolScope) TableName() string { return "ai_tool_scopes" }

type ConversationToolWorkingSet struct {
	ID             string     `gorm:"primaryKey;size:64" json:"id"`
	TenantID       string     `gorm:"size:64;not null;uniqueIndex:ux_conversation_tool_working_set,priority:1" json:"-"`
	ConversationID string     `gorm:"size:64;not null;uniqueIndex:ux_conversation_tool_working_set,priority:2;index" json:"conversation_id"`
	ToolID         string     `gorm:"size:255;not null;uniqueIndex:ux_conversation_tool_working_set,priority:3" json:"tool_id"`
	SchemaHash     string     `gorm:"size:64;not null" json:"schema_hash"`
	ActivatedAt    time.Time  `gorm:"not null" json:"activated_at"`
	LastUsedAt     *time.Time `json:"last_used_at,omitempty"`
	UseCount       uint64     `gorm:"not null;default:0" json:"use_count"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func (ConversationToolWorkingSet) TableName() string { return "ai_conversation_tool_working_set" }

const (
	ToolConfirmationPending   = "pending"
	ToolConfirmationApproved  = "approved"
	ToolConfirmationRejected  = "rejected"
	ToolConfirmationExpired   = "expired"
	ToolConfirmationConsumed  = "consumed"
	ToolConfirmationCancelled = "cancelled"

	ToolInvocationRunning             = "running"
	ToolInvocationWaitingConfirmation = "waiting_confirmation"
	ToolInvocationCompleted           = "completed"
	ToolInvocationFailed              = "failed"
)

type ToolConfirmation struct {
	ID                   string                           `gorm:"primaryKey;size:64" json:"id"`
	TenantID             string                           `gorm:"size:64;not null;index" json:"-"`
	PrincipalID          string                           `gorm:"size:64;not null;index" json:"-"`
	ScopeID              string                           `gorm:"size:64;not null;index" json:"scope_id"`
	InvocationID         string                           `gorm:"size:64;not null;index" json:"invocation_id"`
	ToolCallID           string                           `gorm:"size:128;not null" json:"tool_call_id"`
	ToolID               string                           `gorm:"size:255;not null" json:"tool_id"`
	SchemaHash           string                           `gorm:"size:64;not null" json:"schema_hash"`
	ArgumentsHash        string                           `gorm:"size:64;not null" json:"arguments_hash"`
	ArgumentsJSON        json.RawMessage                  `gorm:"type:jsonb;not null" json:"-"`
	ArgumentsSummaryJSON json.RawMessage                  `gorm:"type:jsonb;not null" json:"arguments_summary"`
	PresentationJSON     json.RawMessage                  `gorm:"type:jsonb" json:"-"`
	PresentationVersion  int                              `gorm:"not null;default:0" json:"presentation_version"`
	Presentation         *aitool.ConfirmationPresentation `gorm:"-" json:"presentation,omitempty"`
	Risk                 string                           `gorm:"size:16;not null" json:"risk"`
	SideEffect           string                           `gorm:"size:16;not null" json:"side_effect"`
	Status               string                           `gorm:"size:16;not null;index" json:"status"`
	ExpiresAt            time.Time                        `gorm:"not null" json:"expires_at"`
	DecidedAt            *time.Time                       `json:"decided_at,omitempty"`
	ConsumedAt           *time.Time                       `json:"consumed_at,omitempty"`
	CreatedAt            time.Time                        `json:"created_at"`
	UpdatedAt            time.Time                        `json:"updated_at"`
}

func (ToolConfirmation) TableName() string { return "ai_tool_confirmations" }

func (c *ToolConfirmation) AfterFind(*gorm.DB) error {
	if len(c.PresentationJSON) == 0 || string(c.PresentationJSON) == "null" {
		return nil
	}
	var presentation aitool.ConfirmationPresentation
	if err := json.Unmarshal(c.PresentationJSON, &presentation); err != nil {
		return nil
	}
	c.Presentation = &presentation
	return nil
}

type ToolInvocation struct {
	ID             string          `gorm:"primaryKey;size:64" json:"id"`
	TenantID       string          `gorm:"size:64;not null;index" json:"-"`
	PrincipalID    string          `gorm:"size:64;not null;index" json:"-"`
	ScopeID        string          `gorm:"size:64;not null;index" json:"scope_id"`
	AgentRunID     *string         `gorm:"size:64" json:"agent_run_id,omitempty"`
	ToolCallID     string          `gorm:"size:128;not null" json:"tool_call_id"`
	ToolID         string          `gorm:"size:255;not null" json:"tool_id"`
	Source         string          `gorm:"size:32;not null" json:"source"`
	TargetAction   string          `gorm:"size:255" json:"target_action,omitempty"`
	Risk           string          `gorm:"size:16;not null" json:"risk"`
	SideEffect     string          `gorm:"size:16;not null" json:"side_effect"`
	Status         string          `gorm:"size:24;not null;index" json:"status"`
	StartedAt      time.Time       `gorm:"not null" json:"started_at"`
	FinishedAt     *time.Time      `json:"finished_at,omitempty"`
	DurationMS     *int64          `json:"duration_ms,omitempty"`
	ConfirmationID *string         `gorm:"size:64" json:"confirmation_id,omitempty"`
	ErrorCategory  string          `gorm:"size:64" json:"error_category,omitempty"`
	ResultJSON     json.RawMessage `gorm:"type:jsonb;not null;default:'{}'" json:"-"`
	RequestID      string          `gorm:"size:64;index" json:"request_id,omitempty"`
	TraceID        string          `gorm:"size:64;index" json:"trace_id,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

func (ToolInvocation) TableName() string { return "ai_tool_invocations" }
