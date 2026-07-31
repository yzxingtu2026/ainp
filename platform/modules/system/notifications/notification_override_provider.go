package notifications

import (
	"context"
	"errors"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/notification"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"gorm.io/gorm"
)

// GormOverrideProvider bridges notification.OverrideProvider to the repository.
type GormOverrideProvider struct {
	repo *NotificationRepository
}

func NewGormOverrideProvider(repo *NotificationRepository) *GormOverrideProvider {
	return &GormOverrideProvider{repo: repo}
}

// GetOverride implements notification.OverrideProvider.
func (p *GormOverrideProvider) GetOverride(ctx context.Context, tenantID, definitionKey, channel, locale string) (*notification.TemplateOverride, error) {
	ac, ok := auth.FromContext(ctx)
	if !ok || ac == nil || ac.TenantID != tenantID {
		return nil, tenant.ErrTenantConflict
	}
	repo, err := p.repo.overrideRepository(ctx)
	if err != nil {
		return nil, err
	}
	row, err := repo.First("definition_key = ? AND channel = ? AND locale = ?", definitionKey, channel, locale)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &notification.TemplateOverride{
		TenantID:      row.TenantID,
		DefinitionKey: row.DefinitionKey,
		Channel:       row.Channel,
		Locale:        row.Locale,
		Version:       row.Version,
		Subject:       row.Subject,
		Body:          row.Body,
		ConfigJSON:    string(row.ConfigJSON),
		Enabled:       row.Enabled,
	}, nil
}

var _ notification.OverrideProvider = (*GormOverrideProvider)(nil)
