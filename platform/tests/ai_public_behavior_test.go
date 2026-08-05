package tests

import (
	"context"
	"testing"

	"github.com/yzxingtu2026/ainp/platform/framework/aimodel"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/setting"
	"github.com/yzxingtu2026/ainp/platform/modules/ai"
)

type emptySettingStore struct{}

func (emptySettingStore) Get(context.Context, string, string) (*setting.StoredValue, error) {
	return nil, nil
}

func (emptySettingStore) Put(context.Context, string, string, int64, int, map[string]any, string) (*setting.StoredValue, error) {
	panic("unexpected setting write")
}

func (emptySettingStore) Delete(context.Context, string, string, int64) error {
	panic("unexpected setting delete")
}

func TestDeploymentStructuredOutputCapabilitiesFailClosedThroughService(t *testing.T) {
	service := ai.NewService(nil, aimodel.NewProviderTypeRegistry(), nil)
	ctx := auth.WithContext(context.Background(), &auth.Context{Authenticated: true, TenantID: "tenant-a", Realm: auth.RealmTenant})
	base := ai.DeploymentCreateRequest{Code: "chat", ModelType: aimodel.ModelTypeChat}

	featureOnly := base
	featureOnly.Capabilities.Features = []aimodel.Feature{aimodel.FeatureStructuredOutput}
	if _, err := service.CreateDeployment(ctx, featureOnly); err == nil {
		t.Fatal("structured_output without strategies must fail")
	}

	strategyOnly := base
	strategyOnly.Capabilities.StructuredOutputStrategies = []aimodel.StructuredOutputStrategy{aimodel.StructuredOutputResponseOnlyFunction}
	if _, err := service.CreateDeployment(ctx, strategyOnly); err == nil {
		t.Fatal("structured output strategies without feature must fail")
	}

	duplicate := base
	duplicate.Capabilities.Features = []aimodel.Feature{aimodel.FeatureStructuredOutput}
	duplicate.Capabilities.StructuredOutputStrategies = []aimodel.StructuredOutputStrategy{aimodel.StructuredOutputJSONMode, aimodel.StructuredOutputJSONMode}
	if _, err := service.CreateDeployment(ctx, duplicate); err == nil {
		t.Fatal("duplicate structured output strategies must fail")
	}
}

func TestPersonalizationDisabledFallbackUsesPublicServiceContract(t *testing.T) {
	registry := setting.NewRegistry()
	if err := registry.RegisterCategory(setting.Category{Key: "ai", Title: "AI", Icon: "mdi:brain", Surface: auth.RealmTenant}); err != nil {
		t.Fatal(err)
	}
	if err := registry.RegisterDefinition(setting.Definition{
		Key: ai.AssistantPersonalizationSettingKey, CategoryKey: "ai", Title: "Personalization",
		Scope: setting.ScopeTenant, Surface: auth.RealmTenant, SchemaVersion: 1,
		Fields: []setting.Field{{Key: "enabled", Type: setting.FieldBoolean, Label: "Enabled", Default: false}},
	}); err != nil {
		t.Fatal(err)
	}
	service := ai.NewPersonalizationService(nil, setting.NewProvider(registry, emptySettingStore{}), nil, nil)
	ctx := auth.WithContext(context.Background(), &auth.Context{Authenticated: true, TenantID: "tenant-a", PrincipalID: "user-a", Realm: auth.RealmTenant, Language: "en-US"})

	result, err := service.Read(ctx, struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Freshness != "disabled" || result.Greeting == "" || len(result.Suggestions) != 2 {
		t.Fatalf("unexpected disabled fallback: %#v", result)
	}
	for _, suggestion := range result.Suggestions {
		if suggestion.TargetRef != nil || suggestion.Title == "" || suggestion.Prompt == "" {
			t.Fatalf("fallback suggestion must be safe and actionable: %#v", suggestion)
		}
	}
}
