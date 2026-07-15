package model

import (
	"github.com/yzxingtu2026/ainp/platform/framework/status"
	"time"
)

// User 一用户一租户；OrgID/DeptID 仅预留。
type User struct {
	ID           string                  `gorm:"primaryKey;size:64"`
	TenantID     string                  `gorm:"size:64;uniqueIndex:ux_users_tenant_username;not null"`
	Username     string                  `gorm:"size:64;uniqueIndex:ux_users_tenant_username;not null"`
	PasswordHash string                  `gorm:"size:255;not null"`
	DisplayName  string                  `gorm:"size:128"`
	Status       status.ActivationStatus `gorm:"size:32;not null;default:enabled"`
	AuthVersion  uint64                  `gorm:"not null;default:1"`
	OrgID        string                  `gorm:"size:64"`
	DeptID       string                  `gorm:"size:64"`
	CreatedAt    time.Time               `gorm:"autoCreateTime"`
	UpdatedAt    time.Time               `gorm:"autoUpdateTime"`
}

func (User) TableName() string { return "users" }

type Role struct {
	ID          string                  `gorm:"primaryKey;size:64"`
	TenantID    string                  `gorm:"size:64;uniqueIndex:ux_roles_tenant_code;not null"`
	Code        string                  `gorm:"size:64;uniqueIndex:ux_roles_tenant_code;not null"`
	Name        string                  `gorm:"size:128;not null"`
	Description string                  `gorm:"size:512"`
	Status      status.ActivationStatus `gorm:"size:32;not null;default:enabled"`
	IsBuiltin   bool                    `gorm:"not null;default:false"`
	CreatedAt   time.Time               `gorm:"autoCreateTime"`
	UpdatedAt   time.Time               `gorm:"autoUpdateTime"`
}

func (Role) TableName() string { return "roles" }

type RolePermission struct {
	ID         string `gorm:"primaryKey;size:64"`
	RoleID     string `gorm:"size:64;uniqueIndex:ux_role_perm;not null"`
	Permission string `gorm:"size:128;uniqueIndex:ux_role_perm;not null"`
}

func (RolePermission) TableName() string { return "role_permissions" }

type UserRole struct {
	ID       string `gorm:"primaryKey;size:64"`
	TenantID string `gorm:"size:64;index;not null"`
	UserID   string `gorm:"size:64;uniqueIndex:ux_user_role;not null"`
	RoleID   string `gorm:"size:64;uniqueIndex:ux_user_role;not null"`
}

func (UserRole) TableName() string { return "user_roles" }

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
