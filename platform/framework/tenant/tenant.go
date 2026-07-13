package tenant

import (
	"context"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"gorm.io/gorm"
)

const (
	StatusEnabled  = "enabled"
	StatusDisabled = "disabled"
)

// Tenant 模型预留自定义域名扩展字段；Host 解析本阶段不实现。
type Tenant struct {
	ID                 string    `gorm:"primaryKey;size:64"`
	Name               string    `gorm:"size:128;not null"`
	Status             string    `gorm:"size:32;not null;default:enabled"` // enabled | disabled
	PrimaryDomain      string    `gorm:"size:255"`
	CustomDomainsJSON  string    `gorm:"type:text"` // JSON 数组预留
	DomainVerifyStatus string    `gorm:"size:32"`
	CreatedAt          time.Time `gorm:"autoCreateTime"`
	UpdatedAt          time.Time `gorm:"autoUpdateTime"`
}

func (Tenant) TableName() string { return "tenants" }

func ScopeByTenant(ctx context.Context) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		ac, ok := auth.FromContext(ctx)
		if !ok || ac == nil || ac.TenantID == "" || ac.IsSystemOperator {
			return db
		}
		return db.Where("tenant_id = ?", ac.TenantID)
	}
}
