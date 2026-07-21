package system

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/setting"
)

type changeTestStore struct{ value *setting.StoredValue }

func (s *changeTestStore) Get(context.Context, string, string) (*setting.StoredValue, error) {
	return s.value, nil
}
func (s *changeTestStore) Put(_ context.Context, _ string, _ string, _ int64, schema int, values map[string]any, _ string) (*setting.StoredValue, error) {
	s.value = &setting.StoredValue{SchemaVersion: schema, Revision: 1, Values: values}
	return s.value, nil
}
func (s *changeTestStore) Delete(context.Context, string, string, int64) error {
	s.value = nil
	return nil
}

func TestSettingUpdatePublishesPostCommitChange(t *testing.T) {
	registry := setting.NewRegistry()
	if err := registry.RegisterCategory(setting.Category{Key: "ai", Title: "AI", Icon: "mdi:brain", Surface: auth.RealmTenant}); err != nil {
		t.Fatal(err)
	}
	if err := registry.RegisterDefinition(setting.Definition{Key: "ai.navigation_search", CategoryKey: "ai", Title: "Navigation", Surface: auth.RealmTenant, Scope: setting.ScopeTenant, SchemaVersion: 1, Fields: []setting.Field{{Key: "embedding_model_alias", Type: setting.FieldText, Label: "Alias", Default: ""}}}); err != nil {
		t.Fatal(err)
	}
	store := &changeTestStore{}
	changes := setting.NewChangeRegistry()
	var received setting.Change
	if err := changes.Register("ai.navigation_search", func(_ context.Context, change setting.Change) error { received = change; return nil }); err != nil {
		t.Fatal(err)
	}
	service := &SettingService{registry: registry, store: store, provider: setting.NewProvider(registry, store), changes: changes}
	ctx := auth.WithContext(context.Background(), &auth.Context{Authenticated: true, TenantID: "tenant-1", UserID: "user-1", Realm: auth.RealmTenant})
	if _, err := service.Update(ctx, json.RawMessage(`{"key":"ai.navigation_search","revision":0,"values":{"embedding_model_alias":"embed"}}`)); err != nil {
		t.Fatal(err)
	}
	if received.TenantID != "tenant-1" || received.UserID != "user-1" || received.Operation != setting.ChangeUpdated {
		t.Fatalf("unexpected change: %#v", received)
	}
}
