package model

import (
	"time"

	accessmodel "github.com/yzxingtu2026/ainp/platform/modules/system/access/model"
)

type (
	TenantAccount       = accessmodel.TenantAccount
	AuthIdentity        = accessmodel.AuthIdentity
	AuthSession         = accessmodel.AuthSession
	AuthRefreshTokenUse = accessmodel.AuthRefreshTokenUse
	AuthChallenge       = accessmodel.AuthChallenge
	Role                = accessmodel.Role
	RolePermission      = accessmodel.RolePermission
	AccountRole         = accessmodel.AccountRole

	// Deprecated: use TenantAccount.
	User = TenantAccount
	// Deprecated: use AccountRole.
	UserRole = AccountRole
)

// TenantSettingValue stores tenant overrides for one setting definition.
type TenantSettingValue struct {
	ID            string    `gorm:"primaryKey;size:64"`
	TenantID      string    `gorm:"size:64;uniqueIndex:ux_tenant_setting_key;not null;index"`
	SettingKey    string    `gorm:"size:128;uniqueIndex:ux_tenant_setting_key;not null"`
	SchemaVersion int       `gorm:"not null"`
	ValueJSON     []byte    `gorm:"type:jsonb;not null"`
	Revision      int64     `gorm:"not null;default:1"`
	UpdatedBy     string    `gorm:"size:64"`
	CreatedAt     time.Time `gorm:"autoCreateTime"`
	UpdatedAt     time.Time `gorm:"autoUpdateTime"`
}

func (TenantSettingValue) TableName() string { return "tenant_setting_values" }

// UserSettingValue stores one account's private, schema-registered overrides.
type UserSettingValue struct {
	ID            string    `gorm:"primaryKey;size:64"`
	TenantID      string    `gorm:"size:64;uniqueIndex:ux_user_setting_key;not null;index"`
	AccountID     string    `gorm:"size:64;uniqueIndex:ux_user_setting_key;not null;index"`
	SettingKey    string    `gorm:"size:128;uniqueIndex:ux_user_setting_key;not null"`
	SchemaVersion int       `gorm:"not null"`
	ValueJSON     []byte    `gorm:"type:jsonb;not null"`
	Revision      int64     `gorm:"not null;default:1"`
	UpdatedBy     string    `gorm:"size:64;not null"`
	CreatedAt     time.Time `gorm:"autoCreateTime"`
	UpdatedAt     time.Time `gorm:"autoUpdateTime"`
}

func (UserSettingValue) TableName() string { return "user_setting_values" }
