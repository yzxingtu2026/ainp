package tests

import (
	"encoding/json"
	"testing"

	. "github.com/yzxingtu2026/ainp/platform/framework/agentslot"
	"github.com/yzxingtu2026/ainp/platform/framework/aiagent"
)

func contracts(t *testing.T) *aiagent.Registry {
	t.Helper()
	r := aiagent.NewRegistry()
	err := r.Register(aiagent.Contract{Key: "system.operations", Version: "1.0.0", Name: "Operations", BaseInstructions: "safe", RequiredModelCapabilities: []string{"chat", "tools"}, ConfigSchema: json.RawMessage(`{"type":"object"}`), SingletonScope: aiagent.SingletonTenant, UpgradePolicy: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func validSlot() Slot {
	return Slot{Key: "workspace.default", Version: "1.0.0", Module: "workspace", Title: "Default", DefaultContractKey: "system.operations", DefaultContractVersion: "1.0.0", RequiredCapabilities: []string{"chat"}, TenantOverride: true, UserOverride: true, FallbackPolicy: FallbackFailClosed, UpgradePolicy: UpgradeCompatible}
}

func TestRegistryRejectsDuplicateAndUnsafeSlots(t *testing.T) {
	r := NewRegistry(contracts(t))
	slot := validSlot()
	if err := r.Register(slot); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(slot); err == nil {
		t.Fatal("expected duplicate registration to fail")
	}
	tests := []Slot{
		func() Slot { s := validSlot(); s.Key = ""; return s }(),
		func() Slot { s := validSlot(); s.DefaultContractKey = "unknown.contract"; return s }(),
		func() Slot { s := validSlot(); s.RequiredCapabilities = []string{"audio"}; return s }(),
		func() Slot { s := validSlot(); s.TenantOverride = false; return s }(),
		func() Slot { s := validSlot(); s.FallbackPolicy = "fallback"; return s }(),
	}
	for i, candidate := range tests {
		if err := NewRegistry(contracts(t)).Register(candidate); err == nil {
			t.Fatalf("case %d should fail", i)
		}
	}
}

func TestRegistryCurrentAndList(t *testing.T) {
	r := NewRegistry(contracts(t))
	first := validSlot()
	second := first
	second.Version = "2.0.0"
	if err := r.Register(second); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(first); err != nil {
		t.Fatal(err)
	}
	current, ok := r.Current(first.Key)
	if !ok || current.Version != "2.0.0" {
		t.Fatalf("unexpected current slot: %#v", current)
	}
	if got := r.List(); len(got) != 2 || got[0].Version != "1.0.0" {
		t.Fatalf("unexpected list: %#v", got)
	}
}
