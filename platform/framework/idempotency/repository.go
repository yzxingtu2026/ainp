package idempotency

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"gorm.io/gorm"
)

// ErrDuplicateKey indicates the same key already exists for this tenant/principal/action.
var ErrDuplicateKey = errors.New("idempotency: duplicate key")

// Repository provides CRUD operations on idempotency records.
type Repository struct {
	db *gorm.DB
}

var recordModelRegistration sync.Once

// NewRepository creates a new Repository.
func NewRepository(db *gorm.DB) *Repository {
	recordModelRegistration.Do(func() {
		if err := tenant.DefaultRegistry.Register(Record{}, tenant.ModelOptions{
			Kind: tenant.TenantOwned, InjectOnCreate: true, SystemAccess: true,
		}); err != nil {
			panic(err)
		}
	})
	return &Repository{db: db}
}

func (r *Repository) tenantRepository(ctx context.Context, tenantID string) (*tenant.Repository[Record], error) {
	ac := auth.MustFromContext(ctx)
	if ac.TenantID != tenantID {
		return nil, tenant.ErrTenantConflict
	}
	tdb, err := tenant.TenantDB(ctx, r.db)
	if err != nil {
		return nil, err
	}
	return tenant.NewTenantRepository[Record](tdb)
}

func (r *Repository) currentTenantRepository(ctx context.Context) (*tenant.Repository[Record], error) {
	ac := auth.MustFromContext(ctx)
	return r.tenantRepository(ctx, ac.TenantID)
}

// Create inserts a new idempotency record in processing state.
func (r *Repository) Create(ctx context.Context, tenantID, principalID, actionID, key, requestHash string, ttl time.Duration) (*Record, error) {
	now := time.Now().UTC()
	rec := &Record{
		ID:             uuid.NewString(),
		PrincipalID:    principalID,
		ActionID:       actionID,
		IdempotencyKey: key,
		RequestHash:    requestHash,
		Status:         StatusProcessing,
		StartedAt:      now,
		ExpiresAt:      now.Add(ttl),
	}
	repo, err := r.tenantRepository(ctx, tenantID)
	if err == nil {
		err = repo.Create(rec)
	}
	if err != nil && isDuplicateKeyError(err) {
		return nil, ErrDuplicateKey
	}
	return rec, err
}

// GetByKey looks up an idempotency record by its composite key.
func (r *Repository) GetByKey(ctx context.Context, tenantID, principalID, actionID, key string) (*Record, error) {
	repo, err := r.tenantRepository(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	rec, err := repo.First("principal_id = ? AND action_id = ? AND idempotency_key = ?", principalID, actionID, key)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

// ClaimProcessing atomically takes over a stale processing record.
// Returns true if the record was successfully claimed.
func (r *Repository) ClaimProcessing(ctx context.Context, id string, timeout time.Duration) (bool, error) {
	cutoff := time.Now().UTC().Add(-timeout)
	repo, err := r.currentTenantRepository(ctx)
	if err != nil {
		return false, err
	}
	result := repo.Where("id = ? AND status = ? AND started_at < ?", id, StatusProcessing, cutoff).
		Updates(map[string]any{"started_at": time.Now().UTC()})
	return result.RowsAffected > 0, result.Error
}

// Complete marks a record as completed with optional cached response/error.
func (r *Repository) Complete(ctx context.Context, id string, status Status, responseJSON, errorJSON []byte) error {
	now := time.Now().UTC()
	updates := map[string]any{
		"status":       status,
		"completed_at": now,
	}
	if responseJSON != nil {
		updates["response_json"] = responseJSON
	}
	if errorJSON != nil {
		updates["error_json"] = errorJSON
	}
	repo, err := r.currentTenantRepository(ctx)
	if err != nil {
		return err
	}
	return repo.Where("id = ?", id).Updates(updates).Error
}

// CleanupExpired deletes expired idempotency records.
func (r *Repository) CleanupExpired(ctx context.Context) (int64, error) {
	sdb, err := tenant.SystemDB(ctx, r.db)
	if err != nil {
		return 0, err
	}
	repo, _ := tenant.NewSystemRepository[Record](sdb)
	result := repo.DeleteWhere("expires_at < ?", time.Now().UTC())
	return result.RowsAffected, result.Error
}

func isDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}
	// PostgreSQL unique_violation: SQLSTATE 23505
	return containsAny(err.Error(), "duplicate key", "unique constraint", "23505")
}

func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if len(s) >= len(sub) {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}
