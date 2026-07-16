package tests

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/logcenter"
)

func TestLogProviderRegistryRemainsAvailableWithoutProductSurface(t *testing.T) {
	registry := logcenter.NewRegistry()
	provider := &logcenter.FakeProvider{SourceName: "app", Caps: logcenter.Capabilities{TenantFilter: true}}
	if err := registry.Register(provider); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(provider); err == nil {
		t.Fatal("duplicate provider must fail")
	}
	sources := registry.List()
	if len(sources) != 1 || sources[0].Name != "app" || !sources[0].Capabilities.TenantFilter {
		t.Fatalf("unexpected sources: %#v", sources)
	}
}

func TestLogProviderTenantFilterAndSanitizerRemainAvailable(t *testing.T) {
	now := time.Now().UTC()
	provider := &logcenter.FakeProvider{SourceName: "app", Caps: logcenter.Capabilities{TenantFilter: true}, Entries: []logcenter.Entry{
		{ID: "l1", Timestamp: now, TenantID: "t1", Message: "Authorization: Bearer secret-token", Attributes: map[string]any{"password": "secret", "safe": strings.Repeat("x", 1200)}},
		{ID: "l2", Timestamp: now, TenantID: "t2", Message: "other tenant"},
	}}
	page, err := provider.Query(context.Background(), logcenter.Query{TenantID: "t1", StartAt: now.Add(-time.Minute), EndAt: now.Add(time.Minute), Page: 1, PageSize: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].TenantID != "t1" {
		t.Fatalf("provider tenant filter failed: %#v", page)
	}
	clean := logcenter.Sanitize(page.Items[0])
	if strings.Contains(clean.Message, "secret-token") || clean.Attributes["password"] != logcenter.Redacted {
		t.Fatalf("sensitive values leaked: %#v", clean)
	}
	if len(clean.Attributes["safe"].(string)) <= logcenter.MaxAttributeLength {
		t.Fatal("large attribute must be marked as truncated")
	}
}
