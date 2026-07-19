package ai

import (
	"github.com/yzxingtu2026/ainp/platform/framework/aimemory"
	"testing"
)

func TestGovernanceOutcomeIsFailClosed(t *testing.T) {
	policy := aimemory.SystemDefaultPolicy()
	policy.AutoSaveLowSensitive = true
	base := MemoryItem{Type: aimemory.TypePreference, Content: "老板", AttributeKey: "preferred_address", AttributeValue: "老板", ExplicitUserIntent: true, Confidence: .95, Sensitivity: aimemory.SensitivityNormal}
	if action, _, _ := governanceOutcome(base, policy, nil); action != "activate" {
		t.Fatalf("explicit low-sensitive preference = %s", action)
	}
	sensitive := base
	sensitive.Sensitivity = aimemory.SensitivitySensitive
	if action, _, _ := governanceOutcome(sensitive, policy, nil); action != "pending_review" {
		t.Fatalf("sensitive candidate = %s", action)
	}
	inferred := base
	inferred.ExplicitUserIntent = false
	if action, _, _ := governanceOutcome(inferred, policy, nil); action != "pending_review" {
		t.Fatalf("inferred candidate = %s", action)
	}
	low := base
	low.Confidence = .1
	if action, _, _ := governanceOutcome(low, policy, nil); action != "reject" {
		t.Fatalf("low-confidence candidate = %s", action)
	}
	duplicate := base
	if action, _, related := governanceOutcome(base, policy, []MemoryItem{duplicate}); action != "duplicate" || related == nil {
		t.Fatalf("duplicate outcome = %s", action)
	}
	old := base
	old.AttributeValue = "老师"
	old.Content = "老师"
	if action, _, related := governanceOutcome(base, policy, []MemoryItem{old}); action != "supersede" || related == nil {
		t.Fatalf("correction outcome = %s", action)
	}
}

func TestConsolidationPlanRejectsRevisionDriftAndIllegalOperations(t *testing.T) {
	allowed := map[string]uint64{"a": 2, "b": 3}
	valid := consolidationPlan{Operations: []consolidationOperation{{Operation: "duplicate", FromItemID: "a", FromRevision: 2, ToItemID: "b", ToRevision: 3}}}
	if err := validateConsolidationPlan(valid, allowed); err != nil {
		t.Fatal(err)
	}
	valid.Operations[0].FromRevision = 1
	if err := validateConsolidationPlan(valid, allowed); err == nil {
		t.Fatal("revision drift must fail")
	}
	valid.Operations[0] = consolidationOperation{Operation: "delete", FromItemID: "a", FromRevision: 2}
	if err := validateConsolidationPlan(valid, allowed); err == nil {
		t.Fatal("delete plan must fail")
	}
	valid.Operations[0] = consolidationOperation{Operation: "conflict", FromItemID: "outside", FromRevision: 1, ToItemID: "b", ToRevision: 3}
	if err := validateConsolidationPlan(valid, allowed); err == nil {
		t.Fatal("out-of-scope item must fail")
	}
}
