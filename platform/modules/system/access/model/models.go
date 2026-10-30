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
	TenantID             string                  `gorm:"size:64;uniqueIndex:ux_auth_identity_subject;uniqueIndex:ux_auth_identities_tenant_account_provider;not null;index:idx_auth_identities_account"`
	AccountID            string                  `gorm:"size:64;uniqueIndex:ux_auth_identities_tenant_account_provider;not null;index:idx_auth_identities_account"`
	Provider             string                  `gorm:"size:64;uniqueIndex:ux_auth_identity_subject;uniqueIndex:ux_auth_identities_tenant_account_provider;not null"`
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
	TenantID                 string     `gorm:"size:64;not null;index:idx_auth_sessions_principal"`
	PrincipalKind            string     `gorm:"size:128;not null;index:idx_auth_sessions_principal"`
	PrincipalID              string     `gorm:"size:255;not null;index:idx_auth_sessions_principal"`
	AccountID                *string    `gorm:"size:64;index:idx_auth_sessions_account"`
	Realm                    string     `gorm:"size:32;not null"`
	Client                   string     `gorm:"size:64;not null"`
	ClientSurface            string     `gorm:"size:128;not null"`
	Audience                 string     `gorm:"size:255;not null"`
	TokenFamilyID            string     `gorm:"size:64;not null;index"`
	DeviceSummary            string     `gorm:"size:255"`
	RefreshTokenHash         string     `gorm:"size:64;not null;uniqueIndex"`
	PreviousRefreshTokenHash string     `gorm:"size:64;index"`
	AuthVersion              uint64     `gorm:"not null"`
	ExpiresAt                time.Time  `gorm:"not null;index"`
	LastSeenAt               time.Time  `gorm:"not null"`
	RevokedAt                *time.Time `gorm:"index"`
	RevokedReason            string     `gorm:"size:64;not null;default:''"`
	CreatedAt                time.Time  `gorm:"autoCreateTime"`
	UpdatedAt                time.Time  `gorm:"autoUpdateTime"`
}

func (AuthSession) TableName() string { return "auth_sessions" }

type AuthRefreshTokenUse struct {
	TokenHash     string    `gorm:"primaryKey;size:64"`
	SessionID     string    `gorm:"size:64;not null;index"`
	TokenFamilyID string    `gorm:"size:64;not null;index"`
	UsedAt        time.Time `gorm:"not null"`
}

func (AuthRefreshTokenUse) TableName() string { return "auth_refresh_token_uses" }

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

type WebSocketHandshakeGrant struct {
	ID            string      `gorm:"primaryKey;size:64"`
	SecretHash    string      `gorm:"size:64;not null;uniqueIndex:uni_auth_websocket_handshake_grants_secret_hash"`
	TenantID      string      `gorm:"size:64;not null;index:idx_ws_handshake_grants_principal"`
	PrincipalKind string      `gorm:"size:128;not null;index:idx_ws_handshake_grants_principal"`
	PrincipalID   string      `gorm:"size:255;not null;index:idx_ws_handshake_grants_principal"`
	SessionID     string      `gorm:"size:64;not null;index"`
	Session       AuthSession `gorm:"foreignKey:SessionID;references:ID;constraint:OnDelete:CASCADE" json:"-"`
	Realm         string      `gorm:"size:32;not null;check:ck_ws_handshake_grants_realm,realm = 'tenant'"`
	ClientSurface string      `gorm:"size:128;not null"`
	Audience      string      `gorm:"size:255;not null"`
	TargetID      string      `gorm:"size:128;not null"`
	TargetPath    string      `gorm:"size:512;not null"`
	AuthVersion   uint64      `gorm:"not null;check:ck_ws_handshake_grants_auth_version,auth_version > 0"`
	ExpiresAt     time.Time   `gorm:"not null;index"`
	ConsumedAt    *time.Time  `gorm:"index"`
	CreatedAt     time.Time   `gorm:"autoCreateTime"`
}

func (WebSocketHandshakeGrant) TableName() string { return "auth_websocket_handshake_grants" }

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
