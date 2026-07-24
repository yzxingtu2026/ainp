package system

import (
	"context"

	"github.com/yzxingtu2026/ainp/platform/framework/attachment"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
)

const (
	UserProfileAttachmentResourceType = "system.user_profile"
	UserProfileAvatarField            = "avatar"
)

type userProfileAttachmentPolicy struct{}

func (userProfileAttachmentPolicy) ResourceType() string { return UserProfileAttachmentResourceType }

func (userProfileAttachmentPolicy) CanAccess(ctx context.Context, tenantID, userID, resourceID, field string) error {
	ac := auth.MustFromContext(ctx)
	if !ac.Authenticated || ac.TenantID != tenantID || ac.UserID != userID || resourceID != userID || field != UserProfileAvatarField {
		return errs.New(errs.CodeNotFound, "user avatar not found")
	}
	return nil
}

var _ attachment.AccessPolicy = userProfileAttachmentPolicy{}
