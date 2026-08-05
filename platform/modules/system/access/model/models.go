package model

import (
	"encoding/json"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
	"time"
)

// TenantAccount represents one tenant-console login account. OrgID/DeptID are reserved.
type TenantAccount struct {
	ID                 string                  `gorm:"primaryKey;size:64"`
	TenantID           string                  `gorm:"size:64;uniqueIndex:ux_tenant_accounts_tenant_username;not null"`
	Username           string                  `gorm:"size:64;uniqueIndex:ux_tenant_accounts_tenant_username;not null"`
	PasswordHash       string                  `gorm:"size:255;not null"`
	DisplayName        string                  `gorm:"size:128"`
	AvatarAttachmentID *string                 `gorm:"size:64"`
	Status             status.ActivationStatus `gorm:"size:32;not null;default:enabled"`
	AuthVersion        uint64                  `gorm:"not null;default:1"`
	OrgID              string                  `gorm:"size:64"`
	DeptID             string                  `gorm:"size:64"`
	CreatedAt          time.Time               `gorm:"autoCreateTime"`
	UpdatedAt          time.Time               `gorm:"autoUpdateTime"`
}

func (TenantAccount) TableName() string { return "tenant_accounts" }

type AuthIdentity struct {
	ID                   string                  `gorm:"primaryKey;size:64"`
	TenantID             string                  `gorm:"size:64;uniqueIndex:ux_auth_identity_subject;not null;index:idx_auth_identities_account"`
	AccountID            string                  `gorm:"size:64;not null;index:idx_auth_identities_account"`
	Provider             string                  `gorm:"size:64;uniqueIndex:ux_auth_identity_subject;not null"`
	Subject              string                  `gorm:"size:255;uniqueIndex:ux_auth_identity_subject;not null"`
	NormalizedIdentifier string                  `gorm:"size:255;index"`
	Status               status.ActivationStatus `gorm:"size:32;not null;default:enabled"`
	VerifiedAt           time.Time               `gorm:"not null"`
	Metadata             json.RawMessage         `gorm:"type:jsonb"`
	CreatedAt            time.Time               `gorm:"autoCreateTime"`
	UpdatedAt            time.Time               `gorm:"autoUpdateTime"`
}

func (AuthIdentity) TableName() string { return "auth_identities" }

type AuthSession struct {
	ID                       string     `gorm:"primaryKey;size:64"`
	TenantID                 string     `gorm:"size:64;not null;index:idx_auth_sessions_account"`
	AccountID                string     `gorm:"size:64;not null;index:idx_auth_sessions_account"`
	Realm                    string     `gorm:"size:32;not null"`
	Client                   string     `gorm:"size:64;not null"`
	DeviceSummary            string     `gorm:"size:255"`
	RefreshTokenHash         string     `gorm:"size:64;not null;uniqueIndex"`
	PreviousRefreshTokenHash string     `gorm:"size:64;index"`
	AuthVersion              uint64     `gorm:"not null"`
	ExpiresAt                time.Time  `gorm:"not null;index"`
	LastSeenAt               time.Time  `gorm:"not null"`
	RevokedAt                *time.Time `gorm:"index"`
	CreatedAt                time.Time  `gorm:"autoCreateTime"`
	UpdatedAt                time.Time  `gorm:"autoUpdateTime"`
}

func (AuthSession) TableName() string { return "auth_sessions" }

type AuthChallenge struct {
	ID          string     `gorm:"primaryKey;size:64"`
	Method      string     `gorm:"size:64;not null;index"`
	SecretHash  string     `gorm:"size:64"`
	Client      string     `gorm:"size:64"`
	Attempts    int        `gorm:"not null;default:0"`
	MaxAttempts int        `gorm:"not null;default:5"`
	ExpiresAt   time.Time  `gorm:"not null;index"`
	ConsumedAt  *time.Time `gorm:"index"`
	CreatedAt   time.Time  `gorm:"autoCreateTime"`
}

func (AuthChallenge) TableName() string { return "auth_challenges" }

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

type AccountRole struct {
	ID        string `gorm:"primaryKey;size:64"`
	TenantID  string `gorm:"size:64;index;not null"`
	AccountID string `gorm:"size:64;uniqueIndex:ux_tenant_account_role;not null"`
	RoleID    string `gorm:"size:64;uniqueIndex:ux_tenant_account_role;not null"`
}

func (AccountRole) TableName() string { return "tenant_account_roles" }
