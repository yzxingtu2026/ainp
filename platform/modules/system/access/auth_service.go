package access

import (
	"context"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"github.com/yzxingtu2026/ainp/platform/modules/system/access/model"
	"gorm.io/gorm"
)

type AuthService struct {
	db      *gorm.DB
	preAuth *preAuthStore
}

func NewAuthService(db *gorm.DB, registries ...*action.Registry) *AuthService {
	var actions *action.Registry
	if len(registries) > 0 {
		actions = registries[0]
	}
	return &AuthService{db: db, preAuth: newPreAuthStore(db, actions)}
}

type AccountProfile struct {
	ID                 string     `json:"id"`
	Username           string     `json:"username"`
	DisplayName        string     `json:"display_name"`
	AvatarAttachmentID *string    `json:"avatar_attachment_id,omitempty"`
	TenantID           string     `json:"tenant_id"`
	TenantName         string     `json:"tenant_name,omitempty"`
	OrgID              string     `json:"org_id,omitempty"`
	DeptID             string     `json:"dept_id,omitempty"`
	Roles              []string   `json:"roles"`
	Permissions        []string   `json:"permissions"`
	Realm              auth.Realm `json:"realm"`
}

func (s *AuthService) FindAccountByUsername(ctx context.Context, tenantID, username string) (*model.TenantAccount, error) {
	return s.preAuth.findAccountByUsername(ctx, tenantID, username)
}

func (s *AuthService) FindDefaultTenant(ctx context.Context) (*tenant.Tenant, error) {
	return s.preAuth.findDefaultTenant(ctx)
}

func (s *AuthService) FindTenant(ctx context.Context, tenantID string) (*tenant.Tenant, error) {
	return s.preAuth.findTenant(ctx, tenantID)
}

func (s *AuthService) LoadRolesAndPermissions(ctx context.Context, accountID, tenantID string) (roles []string, perms []string, err error) {
	_, roles, perms, err = s.preAuth.loadAuthorization(ctx, tenantID, accountID, "", 0)
	return roles, perms, err
}

// LoadAuthorization validates the current tenant/principal state and returns a fresh
// authorization snapshot. Token claims are identity inputs, never permission inputs.
func (s *AuthService) LoadAuthorization(ctx context.Context, req auth.AuthorizationRequest) (*auth.AuthorizationSnapshot, error) {
	principalID := req.PrincipalID
	if req.TenantID == "" || principalID == "" || req.Realm != auth.RealmTenant || req.AuthVersion == 0 {
		return nil, auth.ErrAuthorizationInvalid
	}

	u, roles, permissions, err := s.preAuth.loadAuthorization(ctx, req.TenantID, principalID, req.SessionID, req.AuthVersion)
	if err != nil || u == nil {
		return nil, auth.ErrAuthorizationInvalid
	}
	if u.AuthVersion != req.AuthVersion {
		return nil, auth.ErrAuthorizationStale
	}
	return &auth.AuthorizationSnapshot{
		TenantID: u.TenantID, PrincipalID: u.ID, SessionID: req.SessionID, OrgID: u.OrgID, DeptID: u.DeptID,
		Roles: roles, Permissions: permissions, AuthVersion: u.AuthVersion,
	}, nil
}

func (s *AuthService) BumpAuthVersionScoped(db tenant.TenantDatabase, accountID string) error {
	if accountID == "" {
		return auth.ErrAuthorizationInvalid
	}
	return tenant.TenantExec(db, `UPDATE tenant_accounts SET auth_version = auth_version + 1
WHERE tenant_id = ? AND id = ?`, accountID)
}

// BumpAuthVersionForRoleScoped invalidates accounts through a trusted tenant transaction.
func (s *AuthService) BumpAuthVersionForRoleScoped(db tenant.TenantDatabase, roleID string) error {
	if roleID == "" {
		return auth.ErrAuthorizationInvalid
	}
	return tenant.TenantExec(db, `UPDATE tenant_accounts AS u SET auth_version = auth_version + 1
WHERE u.tenant_id = ? AND u.id IN (
  SELECT ur.account_id FROM tenant_account_roles ur JOIN roles r ON r.id = ur.role_id AND r.tenant_id = ur.tenant_id
  WHERE ur.tenant_id = u.tenant_id AND r.id = ?
)`, roleID)
}

func (s *AuthService) BuildProfile(ctx context.Context, u *model.TenantAccount) (*AccountProfile, error) {
	snapshot, err := s.LoadAuthorization(ctx, auth.AuthorizationRequest{
		TenantID: u.TenantID, PrincipalID: u.ID, Realm: auth.RealmTenant, AuthVersion: u.AuthVersion,
	})
	if err != nil {
		return nil, err
	}
	profile := &AccountProfile{
		ID:                 u.ID,
		Username:           u.Username,
		DisplayName:        u.DisplayName,
		AvatarAttachmentID: u.AvatarAttachmentID,
		TenantID:           u.TenantID,
		OrgID:              u.OrgID,
		DeptID:             u.DeptID,
		Roles:              snapshot.Roles,
		Permissions:        snapshot.Permissions,
		Realm:              auth.RealmTenant,
	}
	if t, err := s.FindTenant(ctx, u.TenantID); err == nil && t != nil {
		profile.TenantName = t.Name
	}
	return profile, nil
}
