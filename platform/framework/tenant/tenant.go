package tenant

import (
	"errors"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/status"
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
