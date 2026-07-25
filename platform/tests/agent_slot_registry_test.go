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
	err = r.Register(aiagent.Contract{Key: "system.operations", Version: "2.2.0", Name: "Operations", BaseInstructions: "safe", RequiredModelCapabilities: []string{"chat", "tools"}, ConfigSchema: json.RawMessage(`{"type":"object"}`), SingletonScope: aiagent.SingletonTenant, UpgradePolicy: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestCompatibleSlotAcceptsOnlyRegisteredContractVersions(t *testing.T) {
	r := NewRegistry(contracts(t))
	slot := validSlot()
	if !r.AcceptsContract(slot, "system.operations", "1.0.0") || !r.AcceptsContract(slot, "system.operations", "2.2.0") {
		t.Fatal("compatible slot must accept registered versions of its contract")
	}
	if r.AcceptsContract(slot, "system.operations", "9.9.9") || r.AcceptsContract(slot, "other.contract", "1.0.0") {
		t.Fatal("compatible slot must reject unknown contract references")
	}
	slot.UpgradePolicy = UpgradePinned
	if r.AcceptsContract(slot, "system.operations", "2.2.0") {
		t.Fatal("pinned slot must reject other contract versions")
	}
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
