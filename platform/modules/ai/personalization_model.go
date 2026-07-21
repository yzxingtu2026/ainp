package ai

import "time"

type AssistantPersonalizationSnapshot struct {
	ID                 string `gorm:"primaryKey;size:64"`
	TenantID           string `gorm:"size:64;not null;uniqueIndex:ux_ai_assistant_personalization,priority:1"`
	UserID             string `gorm:"size:64;not null;uniqueIndex:ux_ai_assistant_personalization,priority:2"`
	SlotKey            string `gorm:"size:128;not null;uniqueIndex:ux_ai_assistant_personalization,priority:3"`
	AgentVersionID     string `gorm:"size:64;not null;default:''"`
	Locale             string `gorm:"size:16;not null"`
	Timezone           string `gorm:"size:64;not null"`
	ContextScope       string `gorm:"size:32;not null;default:'global'"`
	ContextHash        string `gorm:"size:64;not null;default:''"`
	Greeting           string `gorm:"size:240;not null"`
	SuggestionsJSON    []byte `gorm:"type:jsonb;not null;default:'[]'"`
	SourceVersionsJSON []byte `gorm:"type:jsonb;not null;default:'{}'"`
	ModelAlias         string `gorm:"size:64;not null"`
	PromptVersion      string `gorm:"size:32;not null"`
	Status             string `gorm:"size:16;not null;index"`
	GenerationRevision uint64 `gorm:"not null;default:1"`
	GeneratedAt        time.Time
	ExpiresAt          time.Time `gorm:"index"`
	LastErrorCategory  string    `gorm:"size:64;not null;default:''"`
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (AssistantPersonalizationSnapshot) TableName() string {
	return "ai_assistant_personalization_snapshots"
}
