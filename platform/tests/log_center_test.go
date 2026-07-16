package tests

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/audit"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/config"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/logcenter"
	"github.com/yzxingtu2026/ainp/platform/modules/operations"
)

func TestLogProviderRegistryRejectsDuplicates(t *testing.T) {
	registry := logcenter.NewRegistry()
	provider := &logcenter.FakeProvider{SourceName: "app", Caps: logcenter.Capabilities{TenantFilter: true}}
	if err := registry.Register(provider); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(provider); err == nil {
		t.Fatal("duplicate provider must fail")
	}
	if sources := registry.List(); len(sources) != 1 || sources[0].Name != "app" {
		t.Fatalf("unexpected sources: %#v", sources)
	}
}

func TestLogCenterTenantScopeFiltersAndSanitizes(t *testing.T) {
	db := openOutboxTestDB(t)
	if err := db.AutoMigrate(&audit.Record{}); err != nil {
		t.Fatal(err)
	}
	application := app.New(&config.Config{}, db, audit.NewGormRecorder(db), nil)
	now := time.Now().UTC()
	provider := &logcenter.FakeProvider{SourceName: "app", Label: "Application", Caps: logcenter.Capabilities{TenantFilter: true, Levels: []string{"info", "error"}}, Entries: []logcenter.Entry{
		{ID: "l1", Timestamp: now.Add(-time.Minute), Level: "error", Service: "api", Component: "action", TenantID: "t1", Message: "failed Authorization: Bearer secret-token", RequestID: "req-1", Attributes: map[string]any{"password": "secret", "safe": strings.Repeat("x", 1200), "nested": []any{map[string]any{"api_key": "nested-secret"}}}},
		{ID: "l2", Timestamp: now.Add(-time.Minute), Level: "error", Service: "api", TenantID: "t2", Message: "other tenant"},
	}}
	if err := application.LogProviders.Register(provider); err != nil {
		t.Fatal(err)
	}
	if err := application.LogProviders.Register(&logcenter.FakeProvider{SourceName: "platform", Caps: logcenter.Capabilities{TenantFilter: false}}); err != nil {
		t.Fatal(err)
	}
	if err := operations.Register(application, nil); err != nil {
		t.Fatal(err)
	}
	ctx := auth.WithContext(context.Background(), &auth.Context{Authenticated: true, TenantID: "t1", Realm: auth.RealmTenant, UserID: "viewer", Permissions: []string{operations.ActionLogSourceList, operations.ActionLogQuery, operations.ActionLogGet}, RequestID: "query-log"})
	denied := auth.WithContext(context.Background(), &auth.Context{Authenticated: true, TenantID: "t1", Realm: auth.RealmTenant})
	if _, err := application.Actions.Invoke(denied, operations.ActionLogQuery, json.RawMessage(`{"source":"app"}`)); errorCode(err) != errs.CodeForbidden {
		t.Fatalf("log query without permission must be forbidden: %v", err)
	}

	sourceResult, err := application.Actions.Invoke(ctx, operations.ActionLogSourceList, nil)
	if err != nil {
		t.Fatal(err)
	}
	sources := sourceResult.(map[string]any)["sources"].([]logcenter.Source)
	if len(sources) != 1 || sources[0].Name != "app" {
		t.Fatalf("tenant sources must hide unsafe provider: %#v", sources)
	}

	result, err := application.Actions.Invoke(ctx, operations.ActionLogQuery, json.RawMessage(`{"source":"app","level":"error","service":"api","keyword":"failed","page":1,"page_size":20}`))
	if err != nil {
		t.Fatal(err)
	}
	page := result.(logcenter.Page)
	if page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != "l1" {
		t.Fatalf("unexpected log page: %#v", page)
	}
	if strings.Contains(page.Items[0].Message, "secret-token") || page.Items[0].Attributes["password"] != logcenter.Redacted {
		t.Fatalf("sensitive values leaked: %#v", page.Items[0])
	}
	nested := page.Items[0].Attributes["nested"].([]any)[0].(map[string]any)
	if nested["api_key"] != logcenter.Redacted {
		t.Fatalf("nested secret leaked: %#v", nested)
	}
	if len(page.Items[0].Attributes["safe"].(string)) <= logcenter.MaxAttributeLength {
		t.Fatal("large attribute must be marked as truncated")
	}

	detail, err := application.Actions.Invoke(ctx, operations.ActionLogGet, json.RawMessage(`{"source":"app","id":"l1"}`))
	if err != nil {
		t.Fatal(err)
	}
	if detail.(map[string]any)["log_entry"].(logcenter.Entry).TenantID != "t1" {
		t.Fatal("detail tenant mismatch")
	}
	if _, err := application.Actions.Invoke(ctx, operations.ActionLogGet, json.RawMessage(`{"source":"app","id":"l2"}`)); errorCode(err) != errs.CodeNotFound {
		t.Fatalf("cross-tenant detail must be opaque 404: %v", err)
	}
	if _, err := application.Actions.Invoke(ctx, operations.ActionLogQuery, json.RawMessage(`{"source":"platform"}`)); errorCode(err) != errs.CodeForbidden {
		t.Fatalf("unsafe provider must fail closed: %v", err)
	}

	var audited int64
	if err := db.Model(&audit.Record{}).Where("tenant_id = ? AND action_id IN ?", "t1", []string{operations.ActionLogSourceList, operations.ActionLogQuery, operations.ActionLogGet}).Count(&audited).Error; err != nil {
		t.Fatal(err)
	}
	if audited < 3 {
		t.Fatalf("log access must be audited, count=%d", audited)
	}
}

func TestLogCenterRejectsInvalidLimitsAndMapsProviderErrors(t *testing.T) {
	application := app.New(&config.Config{}, openOutboxTestDB(t), nil, nil)
	if err := application.LogProviders.Register(&logcenter.FakeProvider{SourceName: "down", Caps: logcenter.Capabilities{TenantFilter: true}, Err: logcenter.ErrTimeout}); err != nil {
		t.Fatal(err)
	}
	largeEntries := make([]logcenter.Entry, 100)
	for i := range largeEntries {
		attributes := map[string]any{}
		for n := 0; n < 20; n++ {
			attributes[string(rune('a'+n))] = strings.Repeat("x", logcenter.MaxAttributeLength)
		}
		largeEntries[i] = logcenter.Entry{ID: string(rune('a' + i)), Timestamp: time.Now().UTC(), TenantID: "t1", Level: "info", Message: "large", Attributes: attributes}
	}
	if err := application.LogProviders.Register(&logcenter.FakeProvider{SourceName: "large", Caps: logcenter.Capabilities{TenantFilter: true}, Entries: largeEntries}); err != nil {
		t.Fatal(err)
	}
	if err := operations.Register(application, nil); err != nil {
		t.Fatal(err)
	}
	ctx := auth.WithContext(context.Background(), &auth.Context{Authenticated: true, TenantID: "t1", Realm: auth.RealmTenant, Permissions: []string{operations.ActionLogQuery}})
	for _, input := range []string{
		`{"source":"down","page_size":101}`,
		`{"source":"down","start_at":"invalid"}`,
		`{"source":"down","start_at":"2026-01-01T00:00:00Z","end_at":"2026-01-03T00:00:00Z"}`,
	} {
		if _, err := application.Actions.Invoke(ctx, operations.ActionLogQuery, json.RawMessage(input)); errorCode(err) != errs.CodeValidation {
			t.Fatalf("input %s must be rejected: %v", input, err)
		}
	}
	if _, err := application.Actions.Invoke(ctx, operations.ActionLogQuery, json.RawMessage(`{"source":"down"}`)); errorCode(err) != errs.CodeInternal {
		t.Fatalf("timeout must use provider error semantics: %v", err)
	}
	if _, err := application.Actions.Invoke(ctx, operations.ActionLogQuery, json.RawMessage(`{"source":"large","page_size":100}`)); errorCode(err) != errs.CodeValidation {
		t.Fatalf("oversized result must be rejected: %v", err)
	}
}
