package ai

import (
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

type Provider struct {
	ID                   string          `gorm:"primaryKey;size:64" json:"id"`
	TenantID             string          `gorm:"size:64;not null;uniqueIndex:ux_ai_providers_tenant_code" json:"-"`
	Code                 string          `gorm:"size:64;not null;uniqueIndex:ux_ai_providers_tenant_code" json:"code"`
	Name                 string          `gorm:"size:128;not null" json:"name"`
	Protocol             string          `gorm:"size:64;not null" json:"protocol"`
	BaseURL              string          `gorm:"size:512" json:"base_url"`
	CredentialCiphertext string          `gorm:"type:text;not null" json:"-"`
	CredentialConfigured bool            `gorm:"-" json:"credential_configured"`
	Status               string          `gorm:"size:32;not null;default:enabled" json:"status"`
	ConfigJSON           json.RawMessage `gorm:"type:jsonb;not null;default:'{}'" json:"config_json"`
	CreatedAt            time.Time       `json:"created_at"`
	UpdatedAt            time.Time       `json:"updated_at"`
}

func (Provider) TableName() string { return "ai_providers" }

type Deployment struct {
	ID               string          `gorm:"primaryKey;size:64" json:"id"`
	TenantID         string          `gorm:"size:64;not null;uniqueIndex:ux_ai_deployments_tenant_code" json:"-"`
	ProviderID       string          `gorm:"size:64;not null;uniqueIndex:ux_ai_deployments_provider_remote;index" json:"provider_id"`
	Code             string          `gorm:"size:64;not null;uniqueIndex:ux_ai_deployments_tenant_code" json:"code"`
	RemoteModelID    string          `gorm:"size:255;not null;uniqueIndex:ux_ai_deployments_provider_remote" json:"remote_model_id"`
	DisplayName      string          `gorm:"size:128;not null" json:"display_name"`
	ModelType        string          `gorm:"size:32;not null" json:"model_type"`
	CapabilitiesJSON json.RawMessage `gorm:"type:jsonb;not null" json:"capabilities_json"`
	LimitsJSON       json.RawMessage `gorm:"type:jsonb;not null" json:"limits_json"`
	Status           string          `gorm:"size:32;not null;default:enabled" json:"status"`
	ConfigJSON       json.RawMessage `gorm:"type:jsonb;not null;default:'{}'" json:"config_json"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

func (Deployment) TableName() string { return "ai_model_deployments" }

type Alias struct {
	ID           string    `gorm:"primaryKey;size:64" json:"id"`
	TenantID     string    `gorm:"size:64;not null;uniqueIndex:ux_ai_aliases_tenant_code" json:"-"`
	Code         string    `gorm:"size:64;not null;uniqueIndex:ux_ai_aliases_tenant_code" json:"code"`
	DeploymentID string    `gorm:"size:64;not null;index" json:"deployment_id"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (Alias) TableName() string { return "ai_model_aliases" }

type ModelInvocation struct {
	ID                   string          `gorm:"primaryKey;size:64" json:"id"`
	TenantID             string          `gorm:"size:64;not null;index" json:"-"`
	UserID               string          `gorm:"size:64;not null;index" json:"user_id"`
	SessionID            string          `gorm:"size:64" json:"session_id,omitempty"`
	AgentID              *string         `gorm:"size:64" json:"agent_id,omitempty"`
	AgentRunID           *string         `gorm:"size:64" json:"agent_run_id,omitempty"`
	ModelAlias           string          `gorm:"size:64;not null" json:"model_alias"`
	ProviderID           string          `gorm:"size:64;not null" json:"provider_id"`
	DeploymentID         string          `gorm:"size:64;not null" json:"deployment_id"`
	RemoteModelID        string          `gorm:"size:255;not null" json:"remote_model_id"`
	Mode                 string          `gorm:"size:16;not null" json:"mode"`
	Status               string          `gorm:"size:16;not null;index" json:"status"`
	StartedAt            time.Time       `gorm:"not null" json:"started_at"`
	FirstTokenAt         *time.Time      `json:"first_token_at,omitempty"`
	FinishedAt           *time.Time      `json:"finished_at,omitempty"`
	LatencyMS            *int64          `json:"latency_ms,omitempty"`
	FirstTokenLatencyMS  *int64          `json:"first_token_latency_ms,omitempty"`
	InputTokens          *int64          `json:"input_tokens,omitempty"`
	OutputTokens         *int64          `json:"output_tokens,omitempty"`
	TotalTokens          *int64          `json:"total_tokens,omitempty"`
	CachedInputTokens    *int64          `json:"cached_input_tokens,omitempty"`
	ReasoningTokens      *int64          `json:"reasoning_tokens,omitempty"`
	UsageReported        bool            `gorm:"not null;default:false" json:"usage_reported"`
	FinishReason         string          `gorm:"size:32" json:"finish_reason,omitempty"`
	ErrorCategory        string          `gorm:"size:64" json:"error_category,omitempty"`
	ProviderRequestID    string          `gorm:"size:255" json:"provider_request_id,omitempty"`
	RequestID            string          `gorm:"size:64;index" json:"request_id,omitempty"`
	TraceID              string          `gorm:"size:64;index" json:"trace_id,omitempty"`
	InputModalitiesJSON  json.RawMessage `gorm:"type:jsonb;not null;default:'[]'" json:"input_modalities"`
	InputAttachmentCount int             `gorm:"not null;default:0" json:"input_attachment_count"`
	InputAttachmentBytes int64           `gorm:"not null;default:0" json:"input_attachment_bytes"`
	CreatedAt            time.Time       `json:"created_at"`
	UpdatedAt            time.Time       `json:"updated_at"`
}

func (ModelInvocation) TableName() string { return "ai_model_invocations" }

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
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (ToolCatalog) TableName() string { return "ai_tool_catalog" }

type ToolSearchDocument struct {
	ID             string          `gorm:"primaryKey;size:64"`
	TenantID       string          `gorm:"size:64;not null;uniqueIndex:ux_ai_tool_search_tenant_tool"`
	ToolID         string          `gorm:"size:255;not null;uniqueIndex:ux_ai_tool_search_tenant_tool"`
	SearchText     string          `gorm:"type:text;not null"`
	InputHintsJSON json.RawMessage `gorm:"type:jsonb;not null;default:'[]'"`
	IndexStatus    string          `gorm:"size:16;not null"`
	EmbeddingModel string          `gorm:"size:128"`
	UpdatedAt      time.Time
}

func (ToolSearchDocument) TableName() string { return "ai_tool_search_documents" }

type ToolScope struct {
	ID                      string          `gorm:"primaryKey;size:64" json:"id"`
	TenantID                string          `gorm:"size:64;not null;index" json:"-"`
	UserID                  string          `gorm:"size:64;not null;index" json:"-"`
	Realm                   string          `gorm:"size:16;not null" json:"realm"`
	SessionID               string          `gorm:"size:64;not null" json:"-"`
	AuthVersion             uint64          `gorm:"not null" json:"-"`
	AgentRunID              *string         `gorm:"size:64" json:"agent_run_id,omitempty"`
	AllowedSourcesJSON      json.RawMessage `gorm:"type:jsonb;not null;default:'[]'" json:"-"`
	AllowedCapabilitiesJSON json.RawMessage `gorm:"type:jsonb;not null;default:'[]'" json:"-"`
	AllowedToolsJSON        json.RawMessage `gorm:"type:jsonb;not null;default:'[]'" json:"-"`
	LoadedToolsJSON         json.RawMessage `gorm:"type:jsonb;not null;default:'[]'" json:"loaded_tool_ids"`
	SchemaBytes             int             `gorm:"not null;default:0" json:"schema_bytes"`
	ExpiresAt               time.Time       `gorm:"not null;index" json:"expires_at"`
	CreatedAt               time.Time       `json:"created_at"`
	UpdatedAt               time.Time       `json:"updated_at"`
}

func (ToolScope) TableName() string { return "ai_tool_scopes" }

type ToolConfirmation struct {
	ID                   string          `gorm:"primaryKey;size:64" json:"id"`
	TenantID             string          `gorm:"size:64;not null;index" json:"-"`
	UserID               string          `gorm:"size:64;not null;index" json:"-"`
	ScopeID              string          `gorm:"size:64;not null;index" json:"scope_id"`
	InvocationID         string          `gorm:"size:64;not null;index" json:"invocation_id"`
	ToolCallID           string          `gorm:"size:128;not null" json:"tool_call_id"`
	ToolID               string          `gorm:"size:255;not null" json:"tool_id"`
	SchemaHash           string          `gorm:"size:64;not null" json:"schema_hash"`
	ArgumentsHash        string          `gorm:"size:64;not null" json:"arguments_hash"`
	ArgumentsJSON        json.RawMessage `gorm:"type:jsonb;not null" json:"-"`
	ArgumentsSummaryJSON json.RawMessage `gorm:"type:jsonb;not null" json:"arguments_summary"`
	Risk                 string          `gorm:"size:16;not null" json:"risk"`
	SideEffect           string          `gorm:"size:16;not null" json:"side_effect"`
	Status               string          `gorm:"size:16;not null;index" json:"status"`
	ExpiresAt            time.Time       `gorm:"not null" json:"expires_at"`
	DecidedAt            *time.Time      `json:"decided_at,omitempty"`
	ConsumedAt           *time.Time      `json:"consumed_at,omitempty"`
	CreatedAt            time.Time       `json:"created_at"`
	UpdatedAt            time.Time       `json:"updated_at"`
}

func (ToolConfirmation) TableName() string { return "ai_tool_confirmations" }

type ToolInvocation struct {
	ID             string     `gorm:"primaryKey;size:64" json:"id"`
	TenantID       string     `gorm:"size:64;not null;index" json:"-"`
	UserID         string     `gorm:"size:64;not null;index" json:"-"`
	ScopeID        string     `gorm:"size:64;not null;index" json:"scope_id"`
	AgentRunID     *string    `gorm:"size:64" json:"agent_run_id,omitempty"`
	ToolCallID     string     `gorm:"size:128;not null" json:"tool_call_id"`
	ToolID         string     `gorm:"size:255;not null" json:"tool_id"`
	Source         string     `gorm:"size:32;not null" json:"source"`
	TargetAction   string     `gorm:"size:255" json:"target_action,omitempty"`
	Risk           string     `gorm:"size:16;not null" json:"risk"`
	SideEffect     string     `gorm:"size:16;not null" json:"side_effect"`
	Status         string     `gorm:"size:24;not null;index" json:"status"`
	StartedAt      time.Time  `gorm:"not null" json:"started_at"`
	FinishedAt     *time.Time `json:"finished_at,omitempty"`
	DurationMS     *int64     `json:"duration_ms,omitempty"`
	ConfirmationID *string    `gorm:"size:64" json:"confirmation_id,omitempty"`
	ErrorCategory  string     `gorm:"size:64" json:"error_category,omitempty"`
	RequestID      string     `gorm:"size:64;index" json:"request_id,omitempty"`
	TraceID        string     `gorm:"size:64;index" json:"trace_id,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func (ToolInvocation) TableName() string { return "ai_tool_invocations" }

func AutoMigrate(db *gorm.DB) error {
	if db.Migrator().HasTable(&Provider{}) && db.Migrator().HasColumn(&Provider{}, "credential_ref") && !db.Migrator().HasColumn(&Provider{}, "credential_ciphertext") {
		if err := db.Exec(`ALTER TABLE ai_providers ADD COLUMN credential_ciphertext text NOT NULL DEFAULT ''`).Error; err != nil {
			return err
		}
		if err := db.Exec(`UPDATE ai_providers SET status = 'disabled' WHERE credential_ciphertext = ''`).Error; err != nil {
			return err
		}
		if err := db.Migrator().DropColumn(&Provider{}, "credential_ref"); err != nil {
			return err
		}
		if err := db.Exec(`ALTER TABLE ai_providers ALTER COLUMN credential_ciphertext DROP DEFAULT`).Error; err != nil {
			return err
		}
	}
	return db.AutoMigrate(&Provider{}, &Deployment{}, &Alias{}, &ModelInvocation{}, &ToolCatalog{}, &ToolSearchDocument{}, &ToolScope{}, &ToolConfirmation{}, &ToolInvocation{}, &Agent{}, &AgentVersion{}, &AgentToolBinding{})
}
