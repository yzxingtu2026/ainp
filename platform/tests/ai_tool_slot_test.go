package tests

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/yzxingtu2026/ainp/platform/framework/aitool"
)

type fakeRuntimeSlot struct {
	id         string
	runtimeRef string
	direct     bool
	visible    bool
}

func (s *fakeRuntimeSlot) Definition() aitool.Definition {
	return aitool.Definition{ID: s.id, Source: aitool.SourceBuiltin, Name: s.id, TitleForHuman: s.id, DescriptionForLLM: "fake tool", DescriptionForHuman: "fake tool", Capability: "test", InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`), Risk: aitool.RiskLow, SideEffect: aitool.SideEffectRead, BindingMode: aitool.BindingOptional, LoadingMode: aitool.LoadingConditional, RuntimeRef: s.runtimeRef, SchemaVersion: 1, Status: "enabled"}
}
func (s *fakeRuntimeSlot) DiscoveryMode() aitool.DiscoveryMode {
	if s.direct {
		return aitool.DiscoveryDirect
	}
	return aitool.DiscoveryProgressive
}
func (s *fakeRuntimeSlot) Project(_ context.Context, scope aitool.RuntimeScope, d aitool.Definition) (aitool.Definition, bool, error) {
	d.DescriptionForLLM += ":" + scope.PrincipalID
	return d, s.visible, nil
}
func (s *fakeRuntimeSlot) Metadata() aitool.RuntimeMetadata {
	return aitool.RuntimeMetadata{StepType: "tool_execute", InvocationClass: "fake", BudgetCategory: "fake", MaxCallsPerRun: 2, Observations: []aitool.RuntimeObservation{{Field: "count", Source: "result_data"}}, FailureCategories: []string{"fake_failed"}}
}
func (s *fakeRuntimeSlot) Execute(context.Context, aitool.LoadedTool, json.RawMessage) (*aitool.Result, error) {
	return &aitool.Result{Status: "completed", Summary: "done"}, nil
}

func TestRuntimeToolSlotsAreStableComposableAndExecutable(t *testing.T) {
	r := aitool.NewRegistry()
	b := &fakeRuntimeSlot{id: "builtin:test.b", runtimeRef: "test.b", visible: true}
	a := &fakeRuntimeSlot{id: "builtin:test.a", runtimeRef: "test.a", direct: true, visible: true}
	if err := r.RegisterSlot(b); err != nil {
		t.Fatal(err)
	}
	if err := r.RegisterSlot(a); err != nil {
		t.Fatal(err)
	}
	if err := r.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	items := r.List()
	if len(items) != 2 || items[0].ID != a.id || items[1].ID != b.id {
		t.Fatalf("slots are not stable: %#v", items)
	}
	if direct := r.DirectSlotIDs(); len(direct) != 1 || direct[0] != a.id {
		t.Fatalf("direct slots mismatch: %#v", direct)
	}
	if !r.IsBindable(a.id) || !r.IsBindable(b.id) {
		t.Fatal("runtime slots explicitly marked optional must be bindable")
	}
	projected, visible, err := r.Project(context.Background(), aitool.RuntimeScope{PrincipalID: "u1"}, items[0])
	if err != nil || !visible || !strings.HasSuffix(projected.DescriptionForLLM, ":u1") {
		t.Fatalf("dynamic projection failed: %#v %v %v", projected, visible, err)
	}
	executor, ok := r.ExecutorFor(items[0])
	if !ok {
		t.Fatal("slot executor not resolved")
	}
	result, err := executor.Execute(context.Background(), aitool.LoadedTool{Definition: items[0]}, json.RawMessage(`{}`))
	if err != nil || result.Status != "completed" {
		t.Fatalf("slot execution failed: %#v %v", result, err)
	}
	metadata, ok := r.SlotMetadata(a.id)
	if !ok || metadata.MaxCallsPerRun != 2 || metadata.Observations[0].Field != "count" {
		t.Fatalf("slot metadata mismatch: %#v", metadata)
	}
}

func TestRuntimeToolSlotRegistrationFailsClosed(t *testing.T) {
	r := aitool.NewRegistry()
	first := &fakeRuntimeSlot{id: "builtin:test.first", runtimeRef: "test.shared", visible: true}
	if err := r.RegisterSlot(first); err != nil {
		t.Fatal(err)
	}
	if err := r.RegisterSlot(first); err == nil || !strings.Contains(err.Error(), "already registered") {
		t.Fatalf("duplicate slot was accepted: %v", err)
	}
	if err := r.RegisterSlot(&fakeRuntimeSlot{id: "builtin:test.second", runtimeRef: "test.shared", visible: true}); err == nil || !strings.Contains(err.Error(), "runtime ref") {
		t.Fatalf("duplicate runtime ref was accepted: %v", err)
	}
}

func TestRuntimeToolSlotRejectsInvalidMetadata(t *testing.T) {
	r := aitool.NewRegistry()
	invalid := &invalidMetadataSlot{fakeRuntimeSlot: fakeRuntimeSlot{id: "builtin:test.invalid", runtimeRef: "test.invalid", visible: true}}
	if err := r.RegisterSlot(invalid); err == nil || !strings.Contains(err.Error(), "metadata") {
		t.Fatalf("invalid metadata was accepted: %v", err)
	}
}

type invalidMetadataSlot struct{ fakeRuntimeSlot }

func (*invalidMetadataSlot) Metadata() aitool.RuntimeMetadata {
	return aitool.RuntimeMetadata{StepType: "business_specific", InvocationClass: "fake"}
}
