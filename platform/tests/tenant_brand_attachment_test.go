package tests

import (
	"context"
	"testing"

	"github.com/yzxingtu2026/ainp/platform/framework/attachment"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	systemattachments "github.com/yzxingtu2026/ainp/platform/modules/system/attachments"
)

func TestTenantBrandAttachmentPolicy(t *testing.T) {
	policy := systemattachments.NewTenantBrandAttachmentPolicy()
	allowed := auth.WithContext(context.Background(), &auth.Context{Authenticated: true, TenantID: "tenant-a", PrincipalID: "user-a", Permissions: []string{"system.setting_list"}})
	if err := policy.CanAccess(allowed, "tenant-a", "user-a", "tenant-a", systemattachments.TenantBrandLogoField); err != nil {
		t.Fatalf("expected access, got %v", err)
	}
	if err := policy.CanAccess(allowed, "tenant-b", "user-a", "tenant-b", systemattachments.TenantBrandLogoField); err == nil {
		t.Fatal("cross-tenant access must fail")
	}
	bind := attachment.WithResourceLinkOperation(allowed, attachment.ResourceLinkBind)
	if err := policy.CanAccess(bind, "tenant-a", "user-a", "tenant-a", systemattachments.TenantBrandLogoField); err == nil {
		t.Fatal("read-only user must not bind brand logo")
	}
}
