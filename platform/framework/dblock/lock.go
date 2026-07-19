package dblock

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"time"

	"gorm.io/gorm"
)

var tableNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// WithinTx executes fn inside a database transaction.
// If fn returns an error, the transaction is rolled back; otherwise committed.
func WithinTx(ctx context.Context, db *gorm.DB, fn func(tx *gorm.DB) error) error {
	return db.WithContext(ctx).Transaction(fn)
}

// LockForTenant acquires a SELECT ... FOR UPDATE row lock for a single row
// identified by primary key, enforcing tenant isolation.
func LockForTenant(tx *gorm.DB, table string, tenantID string, pk any) error {
	if !tableNamePattern.MatchString(table) {
		return fmt.Errorf("dblock: invalid table name")
	}
	if tenantID == "" {
		return fmt.Errorf("dblock: tenant_id is required for row lock")
	}
	result := tx.Exec(
		fmt.Sprintf("SELECT 1 FROM %s WHERE id = ? AND tenant_id = ? FOR UPDATE", table),
		pk, tenantID,
	)
	if result.Error != nil {
		return fmt.Errorf("dblock: lock failed: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("dblock: row not found or tenant mismatch: table=%s id=%v", table, pk)
	}
	return nil
}

// LockRowsForUpdate acquires SELECT ... FOR UPDATE on multiple rows
// within the same tenant. Rows are locked in stable (sorted) order to reduce deadlock risk.
func LockRowsForUpdate(tx *gorm.DB, table string, tenantID string, ids []string) error {
	if !tableNamePattern.MatchString(table) {
		return fmt.Errorf("dblock: invalid table name")
	}
	if tenantID == "" {
		return fmt.Errorf("dblock: tenant_id is required for row lock")
	}
	if len(ids) == 0 {
		return nil
	}
	sorted := SortLockKeys(ids)
	result := tx.Exec(
		fmt.Sprintf("SELECT 1 FROM %s WHERE id IN ? AND tenant_id = ? ORDER BY id FOR UPDATE", table),
		sorted, tenantID,
	)
	if result.Error != nil {
		return fmt.Errorf("dblock: bulk lock failed: %w", result.Error)
	}
	return nil
}

// WithTimeout wraps fn with a context deadline to prevent long-held locks.
func WithTimeout(ctx context.Context, timeout time.Duration, fn func(ctx context.Context) error) error {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return fn(ctx)
}

// SortLockKeys returns a sorted copy of the given keys for stable lock ordering.
func SortLockKeys(keys []string) []string {
	out := make([]string, len(keys))
	copy(out, keys)
	sort.Strings(out)
	return out
}
