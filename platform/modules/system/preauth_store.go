package system

import (
	"context"

	"github.com/yzxingtu2026/ainp/platform/framework/status"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"github.com/yzxingtu2026/ainp/platform/modules/system/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// preAuthStore is the narrow data capability used before an AuthContext exists.
// It deliberately exposes fixed authentication lookups instead of a general DB.
type preAuthStore struct{ db *gorm.DB }

func newPreAuthStore(db *gorm.DB) *preAuthStore { return &preAuthStore{db: db} }

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
