package attachments

import (
	"context"
	"testing"

	"github.com/yzxingtu2026/ainp/platform/framework/attachment"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
)

func TestTenantBrandAttachmentPolicy(t *testing.T) {
	policy := NewTenantBrandAttachmentPolicy()
	allowed := auth.WithContext(context.Background(), &auth.Context{Authenticated: true, TenantID: "tenant-a", PrincipalID: "user-a", Permissions: []string{"system.setting_list"}})
	if err := policy.CanAccess(allowed, "tenant-a", "user-a", "tenant-a", TenantBrandLogoField); err != nil {
		t.Fatalf("expected access, got %v", err)
	}
	if err := policy.CanAccess(allowed, "tenant-b", "user-a", "tenant-b", TenantBrandLogoField); err == nil {
		t.Fatal("cross-tenant access must fail")
	}
	readonly := auth.WithContext(context.Background(), &auth.Context{Authenticated: true, TenantID: "tenant-a", PrincipalID: "user-a", Permissions: []string{"system.setting_list"}})
	if err := policy.CanAccess(readonly, "tenant-a", "user-a", "tenant-a", TenantBrandLogoField); err != nil {
		t.Fatalf("read-only user must download brand logo: %v", err)
	}
	bind := attachment.WithResourceLinkOperation(readonly, attachment.ResourceLinkBind)
	if err := policy.CanAccess(bind, "tenant-a", "user-a", "tenant-a", TenantBrandLogoField); err == nil {
		t.Fatal("read-only user must not bind brand logo")
	}
}
