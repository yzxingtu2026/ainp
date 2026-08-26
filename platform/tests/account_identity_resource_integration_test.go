package tests

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/audit"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	"github.com/yzxingtu2026/ainp/platform/framework/platformresource"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"github.com/yzxingtu2026/ainp/platform/modules/system"
	systemmodel "github.com/yzxingtu2026/ainp/platform/modules/system/model"
)

func TestAccountIdentityResourceBindingLifecycleAndIsolation(t *testing.T) {
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	admin, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "ainp_account_identity_" + uuid.NewString()[:8]
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
	for _, row := range []tenant.Tenant{
		{ID: "tenant-a", Name: "Tenant A", Status: status.ActivationEnabled},
		{ID: "tenant-b", Name: "Tenant B", Status: status.ActivationEnabled},
	} {
		if err := db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, row := range []systemmodel.TenantAccount{
		{ID: "account-a", TenantID: "tenant-a", Username: "account-a", PasswordHash: "x", Status: status.ActivationEnabled},
		{ID: "account-a2", TenantID: "tenant-a", Username: "account-a2", PasswordHash: "x", Status: status.ActivationEnabled},
		{ID: "account-b", TenantID: "tenant-b", Username: "account-b", PasswordHash: "x", Status: status.ActivationEnabled},
	} {
		if err := db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}

	service := system.NewAccountIdentityResourceService(db, auditor)
	ctxA := platformResourceContext("tenant-a", "account-a")
	ctxA2 := platformResourceContext("tenant-a", "account-a2")
	ctxB := platformResourceContext("tenant-b", "account-b")
	validRequest := platformresource.AccountIdentityBindRequest{Provider: "wecom-oauth", Subject: "corp-a:user-a", NormalizedIdentifier: "user-a@example.com"}
	if _, err := system.NewAccountIdentityResourceService(nil, auditor).Bind(ctxA, validRequest); !errors.Is(err, platformresource.ErrUnavailable) {
		t.Fatalf("missing database bind error = %v", err)
	}
	if _, err := system.NewAccountIdentityResourceService(db, nil).Bind(ctxA, validRequest); !errors.Is(err, platformresource.ErrUnavailable) {
		t.Fatalf("missing transactional audit bind error = %v", err)
	}

	bound, err := service.Bind(ctxA, validRequest)
	if err != nil {
		t.Fatal(err)
	}
	if bound.ID == "" || bound.Provider != "wecom-oauth" || bound.Subject != "corp-a:user-a" || bound.NormalizedIdentifier != "user-a@example.com" || bound.VerifiedAt.IsZero() {
		t.Fatalf("unexpected binding projection: %#v", bound)
	}
	listed, err := service.List(ctxA)
	if err != nil || len(listed) != 1 || listed[0] != *bound {
		t.Fatalf("list=%#v err=%v", listed, err)
	}
	if _, err := service.Bind(ctxA, platformresource.AccountIdentityBindRequest{Provider: "wecom-oauth", Subject: "corp-a:user-other", NormalizedIdentifier: "other@example.com"}); !errors.Is(err, platformresource.ErrConflict) {
		t.Fatalf("same-provider binding error = %v", err)
	}

	other, err := service.Bind(ctxA2, platformresource.AccountIdentityBindRequest{Provider: "dingtalk-oauth", Subject: "corp-a:shared", NormalizedIdentifier: "shared@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Bind(ctxA, platformresource.AccountIdentityBindRequest{Provider: "dingtalk-oauth", Subject: "corp-a:shared", NormalizedIdentifier: "shared@example.com"}); !errors.Is(err, platformresource.ErrConflict) {
		t.Fatalf("identity ownership conflict error = %v", err)
	}
	if err := service.Unbind(ctxA, platformresource.AccountIdentityUnbindRequest{IdentityID: other.ID}); !errors.Is(err, platformresource.ErrUnavailable) {
		t.Fatalf("other-account unbind error = %v", err)
	}
	if err := service.Unbind(ctxB, platformresource.AccountIdentityUnbindRequest{IdentityID: bound.ID}); !errors.Is(err, platformresource.ErrUnavailable) {
		t.Fatalf("cross-tenant unbind error = %v", err)
	}
	if _, err := service.Bind(ctxB, platformresource.AccountIdentityBindRequest{Provider: "wecom-oauth", Subject: "corp-a:user-a", NormalizedIdentifier: "user-a@example.com"}); err != nil {
		t.Fatalf("cross-tenant identity should remain isolated: %v", err)
	}

	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, subject := range []string{"github:user-a", "github:user-b"} {
		wg.Add(1)
		go func(subject string) {
			defer wg.Done()
			_, bindErr := service.Bind(ctxA, platformresource.AccountIdentityBindRequest{Provider: "github-oauth", Subject: subject, NormalizedIdentifier: subject + "@example.com"})
			results <- bindErr
		}(subject)
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
			t.Fatalf("unexpected concurrent binding error: %v", result)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent binding successes=%d conflicts=%d", successes, conflicts)
	}

	wrongKind := auth.WithContext(context.Background(), &auth.Context{
		Authenticated: true, Realm: auth.RealmTenant, TenantID: "tenant-a", PrincipalID: "account-a",
		Principal: auth.PrincipalRef{Kind: "example.customer", ID: "account-a"},
	})
	if _, err := service.List(wrongKind); !errors.Is(err, platformresource.ErrForbidden) {
		t.Fatalf("non-account list error = %v", err)
	}
	if _, err := service.List(context.Background()); !errors.Is(err, platformresource.ErrForbidden) {
		t.Fatalf("missing context list error = %v", err)
	}
	if err := db.Model(&systemmodel.TenantAccount{}).Where("tenant_id = ? AND id = ?", "tenant-a", "account-a").Update("status", status.ActivationDisabled).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.List(ctxA); !errors.Is(err, platformresource.ErrForbidden) {
		t.Fatalf("disabled account list error = %v", err)
	}
	if _, err := service.Bind(ctxA, platformresource.AccountIdentityBindRequest{Provider: "disabled-test", Subject: "disabled", NormalizedIdentifier: "disabled"}); !errors.Is(err, platformresource.ErrForbidden) {
		t.Fatalf("disabled account bind error = %v", err)
	}
	if err := service.Unbind(ctxA, platformresource.AccountIdentityUnbindRequest{IdentityID: bound.ID}); !errors.Is(err, platformresource.ErrForbidden) {
		t.Fatalf("disabled account unbind error = %v", err)
	}
	if err := db.Model(&systemmodel.TenantAccount{}).Where("tenant_id = ? AND id = ?", "tenant-a", "account-a").Update("status", status.ActivationEnabled).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.Unbind(ctxA, platformresource.AccountIdentityUnbindRequest{IdentityID: bound.ID}); err != nil {
		t.Fatal(err)
	}
	listed, err = service.List(ctxA)
	if err != nil {
		t.Fatal(err)
	}
	for _, identity := range listed {
		if identity.ID == bound.ID {
			t.Fatal("unbound identity remains visible")
		}
	}

	var records []audit.Record
	if err := db.Where("tenant_id = ? AND resource_type = ?", "tenant-a", "system.account_identity").Find(&records).Error; err != nil {
		t.Fatal(err)
	}
	if len(records) < 4 {
		t.Fatalf("account identity audit records = %d, want at least 4", len(records))
	}
	for _, record := range records {
		serialized := strings.Join([]string{record.ActionID, record.Summary}, " ")
		for _, secret := range []string{"corp-a:user-a", "user-a@example.com", "wecom-oauth", "oauth-code", "oauth-token"} {
			if strings.Contains(serialized, secret) {
				t.Fatalf("audit record leaks identity or OAuth material: %#v", record)
			}
		}
	}
}

func TestAccountIdentityBindingMigrationAddsProviderUniqueness(t *testing.T) {
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	admin, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "ainp_account_identity_migration_" + uuid.NewString()[:8]
	if err := admin.Exec(fmt.Sprintf(`CREATE SCHEMA %q`, schema)).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Exec(fmt.Sprintf(`DROP SCHEMA %q CASCADE`, schema)).Error })
	db, err := database.Open(dsn + " search_path=" + schema)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`CREATE TABLE auth_identities (
id varchar(64) PRIMARY KEY,
tenant_id varchar(64) NOT NULL,
account_id varchar(64) NOT NULL,
provider varchar(64) NOT NULL,
subject varchar(255) NOT NULL
)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO auth_identities (id, tenant_id, account_id, provider, subject)
VALUES ('identity-1', 'tenant-a', 'account-a', 'wecom-oauth', 'subject-1')`).Error; err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile("../migrations/0066_account_identity_binding.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(string(body)).Error; err != nil {
		t.Fatal(err)
	}
	duplicate := db.Exec(`INSERT INTO auth_identities (id, tenant_id, account_id, provider, subject)
VALUES ('identity-2', 'tenant-a', 'account-a', 'wecom-oauth', 'subject-2') ON CONFLICT DO NOTHING`)
	if duplicate.Error != nil || duplicate.RowsAffected != 0 {
		t.Fatalf("same tenant/account/provider was not rejected after migration: rows=%d err=%v", duplicate.RowsAffected, duplicate.Error)
	}
	for _, statement := range []string{
		`INSERT INTO auth_identities (id, tenant_id, account_id, provider, subject) VALUES ('identity-3', 'tenant-a', 'account-b', 'wecom-oauth', 'subject-3')`,
		`INSERT INTO auth_identities (id, tenant_id, account_id, provider, subject) VALUES ('identity-4', 'tenant-a', 'account-a', 'dingtalk-oauth', 'subject-4')`,
		`INSERT INTO auth_identities (id, tenant_id, account_id, provider, subject) VALUES ('identity-5', 'tenant-b', 'account-a', 'wecom-oauth', 'subject-5')`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatalf("valid isolated binding rejected: %v", err)
		}
	}
}
