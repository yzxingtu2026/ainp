package attachments

import (
	"context"

	"github.com/yzxingtu2026/ainp/platform/framework/attachment"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
)

const (
	TenantBrandAttachmentResourceType = "system.tenant_brand"
	TenantBrandLogoField              = "logo"
)

type tenantBrandAttachmentPolicy struct{}

func NewTenantBrandAttachmentPolicy() attachment.AccessPolicy { return tenantBrandAttachmentPolicy{} }
func (tenantBrandAttachmentPolicy) ResourceType() string      { return TenantBrandAttachmentResourceType }
func (tenantBrandAttachmentPolicy) CanAccess(ctx context.Context, tenantID, _ string, resourceID, field string) error {
	ac := auth.MustFromContext(ctx)
	if !ac.Authenticated || ac.TenantID != tenantID || resourceID != tenantID || field != TenantBrandLogoField || !ac.HasPermission("system.setting_list") {
		return errs.New(errs.CodeNotFound, "tenant brand logo not found")
	}
	if attachment.ResourceLinkOperationFromContext(ctx) != "" && !ac.HasPermission("system.setting_update") {
		return errs.New(errs.CodeForbidden, "tenant brand logo is read only")
	}
	return nil
}

var _ attachment.AccessPolicy = tenantBrandAttachmentPolicy{}
