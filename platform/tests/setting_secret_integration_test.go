package tests

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/config"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/setting"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"github.com/yzxingtu2026/ainp/platform/modules/system"
	"github.com/yzxingtu2026/ainp/platform/modules/system/model"
	"gorm.io/gorm"
)

func TestSettingSecretEncryptedPersistenceAndSafeViews(t *testing.T) {
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	db, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "ainp_setting_secret_" + uuid.NewString()[:8]
	if err := db.Exec(fmt.Sprintf(`CREATE SCHEMA %q`, schema)).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Exec(fmt.Sprintf(`DROP SCHEMA %q CASCADE`, schema)).Error })
	testDB, err := database.Open(dsn + " search_path=" + schema)
	if err != nil {
		t.Fatal(err)
	}
	if err := system.AutoMigrate(testDB); err != nil {
		t.Fatal(err)
	}
	for _, row := range []tenant.Tenant{{ID: "tenant-a", Name: "A", Status: status.ActivationEnabled}, {ID: "tenant-b", Name: "B", Status: status.ActivationEnabled}} {
		if err := testDB.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, row := range []model.TenantAccount{{ID: "user-a", TenantID: "tenant-a", Username: "admin-a", PasswordHash: "x", Status: status.ActivationEnabled}, {ID: "user-b", TenantID: "tenant-b", Username: "admin-b", PasswordHash: "x", Status: status.ActivationEnabled}} {
		if err := testDB.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	application := app.New(&config.Config{CredentialEncryptionKey: key}, testDB, nil, nil)
	attachStorage(t, application)
	if err := system.Register(application); err != nil {
		t.Fatal(err)
	}
	minLength := 8
	definition := setting.Definition{Key: "general.integration_secret", CategoryKey: "general", Title: "Integration Secret", Scope: setting.ScopeTenant, Surface: auth.RealmTenant, Permission: system.ActionSettingList, SchemaVersion: 1, Fields: []setting.Field{
		{Key: "endpoint", Type: setting.FieldText, Label: "Endpoint", Required: true, Default: "https://example.test"},
		{Key: "api_key", Type: setting.FieldSecret, Label: "API key", Required: true, MinLength: &minLength},
		{Key: "webhook_secret", Type: setting.FieldSecret, Label: "Webhook secret", MinLength: &minLength},
	}}
	if err := application.RegisterSettingDefinition(definition); err != nil {
		t.Fatal(err)
	}
	svc := system.NewSettingService(testDB, application.Settings, application.CredentialCipher)
	ctxA := auth.WithContext(context.Background(), &auth.Context{TenantID: "tenant-a", PrincipalID: "user-a", Realm: auth.RealmTenant, Authenticated: true, Permissions: []string{system.ActionSettingList, system.ActionSettingUpdate}})
	ctxB := auth.WithContext(context.Background(), &auth.Context{TenantID: "tenant-b", PrincipalID: "user-b", Realm: auth.RealmTenant, Authenticated: true, Permissions: []string{system.ActionSettingList, system.ActionSettingUpdate}})
	platformCtx := auth.WithContext(context.Background(), &auth.Context{Authenticated: true, Realm: auth.RealmPlatform, IsSystemOperator: true})
	systemDB, err := tenant.SystemDB(platformCtx, testDB)
	if err != nil {
		t.Fatal(err)
	}
	inspectionDB := systemDB.WithContext(platformCtx)

	if _, err := svc.Update(ctxA, json.RawMessage(`{"key":"general.integration_secret","revision":0,"values":{"endpoint":"https://plain.test","api_key":"must-not-pass"}}`)); appErrorCode(err) != errs.CodeValidation {
		t.Fatalf("ordinary update must reject secret definition: %v", err)
	}
	firstView, err := svc.UpdateSecret(ctxA, json.RawMessage(`{"key":"general.integration_secret","revision":0,"values":{"endpoint":"https://one.test"},"secrets":{"api_key":"alpha-secret","webhook_secret":"bravo-secret"}}`))
	if err != nil {
		t.Fatal(err)
	}
	assertSafeSecretView(t, firstView.(system.SettingDefinitionView), "alpha-secret", "bravo-secret")
	if strings.Contains(auth.MustFromContext(ctxA).AuditSummary, "alpha-secret") || strings.Contains(auth.MustFromContext(ctxA).AuditSummary, "bravo-secret") {
		t.Fatal("secret leaked to audit summary")
	}
	getView, err := svc.Get(ctxA, json.RawMessage(`{"key":"general.integration_secret"}`))
	if err != nil {
		t.Fatal(err)
	}
	assertSafeSecretView(t, getView.(system.SettingDefinitionView), "alpha-secret", "bravo-secret")
	listed, err := svc.List(ctxA, json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	listedJSON, _ := json.Marshal(listed)
	if strings.Contains(string(listedJSON), "alpha-secret") || strings.Contains(string(listedJSON), "bravo-secret") || strings.Contains(string(listedJSON), "v1:") {
		t.Fatalf("secret leaked from list response: %s", listedJSON)
	}
	firstPayload := loadSettingPayload(t, inspectionDB, "tenant-a", definition.Key)
	if strings.Contains(string(firstPayload), "alpha-secret") || strings.Contains(string(firstPayload), "bravo-secret") {
		t.Fatalf("plaintext leaked to database: %s", firstPayload)
	}

	providerValue, err := svc.Provider().Get(ctxA, "tenant-a", definition.Key)
	if err != nil || providerValue.Values["api_key"] != "alpha-secret" || providerValue.Values["webhook_secret"] != "bravo-secret" {
		t.Fatalf("trusted provider did not decrypt values: %#v err=%v", providerValue, err)
	}
	secondView, err := svc.UpdateSecret(ctxA, json.RawMessage(`{"key":"general.integration_secret","revision":1,"values":{"endpoint":"https://two.test"},"secrets":{"api_key":"alpha-secret"}}`))
	if err != nil {
		t.Fatal(err)
	}
	assertSafeSecretView(t, secondView.(system.SettingDefinitionView), "alpha-secret", "bravo-secret")
	secondPayload := loadSettingPayload(t, inspectionDB, "tenant-a", definition.Key)
	if string(firstPayload) == string(secondPayload) {
		t.Fatal("re-saving the same secret must produce fresh ciphertext")
	}

	if _, err := svc.UpdateSecret(ctxA, json.RawMessage(`{"key":"general.integration_secret","revision":2,"values":{"endpoint":""},"secrets":{"api_key":"changed-secret"}}`)); appErrorCode(err) != errs.CodeValidation {
		t.Fatalf("invalid ordinary field must roll back secret update: %v", err)
	}
	if got := loadSettingPayload(t, inspectionDB, "tenant-a", definition.Key); string(got) != string(secondPayload) {
		t.Fatal("failed update changed persisted ciphertext")
	}
	clearedView, err := svc.UpdateSecret(ctxA, json.RawMessage(`{"key":"general.integration_secret","revision":2,"values":{"endpoint":"https://two.test"},"clear_secret_fields":["webhook_secret"]}`))
	if err != nil {
		t.Fatal(err)
	}
	view := clearedView.(system.SettingDefinitionView)
	if secretConfigured(view, "webhook_secret") || !secretConfigured(view, "api_key") {
		t.Fatalf("unexpected configured state after clear: %#v", view.Fields)
	}
	if _, err := svc.UpdateSecret(ctxA, json.RawMessage(`{"key":"general.integration_secret","revision":3,"values":{"endpoint":"https://two.test"},"clear_secret_fields":["api_key"]}`)); appErrorCode(err) != errs.CodeValidation {
		t.Fatalf("required secret clear must fail: %v", err)
	}

	copyPayload := loadSettingPayload(t, inspectionDB, "tenant-a", definition.Key)
	if err := inspectionDB.Create(&model.TenantSettingValue{ID: uuid.NewString(), TenantID: "tenant-b", SettingKey: definition.Key, SchemaVersion: 1, ValueJSON: copyPayload, Revision: 1, UpdatedBy: "user-b"}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Provider().Get(ctxB, "tenant-b", definition.Key); !errors.Is(err, setting.ErrSecretUnavailable) {
		t.Fatalf("cross-tenant ciphertext replacement must fail closed: %v", err)
	}
	repaired, err := svc.UpdateSecret(ctxB, json.RawMessage(`{"key":"general.integration_secret","revision":1,"values":{"endpoint":"https://repaired.test"},"secrets":{"api_key":"repaired-secret"},"clear_secret_fields":["webhook_secret"]}`))
	if err != nil || !secretConfigured(repaired.(system.SettingDefinitionView), "api_key") {
		t.Fatalf("explicit reconfiguration must repair unavailable ciphertext: %#v err=%v", repaired, err)
	}
	reset, err := svc.Reset(ctxB, json.RawMessage(`{"key":"general.integration_secret","revision":2}`))
	if err != nil {
		t.Fatal(err)
	}
	if secretConfigured(reset.(system.SettingDefinitionView), "api_key") {
		t.Fatal("reset must clear persisted secrets")
	}
}

func loadSettingPayload(t *testing.T, db *gorm.DB, tenantID, key string) json.RawMessage {
	t.Helper()
	var row model.TenantSettingValue
	if err := db.Where("tenant_id=? AND setting_key=?", tenantID, key).First(&row).Error; err != nil {
		t.Fatal(err)
	}
	return append(json.RawMessage(nil), row.ValueJSON...)
}

func assertSafeSecretView(t *testing.T, view system.SettingDefinitionView, forbidden ...string) {
	t.Helper()
	encoded, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range forbidden {
		if strings.Contains(string(encoded), value) {
			t.Fatalf("secret leaked in response: %s", encoded)
		}
	}
	if _, ok := view.Values["api_key"]; ok || !secretConfigured(view, "api_key") {
		t.Fatalf("secret value/status contract violated: %#v", view)
	}
}

func secretConfigured(view system.SettingDefinitionView, key string) bool {
	for _, field := range view.Fields {
		if field.Key == key && field.SecretConfigured != nil {
			return *field.SecretConfigured
		}
	}
	return false
}
