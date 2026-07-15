package idempotency

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ErrDuplicateKey indicates the same key already exists for this tenant/user/action.
var ErrDuplicateKey = errors.New("idempotency: duplicate key")

// Repository provides CRUD operations on idempotency records.
type Repository struct {
	db *gorm.DB
}

// NewRepository creates a new Repository.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Create inserts a new idempotency record in processing state.
func (r *Repository) Create(ctx context.Context, tenantID, userID, actionID, key, requestHash string, ttl time.Duration) (*Record, error) {
	now := time.Now().UTC()
	rec := &Record{
		ID:             uuid.NewString(),
		TenantID:       tenantID,
		UserID:         userID,
		ActionID:       actionID,
		IdempotencyKey: key,
		RequestHash:    requestHash,
		Status:         StatusProcessing,
		StartedAt:      now,
		ExpiresAt:      now.Add(ttl),
	}
	err := r.db.WithContext(ctx).Create(rec).Error
	if err != nil && isDuplicateKeyError(err) {
		return nil, ErrDuplicateKey
	}
	return rec, err
}

// GetByKey looks up an idempotency record by its composite key.
func (r *Repository) GetByKey(ctx context.Context, tenantID, userID, actionID, key string) (*Record, error) {
	var rec Record
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND user_id = ? AND action_id = ? AND idempotency_key = ?",
			tenantID, userID, actionID, key).
		First(&rec).Error
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
	result := r.db.WithContext(ctx).
		Model(&Record{}).
		Where("id = ? AND status = ? AND started_at < ?", id, StatusProcessing, cutoff).
		Update("started_at", time.Now().UTC())
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
	return r.db.WithContext(ctx).Model(&Record{}).Where("id = ?", id).Updates(updates).Error
}

// CleanupExpired deletes expired idempotency records.
func (r *Repository) CleanupExpired(ctx context.Context) (int64, error) {
	result := r.db.WithContext(ctx).
		Where("expires_at < ?", time.Now().UTC()).
		Delete(&Record{})
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
