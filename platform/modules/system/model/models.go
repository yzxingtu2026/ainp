package model

import "time"

const (
	StatusEnabled  = "enabled"
	StatusDisabled = "disabled"
)

// User 一用户一租户；OrgID/DeptID 仅预留。
type User struct {
	ID           string    `gorm:"primaryKey;size:64"`
	TenantID     string    `gorm:"size:64;uniqueIndex:ux_users_tenant_username;not null"`
	Username     string    `gorm:"size:64;uniqueIndex:ux_users_tenant_username;not null"`
	PasswordHash string    `gorm:"size:255;not null"`
	DisplayName  string    `gorm:"size:128"`
	Status       string    `gorm:"size:32;not null;default:enabled"`
	OrgID        string    `gorm:"size:64"`
	DeptID       string    `gorm:"size:64"`
	CreatedAt    time.Time `gorm:"autoCreateTime"`
	UpdatedAt    time.Time `gorm:"autoUpdateTime"`
}

func (User) TableName() string { return "users" }

type Role struct {
	ID        string    `gorm:"primaryKey;size:64"`
	TenantID  string    `gorm:"size:64;uniqueIndex:ux_roles_tenant_code;not null"`
	Code      string    `gorm:"size:64;uniqueIndex:ux_roles_tenant_code;not null"`
	Name      string    `gorm:"size:128"`
	Status    string    `gorm:"size:32;not null;default:enabled"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
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
