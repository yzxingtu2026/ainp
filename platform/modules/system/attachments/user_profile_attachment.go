package attachments

import (
	"context"

	"github.com/yzxingtu2026/ainp/platform/framework/attachment"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
)

const (
	AccountProfileAttachmentResourceType = "system.account_profile"
	AccountProfileAvatarField            = "avatar"
	legacyUserProfileResourceType        = "system.user_profile"
)

type accountProfileAttachmentPolicy struct{ resourceType string }

func NewAccountProfileAttachmentPolicies() []attachment.AccessPolicy {
	return []attachment.AccessPolicy{
		accountProfileAttachmentPolicy{resourceType: AccountProfileAttachmentResourceType},
		accountProfileAttachmentPolicy{resourceType: legacyUserProfileResourceType},
	}
}

func (p accountProfileAttachmentPolicy) ResourceType() string { return p.resourceType }

func (accountProfileAttachmentPolicy) CanAccess(ctx context.Context, tenantID, userID, resourceID, field string) error {
	ac := auth.MustFromContext(ctx)
	if !ac.Authenticated || ac.TenantID != tenantID || ac.PrincipalID != userID || resourceID != userID || field != AccountProfileAvatarField {
		return errs.New(errs.CodeNotFound, "account avatar not found")
	}
	return nil
}

var _ attachment.AccessPolicy = accountProfileAttachmentPolicy{}
