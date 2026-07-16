package tests

import (
	"context"
	"os"
	"testing"

	"github.com/yzxingtu2026/ainp/platform/framework/aimodel"
)

func TestProviderTypeRegistryRejectsInvalidAndDuplicateValues(t *testing.T) {
	r := aimodel.NewProviderTypeRegistry()
	if err := r.Register(aimodel.ProviderType{}); err == nil {
		t.Fatal("empty provider type must fail")
	}
	v := aimodel.ProviderType{Protocol: "test_protocol", Name: "Test", Capabilities: aimodel.Capabilities{Features: []aimodel.Feature{aimodel.FeatureStreaming}}}
	if err := r.Register(v); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(v); err == nil {
		t.Fatal("duplicate protocol must fail")
	}
	v.Protocol = "unknown_feature"
	v.Capabilities.Features = []aimodel.Feature{"not_real"}
	if err := r.Register(v); err == nil {
		t.Fatal("unknown feature must fail")
	}
}

func TestEnvSecretResolverFailClosed(t *testing.T) {
	r := aimodel.EnvSecretResolver{}
	if _, err := r.Resolve(context.Background(), "plain-key"); err == nil {
		t.Fatal("plain secret must fail")
	}
	t.Setenv("AINP_TEST_AI_SECRET", "secret-value")
	value, err := r.Resolve(context.Background(), "env:AINP_TEST_AI_SECRET")
	if err != nil || value != "secret-value" {
		t.Fatalf("unexpected resolution: %q %v", value, err)
	}
	os.Unsetenv("AINP_TEST_AI_MISSING")
	if _, err := r.Resolve(context.Background(), "env:AINP_TEST_AI_MISSING"); err == nil {
		t.Fatal("missing env must fail")
	}
}

func TestCapabilityIntersectionCannotEscalateAdapter(t *testing.T) {
	adapter := aimodel.Capabilities{InputModalities: []aimodel.Modality{aimodel.ModalityText}, Features: []aimodel.Feature{aimodel.FeatureStreaming}}
	declared := aimodel.Capabilities{InputModalities: []aimodel.Modality{aimodel.ModalityText, aimodel.ModalityImage}, Features: []aimodel.Feature{aimodel.FeatureStreaming, aimodel.FeatureVision}}
	effective := aimodel.Intersect(adapter, declared)
	if aimodel.Satisfies(effective, aimodel.CapabilityRequirements{ModelType: aimodel.ModelTypeChat, Features: []aimodel.Feature{aimodel.FeatureVision}}) {
		t.Fatal("deployment must not escalate adapter capability")
	}
	if !aimodel.Satisfies(effective, aimodel.CapabilityRequirements{ModelType: aimodel.ModelTypeChat, Features: []aimodel.Feature{aimodel.FeatureStreaming}}) {
		t.Fatal("shared capability should remain")
	}
}

func TestSafeEndpointRejectsLocalTargets(t *testing.T) {
	for _, raw := range []string{"http://api.openai.com", "https://localhost:8443", "https://127.0.0.1", "https://user:pass@example.com"} {
		if _, err := aimodel.SafeEndpoint(raw); err == nil {
			t.Fatalf("unsafe endpoint accepted: %s", raw)
		}
	}
}
