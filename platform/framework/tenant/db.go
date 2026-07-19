package tenant

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"gorm.io/gorm"
)

var (
	ErrTenantDBRequired = errors.New("tenant database is required")
	ErrSystemDBRequired = errors.New("system database is required")
	ErrTenantConflict   = errors.New("model tenant conflicts with authenticated tenant")
	ErrUnsafeUpsert     = errors.New("tenant upsert conflict target must include tenant column")
	ErrUnsafeQuery      = errors.New("raw tenant query is not allowed")
)

const accessModeKey = "ainp:tenant-access-mode"

type accessMode string

const (
	modeTenant accessMode = "tenant"
	modeSystem accessMode = "system"
)

type TenantDatabase struct {
	db       *gorm.DB
	tenantID string
}
type SystemDatabase struct{ db *gorm.DB }
type GuardedDatabase struct{ db *gorm.DB }

// GuardedDB returns an opaque runtime handle whose GORM operations always run
// with tenant callbacks enabled. It is intended for registered tenant-owned
// models that still need domain-specific GORM queries; new generic CRUD should
// prefer TenantDB plus Repository.
func GuardedDB(db *gorm.DB) GuardedDatabase {
	if db == nil {
		return GuardedDatabase{}
	}
	return GuardedDatabase{db: db.Session(&gorm.Session{NewDB: true})}
}

func (d GuardedDatabase) Available() bool { return d.db != nil }

func (d GuardedDatabase) WithContext(ctx context.Context) *gorm.DB {
	if d.db == nil {
		db := &gorm.DB{}
		_ = db.AddError(ErrTenantDBRequired)
		return db
	}
	return accessDB(d.db, ctx, modeTenant)
}

func (d GuardedDatabase) Tenant(ctx context.Context) (TenantDatabase, error) {
	if d.db == nil {
		return TenantDatabase{}, ErrTenantDBRequired
	}
	return TenantDB(ctx, d.db)
}

// MigrationDB marks schema installation access explicitly. It must not be used
// by request, worker, setup-data, or other runtime business paths.
func MigrationDB(db *gorm.DB) (*gorm.DB, error) {
	if db == nil {
		return nil, ErrSystemDBRequired
	}
	return db.Session(&gorm.Session{NewDB: true}).Set(accessModeKey, modeSystem), nil
}

func TenantDB(ctx context.Context, db *gorm.DB) (TenantDatabase, error) {
	ac, ok := auth.FromContext(ctx)
	if !ok || ac == nil || !ac.Authenticated || auth.NormalizeRealm(ac.Realm) != auth.RealmTenant || ac.TenantID == "" {
		return TenantDatabase{}, ErrTenantContextRequired
	}
	if db == nil {
		return TenantDatabase{}, ErrTenantDBRequired
	}
	scoped := accessDB(db, ctx, modeTenant)
	return TenantDatabase{db: scoped, tenantID: ac.TenantID}, nil
}

func SystemDB(ctx context.Context, db *gorm.DB) (SystemDatabase, error) {
	ac, ok := auth.FromContext(ctx)
	if !ok || ac == nil || !ac.Authenticated || ac.Realm != auth.RealmPlatform || !ac.IsSystemOperator {
		return SystemDatabase{}, ErrPlatformRealmRequired
	}
	if db == nil {
		return SystemDatabase{}, ErrSystemDBRequired
	}
	return SystemDatabase{db: accessDB(db, ctx, modeSystem)}, nil
}

func accessDB(db *gorm.DB, ctx context.Context, mode accessMode) *gorm.DB {
	scoped := db.Session(&gorm.Session{NewDB: true}).WithContext(ctx).Set(accessModeKey, mode)
	return scoped.Session(&gorm.Session{})
}

func (d TenantDatabase) Transaction(fn func(TenantDatabase) error) error {
	if d.db == nil {
		return ErrTenantDBRequired
	}
	return d.db.Transaction(func(tx *gorm.DB) error {
		return fn(TenantDatabase{db: tx.Set(accessModeKey, modeTenant), tenantID: d.tenantID})
	})
}
func (d SystemDatabase) Transaction(fn func(SystemDatabase) error) error {
	if d.db == nil {
		return ErrSystemDBRequired
	}
	return d.db.Transaction(func(tx *gorm.DB) error { return fn(SystemDatabase{db: tx.Set(accessModeKey, modeSystem)}) })
}

func TenantRaw[T any](d TenantDatabase, sql string, args ...any) ([]T, error) {
	if d.db == nil {
		return nil, ErrTenantDBRequired
	}
	if err := requireTenantSQL(sql); err != nil {
		return nil, err
	}
	var out []T
	trustedArgs := append([]any{d.tenantID}, args...)
	err := d.db.Raw(sql, trustedArgs...).Scan(&out).Error
	return out, err
}
func TenantExec(d TenantDatabase, sql string, args ...any) error {
	if d.db == nil {
		return ErrTenantDBRequired
	}
	if err := requireTenantSQL(sql); err != nil {
		return err
	}
	trustedArgs := append([]any{d.tenantID}, args...)
	return d.db.Exec(sql, trustedArgs...).Error
}
func SystemRaw[T any](d SystemDatabase, sql string, args ...any) ([]T, error) {
	if d.db == nil {
		return nil, ErrSystemDBRequired
	}
	if err := requireParameterized(sql, args); err != nil {
		return nil, err
	}
	var out []T
	err := d.db.Raw(sql, args...).Scan(&out).Error
	return out, err
}
func SystemExec(d SystemDatabase, sql string, args ...any) error {
	if d.db == nil {
		return ErrSystemDBRequired
	}
	if err := requireParameterized(sql, args); err != nil {
		return err
	}
	return d.db.Exec(sql, args...).Error
}

func modeAndTenant(db *gorm.DB) (accessMode, string, error) {
	v, ok := db.Get(accessModeKey)
	if !ok {
		return "", "", ErrTenantDBRequired
	}
	mode, ok := v.(accessMode)
	if !ok {
		return "", "", ErrTenantDBRequired
	}
	if mode == modeSystem {
		return mode, "", nil
	}
	ac, ok := auth.FromContext(db.Statement.Context)
	if !ok || ac == nil || !ac.Authenticated || ac.Realm != auth.RealmTenant || ac.TenantID == "" {
		return "", "", ErrTenantContextRequired
	}
	return mode, ac.TenantID, nil
}

func requireParameterized(sql string, args []any) error {
	if strings.TrimSpace(sql) == "" || len(args) == 0 || (!strings.Contains(sql, "?") && !strings.Contains(sql, "$")) {
		return fmt.Errorf("%w: SQL must use bound arguments", ErrUnsafeQuery)
	}
	return nil
}

func requireTenantSQL(sql string) error {
	lower := strings.ToLower(sql)
	if strings.TrimSpace(sql) == "" || !strings.Contains(lower, "tenant_id") || (!strings.Contains(sql, "?") && !strings.Contains(sql, "$1")) {
		return fmt.Errorf("%w: SQL must bind trusted tenant_id as its first argument", ErrUnsafeQuery)
	}
	return nil
}
