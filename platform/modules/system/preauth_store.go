package system

import (
	"context"
	"sort"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"github.com/yzxingtu2026/ainp/platform/modules/system/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// preAuthStore is the narrow data capability used before an AuthContext exists.
// It deliberately exposes fixed authentication lookups instead of a general DB.
type preAuthStore struct {
	db      *gorm.DB
	actions *action.Registry
}

func newPreAuthStore(db *gorm.DB, registries ...*action.Registry) *preAuthStore {
	var actions *action.Registry
	if len(registries) > 0 {
		actions = registries[0]
	}
	return &preAuthStore{db: db, actions: actions}
}

func (s *preAuthStore) findDefaultTenant(ctx context.Context) (*tenant.Tenant, error) {
	var row tenant.Tenant
	err := s.db.WithContext(ctx).Where("status = ?", status.ActivationEnabled).Order("created_at ASC, id ASC").First(&row).Error
	return &row, err
}

func (s *preAuthStore) findTenant(ctx context.Context, tenantID string) (*tenant.Tenant, error) {
	var row tenant.Tenant
	err := s.db.WithContext(ctx).Where("id = ?", tenantID).First(&row).Error
	return &row, err
}

func (s *preAuthStore) findUserByUsername(ctx context.Context, tenantID, username string) (*model.User, error) {
	var row model.User
	err := s.db.WithContext(ctx).Where("tenant_id = ? AND username = ?", tenantID, username).First(&row).Error
	return &row, err
}

func (s *preAuthStore) findUserByID(ctx context.Context, db *gorm.DB, tenantID, userID string) (*model.User, error) {
	var row model.User
	err := db.WithContext(ctx).Where("tenant_id = ? AND id = ?", tenantID, userID).First(&row).Error
	return &row, err
}

func (s *preAuthStore) findIdentity(ctx context.Context, tenantID, provider, subject string) (*model.AuthIdentity, error) {
	var row model.AuthIdentity
	err := s.db.WithContext(ctx).Where("tenant_id = ? AND provider = ? AND subject = ? AND status = ?", tenantID, provider, subject, status.ActivationEnabled).First(&row).Error
	return &row, err
}

func (s *preAuthStore) upsertIdentity(ctx context.Context, row *model.AuthIdentity) error {
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "tenant_id"}, {Name: "provider"}, {Name: "subject"}},
		DoUpdates: clause.Assignments(map[string]any{"verified_at": row.VerifiedAt, "normalized_identifier": row.NormalizedIdentifier}),
	}).Create(row).Error
}

func (s *preAuthStore) createSession(ctx context.Context, row *model.AuthSession) error {
	return s.db.WithContext(ctx).Create(row).Error
}

func (s *preAuthStore) rotateSession(ctx context.Context, sessionID string, fn func(*gorm.DB, *model.AuthSession) error) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row model.AuthSession
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", sessionID).First(&row).Error; err != nil {
			return err
		}
		return fn(tx, &row)
	})
}

func (s *preAuthStore) loadAuthorization(ctx context.Context, tenantID, userID, sessionID string, authVersion uint64) (*model.User, []string, []string, error) {
	tenantRow, err := s.findTenant(ctx, tenantID)
	if err != nil || tenantRow.Status != status.ActivationEnabled {
		return nil, nil, nil, err
	}
	user, err := s.findUserByID(ctx, s.db, tenantID, userID)
	if err != nil || user.Status != status.ActivationEnabled {
		return nil, nil, nil, err
	}
	if sessionID != "" {
		var count int64
		err = s.db.WithContext(ctx).Model(&model.AuthSession{}).
			Where("tenant_id = ? AND id = ? AND user_id = ? AND auth_version = ? AND revoked_at IS NULL AND expires_at > ?", tenantID, sessionID, userID, authVersion, time.Now().UTC()).
			Count(&count).Error
		if err != nil || count != 1 {
			return nil, nil, nil, err
		}
	}
	var rows []struct {
		Role       string `gorm:"column:role_code"`
		Permission string `gorm:"column:permission"`
		Builtin    bool   `gorm:"column:is_builtin"`
	}
	err = s.db.WithContext(ctx).Raw(`SELECT r.code AS role_code, r.is_builtin, rp.permission
FROM user_roles ur JOIN roles r ON r.tenant_id = ur.tenant_id AND r.id = ur.role_id
LEFT JOIN role_permissions rp ON rp.role_id = r.id
WHERE ur.tenant_id = ? AND ur.user_id = ? AND r.status = ?
ORDER BY r.code, rp.permission`, tenantID, userID, status.ActivationEnabled).Scan(&rows).Error
	if err != nil {
		return nil, nil, nil, err
	}
	roleSet, permissionSet := map[string]struct{}{}, map[string]struct{}{}
	builtinAdmin := false
	for _, row := range rows {
		roleSet[row.Role] = struct{}{}
		builtinAdmin = builtinAdmin || (row.Builtin && row.Role == BuiltinAdminCode)
		if row.Permission != "" {
			permissionSet[row.Permission] = struct{}{}
		}
	}
	if builtinAdmin {
		for _, permission := range AssignablePermissions(s.actions) {
			permissionSet[permission] = struct{}{}
		}
	}
	roles, permissions := make([]string, 0, len(roleSet)), make([]string, 0, len(permissionSet))
	for role := range roleSet {
		roles = append(roles, role)
	}
	for permission := range permissionSet {
		permissions = append(permissions, permission)
	}
	sort.Strings(roles)
	sort.Strings(permissions)
	return user, roles, permissions, nil
}
