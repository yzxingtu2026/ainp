package ai

import "time"

type AgentSlotBinding struct {
	ID          string    `gorm:"primaryKey;size:64" json:"id"`
	TenantID    string    `gorm:"size:64;not null;uniqueIndex:ux_ai_agent_slot_binding" json:"-"`
	SlotKey     string    `gorm:"size:128;not null;uniqueIndex:ux_ai_agent_slot_binding" json:"slot_key"`
	SlotVersion string    `gorm:"size:32;not null" json:"slot_version"`
	AgentID     string    `gorm:"size:64;not null;index" json:"agent_id"`
	Revision    uint64    `gorm:"not null;default:1" json:"revision"`
	CreatedBy   string    `gorm:"size:64;not null" json:"created_by"`
	UpdatedBy   string    `gorm:"size:64;not null" json:"updated_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (AgentSlotBinding) TableName() string { return "ai_agent_slot_bindings" }

type AgentSlotPreference struct {
	ID          string    `gorm:"primaryKey;size:64" json:"id"`
	TenantID    string    `gorm:"size:64;not null;uniqueIndex:ux_ai_agent_slot_preference" json:"-"`
	UserID      string    `gorm:"size:64;not null;uniqueIndex:ux_ai_agent_slot_preference" json:"user_id"`
	SlotKey     string    `gorm:"size:128;not null;uniqueIndex:ux_ai_agent_slot_preference" json:"slot_key"`
	SlotVersion string    `gorm:"size:32;not null" json:"slot_version"`
	AgentID     string    `gorm:"size:64;not null;index" json:"agent_id"`
	Revision    uint64    `gorm:"not null;default:1" json:"revision"`
	UpdatedBy   string    `gorm:"size:64;not null" json:"updated_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (AgentSlotPreference) TableName() string { return "ai_agent_slot_preferences" }
