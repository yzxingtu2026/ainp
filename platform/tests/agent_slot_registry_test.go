package tests

import (
	"context"
	"encoding/json"
	"testing"

	. "github.com/yzxingtu2026/ainp/platform/framework/agentslot"
	"github.com/yzxingtu2026/ainp/platform/framework/aiagent"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/modules/ai"
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

func TestWorkspaceDefaultSlotRejectsCommunityActor(t *testing.T) {
	r := NewRegistry(contracts(t))
	if err := r.Register(validSlot()); err != nil {
		t.Fatal(err)
	}
	communitySlot := validSlot()
	communitySlot.Key = "community.neighbor"
	communitySlot.PrincipalKinds = []auth.PrincipalKind{"hoa.resident"}
	communitySlot.ClientSurfaces = []string{"hoa.community"}
	if err := r.Register(communitySlot); err != nil {
		t.Fatal(err)
	}
	current, _ := r.Current("workspace.default")
	if len(current.PrincipalKinds) == 0 || current.PrincipalKinds[0] != auth.PrincipalKindAccount {
		t.Fatalf("workspace.default must default to account: %#v", current)
	}
	account := &auth.Context{Authenticated: true, TenantID: "t1", PrincipalID: "a1", Principal: auth.PrincipalRef{Kind: auth.PrincipalKindAccount, ID: "a1"}, ClientSurface: auth.TenantConsoleSurface}
	resident := &auth.Context{Authenticated: true, TenantID: "t1", PrincipalID: "r1", Principal: auth.PrincipalRef{Kind: "hoa.resident", ID: "r1"}, ClientSurface: "hoa.community"}
	if !current.AllowsActor(account) || current.AllowsActor(resident) {
		t.Fatal("workspace.default must stay tenant-console only")
	}
	neighbor, _ := r.Current("community.neighbor")
	if neighbor.AllowsActor(account) || !neighbor.AllowsActor(resident) {
		t.Fatal("community slot must not resolve for staff account")
	}
	svc := ai.NewAgentSlotService(nil, r, nil)
	_, err := svc.Resolve(auth.WithContext(context.Background(), resident), ai.AgentSlotGetRequest{SlotKey: ai.WorkspaceDefaultSlot})
	if err == nil || errorCode(err) != errs.CodeNotFound {
		t.Fatalf("community actor must not resolve workspace.default: %v", err)
	}
}
