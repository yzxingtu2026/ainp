package system

import (
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/authentication"
	"github.com/yzxingtu2026/ainp/platform/framework/capability"
	"github.com/yzxingtu2026/ainp/platform/modules/system/access"
	"gorm.io/gorm"
)

type (
	AuthService            = access.AuthService
	AccountProfile         = access.AccountProfile
	PasswordAuthenticator  = access.PasswordAuthenticator
	SessionService         = access.SessionService
	SessionTokens          = access.SessionTokens
	UnifiedAuthService     = access.UnifiedAuthService
	AuthenticateRequest    = access.AuthenticateRequest
	ChallengeRequest       = access.ChallengeRequest
	RefreshRequest         = access.RefreshRequest
	LegacyLoginRequest     = access.LegacyLoginRequest
	SessionView            = access.SessionView
	RoleService            = access.RoleService
	RoleView               = access.RoleView
	PermissionCatalogItem  = access.PermissionCatalogItem
	PermissionCatalogGroup = access.PermissionCatalogGroup
	AccountService         = access.AccountService
	AccountSelfProfileView = access.AccountSelfProfileView
	AccountRoleBrief       = access.AccountRoleBrief
	AccountView            = access.AccountView

	// Deprecated: use AccountProfile.
	UserProfile = AccountProfile
	// Deprecated: use AccountService.
	UserService = AccountService
	// Deprecated: use AccountRoleBrief.
	UserRoleBrief = AccountRoleBrief
	// Deprecated: use AccountView.
	UserView = AccountView
	// Deprecated: use AccountSelfProfileView.
	SelfProfileView = AccountSelfProfileView
)

func NewAuthService(db *gorm.DB, registries ...*action.Registry) *AuthService {
	return access.NewAuthService(db, registries...)
}

func NewPasswordAuthenticator(service *AuthService) *PasswordAuthenticator {
	return access.NewPasswordAuthenticator(service)
}

func NewSessionService(db *gorm.DB, tokens *auth.TokenService, service *AuthService, ttl ...time.Duration) *SessionService {
	return access.NewSessionService(db, tokens, service, ttl...)
}

func NewUnifiedAuthService(db *gorm.DB, registry *authentication.Registry, authService *AuthService, sessions *SessionService, limiter authentication.RateLimiter) *UnifiedAuthService {
	return access.NewUnifiedAuthService(db, registry, authService, sessions, limiter)
}

func NewRoleService(db *gorm.DB, authService *AuthService, actions *action.Registry, capabilities *capability.Registry) *RoleService {
	return access.NewRoleService(db, authService, actions, capabilities)
}

func NewAccountService(db *gorm.DB, authService *AuthService) *AccountService {
	return access.NewAccountService(db, authService)
}

// NewUserService is kept for source compatibility.
// Deprecated: use NewAccountService.
func NewUserService(db *gorm.DB, authService *AuthService) *UserService {
	return NewAccountService(db, authService)
}

func ValidatePassword(password string) error { return access.ValidatePassword(password) }
func AssignablePermissions(actions *action.Registry) []string {
	return access.AssignablePermissions(actions)
}

func registerAuthenticationActions(a *app.App, service *UnifiedAuthService, auditOn, auditOff bool) error {
	return access.RegisterAuthenticationActions(a, service, auditOn, auditOff)
}

func registerReferenceResolvers(a *app.App) error { return access.RegisterReferenceResolvers(a) }
func registerRoleActions(a *app.App, service *RoleService) error {
	return access.RegisterRoleActions(a, service)
}
func registerAccountActions(a *app.App, service *AccountService) error {
	return access.RegisterAccountActions(a, service)
}
