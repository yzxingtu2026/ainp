package system

import (
	"context"
	"sort"

	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"github.com/yzxingtu2026/ainp/platform/modules/system/model"
	"gorm.io/gorm"
)

type AuthService struct {
	db *gorm.DB
}

func NewAuthService(db *gorm.DB) *AuthService {
	return &AuthService{db: db}
}

type UserProfile struct {
	ID          string     `json:"id"`
	Username    string     `json:"username"`
	DisplayName string     `json:"display_name"`
	TenantID    string     `json:"tenant_id"`
	TenantName  string     `json:"tenant_name,omitempty"`
	OrgID       string     `json:"org_id,omitempty"`
	DeptID      string     `json:"dept_id,omitempty"`
	Roles       []string   `json:"roles"`
	Permissions []string   `json:"permissions"`
	Realm       auth.Realm `json:"realm"`
}

func (s *AuthService) FindUserByUsername(ctx context.Context, tenantID, username string) (*model.User, error) {
	var u model.User
	err := s.db.WithContext(ctx).Where("tenant_id = ? AND username = ?", tenantID, username).First(&u).Error
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *AuthService) FindDefaultTenant(ctx context.Context) (*tenant.Tenant, error) {
	var t tenant.Tenant
	err := s.db.WithContext(ctx).Where("status = ?", status.ActivationEnabled).Order("created_at ASC, id ASC").First(&t).Error
	return &t, err
}

func (s *AuthService) FindTenant(ctx context.Context, tenantID string) (*tenant.Tenant, error) {
	var t tenant.Tenant
	err := s.db.WithContext(ctx).Where("id = ?", tenantID).First(&t).Error
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *AuthService) LoadRolesAndPermissions(ctx context.Context, userID, tenantID string) (roles []string, perms []string, err error) {
	var userRoles []model.UserRole
	if err = s.db.WithContext(ctx).
		Where("user_id = ? AND tenant_id = ?", userID, tenantID).
		Find(&userRoles).Error; err != nil {
		return nil, nil, err
	}
	if len(userRoles) == 0 {
		return []string{}, []string{}, nil
	}

	roleIDs := make([]string, 0, len(userRoles))
	for _, ur := range userRoles {
		roleIDs = append(roleIDs, ur.RoleID)
	}

	var roleRows []model.Role
	if err = s.db.WithContext(ctx).
		Where("id IN ? AND tenant_id = ? AND status = ?", roleIDs, tenantID, status.ActivationEnabled).
		Find(&roleRows).Error; err != nil {
		return nil, nil, err
	}

	enabledRoleIDs := make([]string, 0, len(roleRows))
	roles = make([]string, 0, len(roleRows))
	for _, r := range roleRows {
		roles = append(roles, r.Code)
		enabledRoleIDs = append(enabledRoleIDs, r.ID)
	}
	sort.Strings(roles)

	if len(enabledRoleIDs) == 0 {
		return roles, []string{}, nil
	}

	var rp []model.RolePermission
	if err = s.db.WithContext(ctx).
		Where("role_id IN ?", enabledRoleIDs).
		Find(&rp).Error; err != nil {
		return nil, nil, err
	}

	seen := map[string]struct{}{}
	for _, p := range rp {
		if _, ok := seen[p.Permission]; ok {
			continue
		}
		seen[p.Permission] = struct{}{}
		perms = append(perms, p.Permission)
	}
	sort.Strings(perms)
	return roles, perms, nil
}

func (s *AuthService) BuildProfile(ctx context.Context, u *model.User) (*UserProfile, error) {
	roles, perms, err := s.LoadRolesAndPermissions(ctx, u.ID, u.TenantID)
	if err != nil {
		return nil, err
	}
	profile := &UserProfile{
		ID:          u.ID,
		Username:    u.Username,
		DisplayName: u.DisplayName,
		TenantID:    u.TenantID,
		OrgID:       u.OrgID,
		DeptID:      u.DeptID,
		Roles:       roles,
		Permissions: perms,
		Realm:       auth.RealmTenant,
	}
	if t, err := s.FindTenant(ctx, u.TenantID); err == nil && t != nil {
		profile.TenantName = t.Name
	}
	return profile, nil
}
