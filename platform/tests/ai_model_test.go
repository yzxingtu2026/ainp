package tests

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/yzxingtu2026/ainp/platform/framework/aimodel"
	"github.com/yzxingtu2026/ainp/platform/modules/ai"
)

func TestChatDeploymentLimitsUseAuthoritativeContextBudget(t *testing.T) {
	tests := []struct {
		name      string
		limits    aimodel.Limits
		wantField string
	}{
		{name: "missing context", limits: aimodel.Limits{}, wantField: "limits.context_window"},
		{name: "negative context", limits: aimodel.Limits{ContextWindow: -1, MaxOutputTokens: 1}, wantField: "limits.context_window"},
		{name: "missing output", limits: aimodel.Limits{ContextWindow: 128000}, wantField: "limits.max_output_tokens"},
		{name: "negative output", limits: aimodel.Limits{ContextWindow: 128000, MaxOutputTokens: -1}, wantField: "limits.max_output_tokens"},
		{name: "equal", limits: aimodel.Limits{ContextWindow: 128000, MaxOutputTokens: 128000}, wantField: "limits.max_output_tokens"},
		{name: "output exceeds window", limits: aimodel.Limits{ContextWindow: 128000, MaxOutputTokens: 1000000}, wantField: "limits.max_output_tokens"},
		{name: "safety margin consumes input", limits: aimodel.Limits{ContextWindow: 10, MaxOutputTokens: 9}, wantField: "limits.max_output_tokens"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := aimodel.ValidateDeploymentLimits(aimodel.ModelTypeChat, "enabled", tt.limits)
			var validationErr *aimodel.LimitsValidationError
			if !errors.As(err, &validationErr) || validationErr.Code != aimodel.ContextBudgetInvalid || validationErr.Field != tt.wantField {
				t.Fatalf("unexpected validation error: %#v", err)
			}
		})
	}

	budget, err := aimodel.ResolveContextBudget(aimodel.Limits{ContextWindow: 128000, MaxOutputTokens: 8192})
	if err != nil {
		t.Fatal(err)
	}
	if budget.SafetyMarginTokens != 12800 || budget.AvailableTokens != 107008 {
		t.Fatalf("unexpected budget: %+v", budget)
	}
	if err = aimodel.ValidateDeploymentLimits(aimodel.ModelTypeEmbedding, "enabled", aimodel.Limits{}); err != nil {
		t.Fatalf("chat limits applied to embedding: %v", err)
	}
	if err = aimodel.ValidateDeploymentLimits(aimodel.ModelTypeChat, "disabled", aimodel.Limits{}); err != nil {
		t.Fatalf("disabled chat model cannot remain pending: %v", err)
	}
}

func TestChatDeploymentLimitsRejectFractionalJSON(t *testing.T) {
	var limits aimodel.Limits
	if err := json.Unmarshal([]byte(`{"context_window":128000.5,"max_output_tokens":8192}`), &limits); err == nil {
		t.Fatal("fractional limits were accepted")
	}
}

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

func TestCredentialCipherEncryptsWithRandomNonceAndAAD(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	cipher, err := aimodel.NewAESGCMCredentialCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	aad := []byte("ai-provider:tenant-a:provider-a")
	first, err := cipher.Encrypt(ctx, "sk-secret", aad)
	if err != nil {
		t.Fatal(err)
	}
	second, err := cipher.Encrypt(ctx, "sk-secret", aad)
	if err != nil {
		t.Fatal(err)
	}
	if first == second || first == "sk-secret" {
		t.Fatal("ciphertext must use a random nonce and hide plaintext")
	}
	plaintext, err := cipher.Decrypt(ctx, first, aad)
	if err != nil || plaintext != "sk-secret" {
		t.Fatalf("unexpected decrypt: %q %v", plaintext, err)
	}
	if _, err := cipher.Decrypt(ctx, first, []byte("ai-provider:tenant-b:provider-a")); err == nil {
		t.Fatal("ciphertext must be bound to tenant and provider")
	}
	if _, err := aimodel.NewAESGCMCredentialCipher("not-a-key"); err == nil {
		t.Fatal("invalid encryption key must fail")
	}
}

func TestProviderJSONNeverExposesCredentialCiphertext(t *testing.T) {
	provider := ai.Provider{ID: "provider-a", CredentialCiphertext: "v1:classified", CredentialConfigured: true}
	encoded, err := json.Marshal(provider)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	if strings.Contains(text, "classified") || strings.Contains(text, "ciphertext") {
		t.Fatalf("credential leaked in JSON: %s", text)
	}
	if !strings.Contains(text, `"credential_configured":true`) {
		t.Fatalf("configured marker missing: %s", text)
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
