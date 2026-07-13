package tenant

import (
	"context"

	"gorm.io/gorm"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
)

// Tenant 模型预留（含自定义域名扩展字段），Phase 1 可不落业务表读写。
type Tenant struct {
	ID                 string `gorm:"primaryKey;size:64"`
	Name               string `gorm:"size:128"`
	Status             string `gorm:"size:32"` // enabled | disabled
	PrimaryDomain      string `gorm:"size:255"`
	CustomDomainsJSON  string `gorm:"type:text"` // JSON 数组预留
	DomainVerifyStatus string `gorm:"size:32"`
}

func ScopeByTenant(ctx context.Context) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		ac, ok := auth.FromContext(ctx)
		if !ok || ac == nil || ac.TenantID == "" || ac.IsSystemOperator {
			return db
		}
		return db.Where("tenant_id = ?", ac.TenantID)
	}
}
