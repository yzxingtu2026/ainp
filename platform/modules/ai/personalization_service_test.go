package ai

import (
	"strings"
	"testing"

	"github.com/yzxingtu2026/ainp/platform/framework/aimodelruntime"
)

func TestPersonalizationAcceptsInvocationRuntimeValueResponse(t *testing.T) {
	want := aimodelruntime.GenerateResponse{ID: "response-1"}
	got, ok := asGenerateResponse(any(want))
	if !ok {
		t.Fatal("InvocationService.Generate value response must be accepted")
	}
	if got.ID != want.ID {
		t.Fatalf("unexpected response id: %q", got.ID)
	}
}

func TestSafeProfileAppliesFieldAndLengthBudgets(t *testing.T) {
	raw := []byte(`{"preferred_address":"杨老师","facts":{"project":"AINP","long":"` + strings.Repeat("中", 200) + `"},"ignored":["raw"]}`)
	profile := safeProfile(raw)
	if profile["preferred_address"] != "杨老师" {
		t.Fatalf("unexpected preferred address: %#v", profile)
	}
	facts, ok := profile["facts"].(map[string]any)
	if !ok || facts["project"] != "AINP" {
		t.Fatalf("unexpected facts projection: %#v", profile)
	}
	if len([]rune(facts["long"].(string))) != 160 {
		t.Fatal("nested profile value was not truncated")
	}
	if _, exists := profile["ignored"]; exists {
		t.Fatal("unsupported raw profile values must not be projected")
	}
}
