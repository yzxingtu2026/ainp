package memory

import (
	"github.com/yzxingtu2026/ainp/platform/framework/aimemory"
	"time"
)

type MemoryItem struct {
	ID                 string                 `gorm:"primaryKey;size:64" json:"id"`
	TenantID           string                 `gorm:"size:64;not null;index:idx_ai_memory_scope,priority:1;index:idx_ai_memory_current,priority:1" json:"-"`
	UserID             string                 `gorm:"size:64;not null;index:idx_ai_memory_scope,priority:2;index:idx_ai_memory_current,priority:2" json:"user_id"`
	AgentID            *string                `gorm:"size:64;index:idx_ai_memory_scope,priority:3" json:"agent_id,omitempty"`
	Type               aimemory.ItemType      `gorm:"size:24;not null;index:idx_ai_memory_current,priority:3" json:"type"`
	Status             aimemory.ItemStatus    `gorm:"size:24;not null;index:idx_ai_memory_current,priority:4" json:"status"`
	Summary            string                 `gorm:"size:512;not null" json:"summary"`
	Content            string                 `gorm:"type:text;not null" json:"content"`
	Confidence         float64                `gorm:"not null" json:"confidence"`
	Sensitivity        aimemory.Sensitivity   `gorm:"size:16;not null" json:"sensitivity"`
	Importance         int                    `gorm:"not null" json:"importance"`
	Revision           uint64                 `gorm:"not null;default:1" json:"revision"`
	OccurredAt         *time.Time             `json:"occurred_at,omitempty"`
	OccurredEndAt      *time.Time             `json:"occurred_end_at,omitempty"`
	OccurredTimezone   string                 `gorm:"size:64;not null;default:''" json:"occurred_timezone"`
	LocalDate          *time.Time             `gorm:"type:date;index" json:"local_date,omitempty"`
	DatePrecision      aimemory.DatePrecision `gorm:"size:16;not null" json:"date_precision"`
	ObservedAt         time.Time              `gorm:"not null" json:"observed_at"`
	RecordedAt         time.Time              `gorm:"not null" json:"recorded_at"`
	ValidFrom          *time.Time             `json:"valid_from,omitempty"`
	ValidTo            *time.Time             `json:"valid_to,omitempty"`
	ExpiresAt          *time.Time             `gorm:"index" json:"expires_at,omitempty"`
	LastRecalledAt     *time.Time             `json:"last_recalled_at,omitempty"`
	RecallCount        uint64                 `gorm:"not null;default:0" json:"recall_count"`
	SupersedesItemID   *string                `gorm:"size:64;index" json:"supersedes_item_id,omitempty"`
	EvaluationID       *string                `gorm:"size:64;index" json:"evaluation_id,omitempty"`
	AgentVersionID     *string                `gorm:"size:64" json:"agent_version_id,omitempty"`
	PolicyJSON         []byte                 `gorm:"type:jsonb" json:"policy,omitempty"`
	PromptVersion      string                 `gorm:"size:32;not null;default:''" json:"prompt_version,omitempty"`
	SchemaVersion      string                 `gorm:"size:32;not null;default:''" json:"schema_version,omitempty"`
	ArgumentsHash      string                 `gorm:"size:64;not null;default:''" json:"arguments_hash,omitempty"`
	ExplicitUserIntent bool                   `gorm:"not null;default:false" json:"explicit_user_intent"`
	AttributeKey       string                 `gorm:"size:64;not null;default:''" json:"attribute_key,omitempty"`
	AttributeValue     string                 `gorm:"size:512;not null;default:''" json:"attribute_value,omitempty"`
	CreatedAt          time.Time              `json:"created_at"`
	UpdatedAt          time.Time              `json:"updated_at"`
}

func (MemoryItem) TableName() string { return "ai_memory_items" }

type MemoryItemVersion struct {
	ID          string `gorm:"primaryKey;size:64"`
	TenantID    string `gorm:"size:64;not null"`
	UserID      string `gorm:"size:64;not null"`
	ItemID      string `gorm:"size:64;not null"`
	Revision    uint64
	Summary     string `gorm:"size:512;not null"`
	Content     string `gorm:"type:text;not null"`
	ContentHash string `gorm:"size:64;not null"`
	CreatedAt   time.Time
}

func (MemoryItemVersion) TableName() string { return "ai_memory_item_versions" }

type MemoryEvidence struct {
	ID                                                                                             string              `gorm:"primaryKey;size:64"`
	TenantID                                                                                       string              `gorm:"size:64;not null"`
	UserID                                                                                         string              `gorm:"size:64;not null"`
	ItemID                                                                                         string              `gorm:"size:64;not null"`
	SourceKind                                                                                     aimemory.SourceKind `gorm:"size:24;not null"`
	ConversationID, MessageID, RunID, AgentID, AgentVersionID, ActorUserID, ExtractionInvocationID *string             `gorm:"size:64"`
	Citation                                                                                       string              `gorm:"size:512;not null;default:''"`
	ContentHash                                                                                    string              `gorm:"size:64;not null"`
	SourceRevision                                                                                 uint64              `gorm:"not null;default:1"`
	RevokedAt                                                                                      *time.Time
	CreatedAt                                                                                      time.Time
}

func (MemoryEvidence) TableName() string { return "ai_memory_evidence" }

type MemoryProjection struct {
	ID                       string                    `gorm:"primaryKey;size:64"`
	TenantID                 string                    `gorm:"size:64;not null"`
	UserID                   string                    `gorm:"size:64;not null"`
	ItemID                   string                    `gorm:"size:64;not null"`
	Kind                     aimemory.ProjectionKind   `gorm:"size:24;not null"`
	Status                   aimemory.ProjectionStatus `gorm:"size:16;not null"`
	ItemRevision             uint64                    `gorm:"not null"`
	ContentHash              string                    `gorm:"size:64;not null"`
	ModelAlias, DeploymentID string                    `gorm:"size:64"`
	Dimensions               int                       `gorm:"not null;default:0"`
	SearchText               string                    `gorm:"type:text"`
	VectorJSON               []byte                    `gorm:"type:jsonb"`
	CreatedAt, UpdatedAt     time.Time
}

func (MemoryProjection) TableName() string { return "ai_memory_projections" }

type MemoryDaySummary struct {
	ID                   string `gorm:"primaryKey;size:64"`
	TenantID             string `gorm:"size:64;not null"`
	UserID               string `gorm:"size:64;not null"`
	AgentID              *string
	LocalDate            time.Time `gorm:"type:date"`
	Timezone             string    `gorm:"size:64;not null"`
	SourceRevision       uint64
	Status               aimemory.ProjectionStatus `gorm:"size:16;not null"`
	Summary              string                    `gorm:"type:text;not null"`
	ContentHash          string                    `gorm:"size:64;not null"`
	DateGroup            string                    `gorm:"size:32;not null;default:'exact'"`
	PolicyVersion        string                    `gorm:"size:32;not null;default:''"`
	ModelInvocationID    *string                   `gorm:"size:64"`
	SourceItemsJSON      []byte                    `gorm:"type:jsonb;not null;default:'[]'"`
	CreatedAt, UpdatedAt time.Time
}

func (MemoryDaySummary) TableName() string { return "ai_memory_day_summaries" }

type MemoryCurrentProfile struct {
	ID                   string `gorm:"primaryKey;size:64"`
	TenantID             string `gorm:"size:64;not null"`
	UserID               string `gorm:"size:64;not null"`
	AgentID              *string
	SourceRevision       uint64
	Status               aimemory.ProjectionStatus `gorm:"size:16;not null"`
	ProfileJSON          []byte                    `gorm:"type:jsonb;not null"`
	ContentHash          string                    `gorm:"size:64;not null"`
	PolicyVersion        string                    `gorm:"size:32;not null;default:''"`
	ModelInvocationID    *string                   `gorm:"size:64"`
	SourceItemsJSON      []byte                    `gorm:"type:jsonb;not null;default:'[]'"`
	CreatedAt, UpdatedAt time.Time
}

func (MemoryCurrentProfile) TableName() string { return "ai_memory_current_profiles" }

type MemoryEvaluation struct {
	ID                   string  `gorm:"primaryKey;size:64" json:"id"`
	TenantID             string  `gorm:"size:64;not null;uniqueIndex:ux_ai_memory_evaluation_run" json:"-"`
	UserID               string  `gorm:"size:64;not null" json:"user_id"`
	ConversationID       string  `gorm:"size:64;not null;index" json:"conversation_id"`
	RunID                string  `gorm:"size:64;not null;uniqueIndex:ux_ai_memory_evaluation_run" json:"run_id"`
	AgentVersionID       string  `gorm:"size:64;not null" json:"agent_version_id"`
	SequenceFrom         uint64  `gorm:"not null" json:"sequence_from"`
	SequenceTo           uint64  `gorm:"not null" json:"sequence_to"`
	Status               string  `gorm:"size:24;not null;index" json:"status"`
	CandidateCount       int     `gorm:"not null;default:0" json:"candidate_count"`
	InvocationID         *string `gorm:"size:64" json:"invocation_id,omitempty"`
	ArgumentsHash        string  `gorm:"size:64;not null" json:"arguments_hash"`
	ErrorCategory        string  `gorm:"size:64;not null;default:''" json:"error_category,omitempty"`
	Attempts             int     `gorm:"not null;default:0" json:"attempts"`
	CreatedAt, UpdatedAt time.Time
	CompletedAt          *time.Time
}

func (MemoryEvaluation) TableName() string { return "ai_memory_evaluations" }

type MemoryGovernanceDecision struct {
	ID             string  `gorm:"primaryKey;size:64"`
	TenantID       string  `gorm:"size:64;not null;index"`
	UserID         string  `gorm:"size:64;not null"`
	ItemID         string  `gorm:"size:64;not null;index"`
	ItemRevision   uint64  `gorm:"not null"`
	Action         string  `gorm:"size:32;not null"`
	Reason         string  `gorm:"size:64;not null"`
	RelatedItemID  *string `gorm:"size:64"`
	PolicyVersion  string  `gorm:"size:32;not null"`
	IdempotencyKey string  `gorm:"size:191;not null;uniqueIndex"`
	CreatedAt      time.Time
}

func (MemoryGovernanceDecision) TableName() string { return "ai_memory_governance_decisions" }

type MemoryRelation struct {
	ID         string `gorm:"primaryKey;size:64"`
	TenantID   string `gorm:"size:64;not null;uniqueIndex:ux_ai_memory_relation"`
	UserID     string `gorm:"size:64;not null"`
	FromItemID string `gorm:"size:64;not null;uniqueIndex:ux_ai_memory_relation"`
	ToItemID   string `gorm:"size:64;not null;uniqueIndex:ux_ai_memory_relation"`
	Kind       string `gorm:"size:24;not null;uniqueIndex:ux_ai_memory_relation"`
	CreatedAt  time.Time
}

func (MemoryRelation) TableName() string { return "ai_memory_relations" }

type MemoryConsolidationBatch struct {
	ID                   string `gorm:"primaryKey;size:64"`
	TenantID             string `gorm:"size:64;not null;uniqueIndex:ux_ai_memory_consolidation_user"`
	UserID               string `gorm:"size:64;not null;uniqueIndex:ux_ai_memory_consolidation_user"`
	Status               string `gorm:"size:24;not null;index"`
	WatermarkAt          *time.Time
	LeaseOwner           string `gorm:"size:128"`
	LeaseExpiresAt       *time.Time
	InvocationID         *string `gorm:"size:64"`
	PlanJSON             []byte  `gorm:"type:jsonb;not null;default:'{}'"`
	Attempts             int     `gorm:"not null;default:0"`
	ErrorCategory        string  `gorm:"size:64;not null;default:''"`
	CreatedAt, UpdatedAt time.Time
}

func (MemoryConsolidationBatch) TableName() string { return "ai_memory_consolidation_batches" }
