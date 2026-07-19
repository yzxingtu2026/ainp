package ai

import (
	"encoding/json"
	"time"
)

const (
	ConversationPurposeWorkspace      = "workspace"
	ConversationPurposeBuilderPreview = "builder_preview"
)

type Conversation struct {
	ID             string     `gorm:"primaryKey;size:64" json:"id"`
	TenantID       string     `gorm:"size:64;not null;index:idx_ai_conversation_list,priority:1" json:"-"`
	OwnerUserID    string     `gorm:"size:64;not null;index:idx_ai_conversation_list,priority:2" json:"owner_user_id"`
	Title          string     `gorm:"size:256;not null" json:"title"`
	AgentID        string     `gorm:"size:64;not null;index" json:"agent_id"`
	AgentVersionID string     `gorm:"size:64;not null;index" json:"agent_version_id"`
	Purpose        string     `gorm:"size:32;not null;default:workspace;index" json:"purpose"`
	Visibility     string     `gorm:"size:16;not null;index;check:ck_ai_conversations_visibility,visibility IN ('private','tenant')" json:"visibility"`
	Status         string     `gorm:"size:16;not null;index:idx_ai_conversation_list,priority:3;check:ck_ai_conversations_status,status IN ('active','archived')" json:"status"`
	LastMessageAt  *time.Time `gorm:"index:idx_ai_conversation_list,priority:4" json:"last_message_at,omitempty"`
	Revision       uint64     `gorm:"not null;default:1" json:"revision"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func (Conversation) TableName() string { return "ai_conversations" }

type ConversationMessage struct {
	ID               string          `gorm:"primaryKey;size:64" json:"id"`
	TenantID         string          `gorm:"size:64;not null;uniqueIndex:ux_ai_message_sequence,priority:1;uniqueIndex:ux_ai_message_idem,priority:1;uniqueIndex:ux_ai_message_stream,priority:1" json:"-"`
	ConversationID   string          `gorm:"size:64;not null;uniqueIndex:ux_ai_message_sequence,priority:2;uniqueIndex:ux_ai_message_idem,priority:2;index" json:"conversation_id"`
	Role             string          `gorm:"size:16;not null;check:ck_ai_messages_role,role IN ('user','assistant','system','tool')" json:"role"`
	Status           string          `gorm:"size:16;not null;index;check:ck_ai_messages_status,status IN ('pending','committed','failed','cancelled')" json:"status"`
	Sequence         uint64          `gorm:"not null;uniqueIndex:ux_ai_message_sequence,priority:3" json:"sequence"`
	ReplyToMessageID *string         `gorm:"size:64;index" json:"reply_to_message_id,omitempty"`
	RunID            *string         `gorm:"size:64;index" json:"run_id,omitempty"`
	StreamID         *string         `gorm:"size:64;uniqueIndex:ux_ai_message_stream,priority:2" json:"stream_id,omitempty"`
	AuthorUserID     *string         `gorm:"size:64" json:"author_user_id,omitempty"`
	Content          json.RawMessage `gorm:"type:jsonb;not null" json:"content"`
	IdempotencyKey   string          `gorm:"size:191;not null;uniqueIndex:ux_ai_message_idem,priority:3" json:"-"`
	CreatedAt        time.Time       `json:"created_at"`
	CommittedAt      *time.Time      `json:"committed_at,omitempty"`
}

func (ConversationMessage) TableName() string { return "ai_messages" }

type AssistantStream struct {
	ID             string     `gorm:"primaryKey;size:64" json:"id"`
	TenantID       string     `gorm:"size:64;not null;uniqueIndex:ux_ai_stream_idem,priority:1" json:"-"`
	ConversationID string     `gorm:"size:64;not null;uniqueIndex:ux_ai_stream_idem,priority:2;index" json:"conversation_id"`
	IdempotencyKey string     `gorm:"size:191;not null;uniqueIndex:ux_ai_stream_idem,priority:3" json:"-"`
	Status         string     `gorm:"size:16;not null;check:ck_ai_stream_status,status IN ('streaming','committed','failed','cancelled')" json:"status"`
	MessageID      *string    `gorm:"size:64" json:"message_id,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
}

func (AssistantStream) TableName() string { return "ai_assistant_streams" }

type AssistantStreamDelta struct {
	ID        string    `gorm:"primaryKey;size:64" json:"id"`
	TenantID  string    `gorm:"size:64;not null;uniqueIndex:ux_ai_stream_cursor,priority:1" json:"-"`
	StreamID  string    `gorm:"size:64;not null;uniqueIndex:ux_ai_stream_cursor,priority:2;index" json:"stream_id"`
	Cursor    uint64    `gorm:"not null;uniqueIndex:ux_ai_stream_cursor,priority:3" json:"cursor"`
	Type      string    `gorm:"size:64;not null" json:"type"`
	Data      []byte    `gorm:"type:bytea;not null" json:"data"`
	CreatedAt time.Time `json:"created_at"`
}

func (AssistantStreamDelta) TableName() string { return "ai_assistant_stream_deltas" }

type ContextSnapshot struct {
	ID                     string          `gorm:"primaryKey;size:64" json:"id"`
	TenantID               string          `gorm:"size:64;not null;uniqueIndex:ux_ai_snapshot_idem,priority:1" json:"-"`
	ConversationID         string          `gorm:"size:64;not null;uniqueIndex:ux_ai_snapshot_idem,priority:2;index" json:"conversation_id"`
	ConversationRevision   uint64          `gorm:"not null" json:"conversation_revision"`
	AgentVersionID         string          `gorm:"size:64;not null" json:"agent_version_id"`
	ModelDeploymentID      string          `gorm:"size:64;not null" json:"model_deployment_id"`
	SourceSequenceFrom     uint64          `gorm:"not null" json:"source_sequence_from"`
	SourceSequenceTo       uint64          `gorm:"not null;check:ck_ai_snapshot_range,source_sequence_from <= source_sequence_to" json:"source_sequence_to"`
	ParentSnapshotID       *string         `gorm:"size:64;index" json:"parent_snapshot_id,omitempty"`
	Kind                   string          `gorm:"size:16;not null;check:ck_ai_snapshot_kind,kind IN ('raw','assembled','compacted')" json:"kind"`
	PolicyVersion          string          `gorm:"size:32;not null" json:"policy_version"`
	SchemaVersion          string          `gorm:"size:32;not null;default:''" json:"schema_version"`
	PromptVersion          string          `gorm:"size:32;not null;default:''" json:"prompt_version"`
	SummarizerDeploymentID string          `gorm:"size:64;not null;default:''" json:"summarizer_deployment_id,omitempty"`
	CompactionDepth        int             `gorm:"not null;default:0;check:ck_ai_snapshot_depth,compaction_depth >= 0 AND compaction_depth <= 3" json:"compaction_depth"`
	Reason                 string          `gorm:"size:64;not null;default:''" json:"reason"`
	QualityStatus          string          `gorm:"size:16;not null;default:'valid';check:ck_ai_snapshot_quality,quality_status IN ('valid','invalid')" json:"quality_status"`
	TokenEstimate          int             `gorm:"not null" json:"token_estimate"`
	Status                 string          `gorm:"size:16;not null;check:ck_ai_snapshot_status,status IN ('ready','invalid')" json:"status"`
	Payload                json.RawMessage `gorm:"type:jsonb;not null" json:"payload"`
	RuntimeContextRefs     json.RawMessage `gorm:"type:jsonb;not null;default:'{}'" json:"runtime_context_refs,omitempty"`
	IdempotencyKey         string          `gorm:"size:191;not null;uniqueIndex:ux_ai_snapshot_idem,priority:3" json:"-"`
	CreatedAt              time.Time       `json:"created_at"`
	Current                bool            `gorm:"-" json:"is_current"`
}

func (ContextSnapshot) TableName() string { return "ai_context_snapshots" }
