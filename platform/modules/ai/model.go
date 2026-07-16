package ai

import (
	"encoding/json"
	"time"
)

type Provider struct {
	ID            string          `gorm:"primaryKey;size:64" json:"id"`
	TenantID      string          `gorm:"size:64;not null;uniqueIndex:ux_ai_providers_tenant_code" json:"-"`
	Code          string          `gorm:"size:64;not null;uniqueIndex:ux_ai_providers_tenant_code" json:"code"`
	Name          string          `gorm:"size:128;not null" json:"name"`
	Protocol      string          `gorm:"size:64;not null" json:"protocol"`
	BaseURL       string          `gorm:"size:512" json:"base_url"`
	CredentialRef string          `gorm:"size:160;not null" json:"credential_ref"`
	Status        string          `gorm:"size:32;not null;default:enabled" json:"status"`
	ConfigJSON    json.RawMessage `gorm:"type:jsonb;not null;default:'{}'" json:"config_json"`
	CreatedAt     time.Time       `json:"created_at"`
	UpdatedAt     time.Time       `json:"updated_at"`
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

func AutoMigrate(db interface{ AutoMigrate(...interface{}) error }) error {
	return db.AutoMigrate(&Provider{}, &Deployment{}, &Alias{})
}
