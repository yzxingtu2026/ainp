package revision

import (
	"errors"
	"fmt"

	"gorm.io/gorm"
)

// ErrRevisionConflict indicates an optimistic lock failure — the stored revision
// does not match the expected revision supplied by the caller.
var ErrRevisionConflict = errors.New("revision conflict: data was updated by another operation")

// Versioned is implemented by models that carry an optimistic revision counter.
type Versioned interface {
	GetRevision() int64
}

// ConditionalUpdate atomically updates a row only if its current revision matches
// the expected value. On success, the revision is incremented by 1.
// Returns ErrRevisionConflict if the stored revision does not match.
func ConditionalUpdate(db *gorm.DB, table string, pk any, expectedRevision int64, updates map[string]any) error {
	if expectedRevision < 0 {
		return fmt.Errorf("expected revision must be >= 0")
	}
	updates["revision"] = gorm.Expr("revision + 1")

	result := db.Table(table).
		Where("id = ? AND revision = ?", pk, expectedRevision).
		Updates(updates)

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrRevisionConflict
	}
	return nil
}

// WithExpectedRevision returns a GORM scope that adds a revision WHERE clause.
// Use with: db.Scopes(WithExpectedRevision(expected)).Updates(&model)
func WithExpectedRevision(expected int64) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("revision = ?", expected)
	}
}

// CheckUpdateResult inspects a GORM update result for optimistic lock failure.
// If RowsAffected == 0 and no error, returns ErrRevisionConflict.
func CheckUpdateResult(result *gorm.DB) error {
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrRevisionConflict
	}
	return nil
}
