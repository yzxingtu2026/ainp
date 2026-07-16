package notification

import (
	"context"
	"fmt"
)

// RecipientResolver resolves recipient specs to concrete addresses for a given channel.
// For Phase 1, it resolves tenant user IDs for in-app channel.
type RecipientResolver interface {
	// Resolve validates and resolves a recipient for the given tenant and channel.
	// Returns an error if the recipient is not found, disabled, or cross-tenant.
	Resolve(ctx context.Context, tenantID string, spec RecipientSpec, ch Channel) (*ResolvedRecipient, error)
}

// ResolvedRecipient holds the resolved address for a specific channel.
type ResolvedRecipient struct {
	Type     RecipientType `json:"type"`
	ID       string        `json:"id"`
	TenantID string        `json:"tenant_id"`
	Enabled  bool          `json:"enabled"`

	// Channel-specific addresses (populated by resolver)
	EmailAddress string `json:"email_address,omitempty"`
	PhoneNumber  string `json:"phone_number,omitempty"`
	IMIdentity   string `json:"im_identity,omitempty"`
}

// Validate checks that the resolved recipient is usable.
func (r *ResolvedRecipient) Validate() error {
	if r.ID == "" {
		return fmt.Errorf("notification: resolved recipient id is empty")
	}
	if !r.Enabled {
		return fmt.Errorf("notification: recipient %q is disabled", r.ID)
	}
	return nil
}

// TenantUserResolver is a simple resolver for tenant users (Phase 1 in-app only).
type TenantUserResolver struct {
	// UserExists checks if a user exists and is enabled in the tenant.
	// This is injected by the module layer to avoid framework importing modules.
	UserExists func(ctx context.Context, tenantID, userID string) (bool, error)
}

func NewTenantUserResolver(userExists func(ctx context.Context, tenantID, userID string) (bool, error)) *TenantUserResolver {
	return &TenantUserResolver{UserExists: userExists}
}

func (r *TenantUserResolver) Resolve(ctx context.Context, tenantID string, spec RecipientSpec, ch Channel) (*ResolvedRecipient, error) {
	if spec.Type != RecipientUser {
		return nil, fmt.Errorf("notification: only user recipients are supported in Phase 1, got %q", spec.Type)
	}
	if tenantID == "" {
		return nil, fmt.Errorf("notification: tenant_id is required for recipient resolution")
	}

	enabled, err := r.UserExists(ctx, tenantID, spec.ID)
	if err != nil {
		return nil, fmt.Errorf("notification: user lookup failed: %w", err)
	}
	if !enabled {
		return nil, fmt.Errorf("notification: user %q not found or disabled in tenant %q", spec.ID, tenantID)
	}

	return &ResolvedRecipient{
		Type:     RecipientUser,
		ID:       spec.ID,
		TenantID: tenantID,
		Enabled:  true,
	}, nil
}
