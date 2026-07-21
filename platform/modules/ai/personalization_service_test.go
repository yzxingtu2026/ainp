package ai

import (
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
