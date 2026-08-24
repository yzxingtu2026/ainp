package tests

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/attachment"
	"github.com/yzxingtu2026/ainp/platform/framework/audit"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/config"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	"github.com/yzxingtu2026/ainp/platform/framework/platformresource"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"github.com/yzxingtu2026/ainp/platform/modules/system"
	systemmodel "github.com/yzxingtu2026/ainp/platform/modules/system/model"
	"gorm.io/gorm"
)

func TestPlatformResourceContractsEnforceTenantPolicyStateAndAudit(t *testing.T) {
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	admin, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "ainp_platform_resource_" + uuid.NewString()[:8]
	if err := admin.Exec(fmt.Sprintf(`CREATE SCHEMA %q`, schema)).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Exec(fmt.Sprintf(`DROP SCHEMA %q CASCADE`, schema)).Error })
	db, err := database.Open(dsn + " search_path=" + schema)
	if err != nil {
		t.Fatal(err)
	}
	if err := system.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	auditor := audit.NewGormRecorder(db)
	if err := auditor.AutoMigrate(); err != nil {
		t.Fatal(err)
	}
	for _, row := range []tenant.Tenant{{ID: "tenant-a", Name: "A", Status: status.ActivationEnabled}, {ID: "tenant-b", Name: "B", Status: status.ActivationEnabled}} {
		if err := db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, row := range []systemmodel.TenantAccount{
		{ID: "actor-a", TenantID: "tenant-a", Username: "actor-a", DisplayName: "Actor A", PasswordHash: "x", Status: status.ActivationEnabled, AuthVersion: 1},
		{ID: "target-a", TenantID: "tenant-a", Username: "target-a", DisplayName: "Target A", PasswordHash: "x", Status: status.ActivationEnabled, AuthVersion: 1},
		{ID: "actor-b", TenantID: "tenant-b", Username: "actor-b", DisplayName: "Actor B", PasswordHash: "x", Status: status.ActivationEnabled, AuthVersion: 1},
	} {
		if err := db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}

	application := app.New(&config.Config{}, db, auditor, nil)
	attachStorage(t, application)
	const resourcePermission = "orders:read"
	if err := application.AttachmentPolicies.Register(attachment.FuncPolicy{Type: "example.order", Fn: func(ctx context.Context, _, _, resourceID, _ string) error {
		if !auth.MustFromContext(ctx).HasPermission(resourcePermission) || resourceID == "denied" {
			return platformresource.ErrForbidden
		}
		return nil
	}}); err != nil {
		t.Fatal(err)
	}
	if err := system.Register(application); err != nil {
		t.Fatal(err)
	}
	if err := application.PlatformResources.Validate(); err != nil {
		t.Fatal(err)
	}

	ctxA := platformResourceContext("tenant-a", "actor-a", resourcePermission, "orders:manage")
	ctxADenied := platformResourceContext("tenant-a", "actor-a")
	ctxB := platformResourceContext("tenant-b", "actor-b", resourcePermission, "orders:manage")
	payload := []byte("controlled-platform-resource")
	svc := system.NewAttachmentService(db, application.Storage, application.TransferTokens, application.AttachmentPolicies, nil)
	prepared, err := svc.UploadPrepare(ctxA, []byte(fmt.Sprintf(`{"original_name":"contract.txt","content_type":"text/plain","size":%d}`, len(payload))))
	if err != nil {
		t.Fatal(err)
	}
	prep := prepared.(system.TransferPrepareView)
	claims, err := svc.ParseTransferToken(prep.Token)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CompleteUpload(ctxA, claims, bytes.NewReader(payload)); err != nil {
		t.Fatal(err)
	}
	if err := db.WithContext(ctxA).Transaction(func(tx *gorm.DB) error {
		_, bindErr := svc.BindResource(ctxA, tx, attachment.ResourceLinkRequest{AttachmentID: prep.Attachment.ID, ResourceType: "example.order", ResourceID: "order-1", Field: "documents"})
		return bindErr
	}); err != nil {
		t.Fatal(err)
	}

	request := platformresource.AttachmentOpenRequest{AttachmentID: prep.Attachment.ID, Binding: platformresource.AttachmentBindingQuery{ResourceType: "example.order", ResourceID: "order-1", Field: "documents"}}
	content, err := application.PlatformResources.Attachments.Open(ctxA, request)
	if err != nil {
		t.Fatal(err)
	}
	if content.Metadata.Size != int64(len(payload)) || content.Metadata.ContentType != "text/plain" || content.Metadata.Variant != platformresource.AttachmentVariantOriginal {
		t.Fatalf("unexpected attachment metadata: %#v", content.Metadata)
	}
	_ = content.Reader.Close()
	if exists, err := application.PlatformResources.Attachments.BindingExists(ctxA, request); err != nil || !exists {
		t.Fatalf("binding exists=%v err=%v", exists, err)
	}
	if _, err := application.PlatformResources.Attachments.Open(ctxADenied, request); !errors.Is(err, platformresource.ErrForbidden) {
		t.Fatalf("policy denial err=%v", err)
	}
	if _, err := application.PlatformResources.Attachments.Open(ctxB, request); !errors.Is(err, platformresource.ErrUnavailable) {
		t.Fatalf("cross-tenant attachment err=%v", err)
	}
	thumb := request
	thumb.Variant = platformresource.AttachmentVariantThumb
	if _, err := application.PlatformResources.Attachments.Open(ctxA, thumb); !errors.Is(err, platformresource.ErrUnavailable) {
		t.Fatalf("missing variant err=%v", err)
	}
	tdb, err := tenant.TenantDB(ctxA, db)
	if err != nil {
		t.Fatal(err)
	}
	if err := tenant.TenantExec(tdb, `WITH scoped AS (SELECT id FROM attachments WHERE tenant_id = ? AND id = ?)
UPDATE attachments SET status = ? WHERE id IN (SELECT id FROM scoped)`, prep.Attachment.ID, status.AttachmentFailed); err != nil {
		t.Fatal(err)
	}
	if _, err := application.PlatformResources.Attachments.Open(ctxA, request); !errors.Is(err, platformresource.ErrUnavailable) {
		t.Fatalf("non-ready attachment err=%v", err)
	}
	type storedAttachment struct{ ObjectKey string }
	storedRows, err := tenant.TenantRaw[storedAttachment](tdb, "SELECT object_key FROM attachments WHERE tenant_id = ? AND id = ? LIMIT 1", prep.Attachment.ID)
	if err != nil || len(storedRows) != 1 {
		t.Fatal(err)
	}
	if err := tenant.TenantExec(tdb, `WITH scoped AS (SELECT id FROM attachments WHERE tenant_id = ? AND id = ?)
UPDATE attachments SET status = ? WHERE id IN (SELECT id FROM scoped)`, prep.Attachment.ID, status.AttachmentReady); err != nil {
		t.Fatal(err)
	}
	if err := application.Storage.Delete(ctxA, storedRows[0].ObjectKey); err != nil {
		t.Fatal(err)
	}
	if _, err := application.PlatformResources.Attachments.Open(ctxA, request); !errors.Is(err, platformresource.ErrUnavailable) {
		t.Fatalf("missing object err=%v", err)
	}

	target := auth.PrincipalRef{Kind: auth.PrincipalKindAccount, ID: "target-a"}
	projection, err := application.PlatformResources.Principals.Get(ctxA, target)
	if err != nil || projection.DisplayName != "Target A" || !projection.Active || projection.Revision != 1 {
		t.Fatalf("principal projection=%#v err=%v", projection, err)
	}
	if _, err := application.PlatformResources.Principals.Get(ctxB, target); !errors.Is(err, platformresource.ErrUnavailable) {
		t.Fatalf("cross-tenant principal err=%v", err)
	}
	if _, err := application.PlatformResources.Principals.Invalidate(ctxADenied, platformresource.PrincipalInvalidation{Principal: target, ExpectedRevision: 1, RequiredPermission: "orders:manage", Reason: "member disabled"}); !errors.Is(err, platformresource.ErrForbidden) {
		t.Fatalf("missing invalidation permission err=%v", err)
	}

	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, invalidateErr := application.PlatformResources.Principals.Invalidate(ctxA, platformresource.PrincipalInvalidation{Principal: target, ExpectedRevision: 1, RequiredPermission: "orders:manage", Reason: "member disabled"})
			results <- invalidateErr
		}()
	}
	wg.Wait()
	close(results)
	successes, conflicts := 0, 0
	for result := range results {
		switch {
		case result == nil:
			successes++
		case errors.Is(result, platformresource.ErrConflict):
			conflicts++
		default:
			t.Fatalf("unexpected concurrent invalidation error: %v", result)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent invalidation successes=%d conflicts=%d", successes, conflicts)
	}
	var auditCount int64
	if err := db.Model(&audit.Record{}).Where("tenant_id = ? AND action_id = ? AND resource_id = ?", "tenant-a", "system.platform_resource_principal_invalidate", "target-a").Count(&auditCount).Error; err != nil || auditCount != 1 {
		t.Fatalf("audit count=%d err=%v", auditCount, err)
	}
}

func platformResourceContext(tenantID, principalID string, permissions ...string) context.Context {
	return auth.WithContext(context.Background(), &auth.Context{Authenticated: true, Realm: auth.RealmTenant, TenantID: tenantID, PrincipalID: principalID, Principal: auth.PrincipalRef{Kind: auth.PrincipalKindAccount, ID: principalID}, ClientSurface: auth.TenantConsoleSurface, Audience: auth.TenantAudience, RequestID: uuid.NewString(), Client: "test", Permissions: permissions})
}
