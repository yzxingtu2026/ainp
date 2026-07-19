package ai

import (
	"github.com/yzxingtu2026/ainp/platform/framework/aimemory"
	"time"
)

type MemoryItem struct {
	ID               string                 `gorm:"primaryKey;size:64" json:"id"`
	TenantID         string                 `gorm:"size:64;not null;uniqueIndex:ux_ai_memory_item_scope,priority:1;index:idx_ai_memory_current,priority:1" json:"-"`
	UserID           string                 `gorm:"size:64;not null;uniqueIndex:ux_ai_memory_item_scope,priority:2;index:idx_ai_memory_current,priority:2" json:"user_id"`
	AgentID          *string                `gorm:"size:64;uniqueIndex:ux_ai_memory_item_scope,priority:3" json:"agent_id,omitempty"`
	Type             aimemory.ItemType      `gorm:"size:24;not null;index:idx_ai_memory_current,priority:3" json:"type"`
	Status           aimemory.ItemStatus    `gorm:"size:24;not null;index:idx_ai_memory_current,priority:4" json:"status"`
	Summary          string                 `gorm:"size:512;not null" json:"summary"`
	Content          string                 `gorm:"type:text;not null" json:"content"`
	Confidence       float64                `gorm:"not null" json:"confidence"`
	Sensitivity      aimemory.Sensitivity   `gorm:"size:16;not null" json:"sensitivity"`
	Importance       int                    `gorm:"not null" json:"importance"`
	Revision         uint64                 `gorm:"not null;default:1" json:"revision"`
	OccurredAt       *time.Time             `json:"occurred_at,omitempty"`
	OccurredEndAt    *time.Time             `json:"occurred_end_at,omitempty"`
	OccurredTimezone string                 `gorm:"size:64;not null;default:''" json:"occurred_timezone"`
	LocalDate        *time.Time             `gorm:"type:date;index" json:"local_date,omitempty"`
	DatePrecision    aimemory.DatePrecision `gorm:"size:16;not null" json:"date_precision"`
	ObservedAt       time.Time              `gorm:"not null" json:"observed_at"`
	RecordedAt       time.Time              `gorm:"not null" json:"recorded_at"`
	ValidFrom        *time.Time             `json:"valid_from,omitempty"`
	ValidTo          *time.Time             `json:"valid_to,omitempty"`
	ExpiresAt        *time.Time             `gorm:"index" json:"expires_at,omitempty"`
	LastRecalledAt   *time.Time             `json:"last_recalled_at,omitempty"`
	RecallCount      uint64                 `gorm:"not null;default:0" json:"recall_count"`
	SupersedesItemID *string                `gorm:"size:64;index" json:"supersedes_item_id,omitempty"`
	CreatedAt        time.Time              `json:"created_at"`
	UpdatedAt        time.Time              `json:"updated_at"`
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
	CreatedAt, UpdatedAt time.Time
}

func (MemoryCurrentProfile) TableName() string { return "ai_memory_current_profiles" }
