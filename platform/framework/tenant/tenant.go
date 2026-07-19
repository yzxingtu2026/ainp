package tenant

import (
	"context"
	"errors"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
	"gorm.io/gorm"
)

var (
	ErrTenantContextRequired = errors.New("tenant context is required")
	ErrPlatformRealmRequired = errors.New("platform realm is required")
)

// Tenant 模型预留自定义域名扩展字段；Host 解析本阶段不实现。
type Tenant struct {
	ID                 string                  `gorm:"primaryKey;size:64"`
	Name               string                  `gorm:"size:128;not null"`
	Status             status.ActivationStatus `gorm:"size:32;not null;default:enabled"`
	PrimaryDomain      string                  `gorm:"size:255"`
	CustomDomainsJSON  string                  `gorm:"type:text"` // JSON 数组预留
	DomainVerifyStatus string                  `gorm:"size:32"`
	CreatedAt          time.Time               `gorm:"autoCreateTime"`
	UpdatedAt          time.Time               `gorm:"autoUpdateTime"`
}

func (Tenant) TableName() string { return "tenants" }

// SystemScope is the only framework-provided unscoped tenant-data entrypoint.
// Callers must already be running in an authenticated platform realm.
func SystemScope(ctx context.Context) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		ac, ok := auth.FromContext(ctx)
		if !ok || ac == nil || !ac.Authenticated || ac.Realm != auth.RealmPlatform || !ac.IsSystemOperator {
			_ = db.AddError(ErrPlatformRealmRequired)
			return db
		}
		return db
	}
}
