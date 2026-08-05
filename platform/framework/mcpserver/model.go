package mcpserver

import (
	"sync"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
)

const (
	Audience       = "ainp:mcp"
	ScopeRead      = "actions:read"
	ScopeWrite     = "actions:write"
	ScopeSensitive = "actions:sensitive"
)

var registerModelsOnce sync.Once

func registerModels() {
	registerModelsOnce.Do(func() {
		_ = tenant.DefaultRegistry.Register(Client{}, tenant.ModelOptions{Kind: tenant.GlobalModel, SystemAccess: true})
		_ = tenant.DefaultRegistry.Register(Grant{}, tenant.ModelOptions{Kind: tenant.TenantOwned, SystemAccess: true})
		_ = tenant.DefaultRegistry.Register(AuthorizationCode{}, tenant.ModelOptions{Kind: tenant.GlobalModel, SystemAccess: true})
		_ = tenant.DefaultRegistry.Register(TokenFamily{}, tenant.ModelOptions{Kind: tenant.GlobalModel, SystemAccess: true})
		_ = tenant.DefaultRegistry.Register(Confirmation{}, tenant.ModelOptions{Kind: tenant.TenantOwned, SystemAccess: true})
	})
}

type Client struct {
	ID           string    `gorm:"primaryKey;size:64" json:"client_id"`
	Name         string    `gorm:"size:160;not null" json:"client_name"`
	RedirectURIs []byte    `gorm:"type:jsonb;not null" json:"-"`
	CreatedAt    time.Time `gorm:"autoCreateTime" json:"created_at"`
}

func (Client) TableName() string { return "mcp_oauth_clients" }

type Grant struct {
	ID          string    `gorm:"primaryKey;size:64"`
	TenantID    string    `gorm:"size:64;not null;index"`
	AccountID   string    `gorm:"size:64;not null;index"`
	ClientID    string    `gorm:"size:64;not null;index"`
	Scopes      string    `gorm:"size:128;not null"`
	AuthVersion uint64    `gorm:"not null"`
	Status      string    `gorm:"size:16;not null"`
	ExpiresAt   time.Time `gorm:"not null;index"`
	LastUsedAt  *time.Time
	RevokedAt   *time.Time `gorm:"index"`
	CreatedAt   time.Time  `gorm:"autoCreateTime"`
	UpdatedAt   time.Time  `gorm:"autoUpdateTime"`
}

func (Grant) TableName() string { return "mcp_oauth_grants" }

type AuthorizationCode struct {
	ID            string    `gorm:"primaryKey;size:64"`
	CodeHash      string    `gorm:"size:64;not null;uniqueIndex"`
	GrantID       string    `gorm:"size:64;not null;index"`
	ClientID      string    `gorm:"size:64;not null"`
	RedirectURI   string    `gorm:"size:1024;not null"`
	CodeChallenge string    `gorm:"size:128;not null"`
	ExpiresAt     time.Time `gorm:"not null;index"`
	ConsumedAt    *time.Time
	CreatedAt     time.Time `gorm:"autoCreateTime"`
}

func (AuthorizationCode) TableName() string { return "mcp_oauth_authorization_codes" }

type TokenFamily struct {
	ID                       string     `gorm:"primaryKey;size:64"`
	GrantID                  string     `gorm:"size:64;not null;index"`
	RefreshTokenHash         string     `gorm:"size:64;not null;uniqueIndex"`
	PreviousRefreshTokenHash string     `gorm:"size:64;index"`
	ExpiresAt                time.Time  `gorm:"not null;index"`
	RevokedAt                *time.Time `gorm:"index"`
	CreatedAt                time.Time  `gorm:"autoCreateTime"`
	UpdatedAt                time.Time  `gorm:"autoUpdateTime"`
}

func (TokenFamily) TableName() string { return "mcp_oauth_token_families" }

type Confirmation struct {
	ID                  string    `gorm:"primaryKey;size:64"`
	TenantID            string    `gorm:"size:64;not null;index"`
	AccountID           string    `gorm:"size:64;not null;index"`
	GrantID             string    `gorm:"size:64;not null;index"`
	ClientID            string    `gorm:"size:64;not null;index"`
	ActionID            string    `gorm:"size:128;not null"`
	ToolName            string    `gorm:"size:128;not null"`
	SchemaHash          string    `gorm:"size:64;not null"`
	ArgumentsHash       string    `gorm:"size:64;not null"`
	ArgumentsCiphertext string    `gorm:"type:text;not null"`
	RecoveryHash        string    `gorm:"size:64;not null;uniqueIndex"`
	Title               string    `gorm:"size:160;not null"`
	Description         string    `gorm:"size:512;not null"`
	Risk                string    `gorm:"size:16;not null"`
	SideEffect          string    `gorm:"size:16;not null"`
	Status              string    `gorm:"size:16;not null;index"`
	ExpiresAt           time.Time `gorm:"not null;index"`
	DecidedAt           *time.Time
	ConsumedAt          *time.Time
	CreatedAt           time.Time `gorm:"autoCreateTime"`
	UpdatedAt           time.Time `gorm:"autoUpdateTime"`
}

func (Confirmation) TableName() string { return "mcp_action_confirmations" }
