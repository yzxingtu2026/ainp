package system

import (
	"github.com/yzxingtu2026/ainp/platform/framework/notification"
)

// GormOverrideProvider bridges notification.OverrideProvider to the repository.
type GormOverrideProvider struct {
	repo *NotificationRepository
}

func NewGormOverrideProvider(repo *NotificationRepository) *GormOverrideProvider {
	return &GormOverrideProvider{repo: repo}
}

// GetOverride implements notification.OverrideProvider.
func (p *GormOverrideProvider) GetOverride(tenantID, definitionKey, channel, locale string) (*notification.TemplateOverride, error) {
	// We need a context-free lookup; the repository methods use tenant scope from context,
	// so we do a direct query here for the override provider contract.
	var row NotificationTemplateOverride
	err := p.repo.db.
		Where("tenant_id = ? AND definition_key = ? AND channel = ? AND locale = ?",
			tenantID, definitionKey, channel, locale).
		First(&row).Error
	if err != nil {
		// Not found is expected, return nil
		return nil, nil
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
