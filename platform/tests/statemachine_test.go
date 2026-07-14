package tests

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/codeset"
	"github.com/yzxingtu2026/ainp/platform/framework/statemachine"
)

func TestStateMachineValidation(t *testing.T) {
	sets := codeset.NewRegistry()
	set := codeset.CodeSet{Key: "demo.status", NameKey: "codeset.demo.status", Items: []codeset.CodeItem{
		{Value: "pending", LabelKey: "codeset.demo.status.pending", Semantic: codeset.SemanticInfo, Order: 10},
		{Value: "completed", LabelKey: "codeset.demo.status.completed", Semantic: codeset.SemanticSuccess, Order: 20},
	}}
	if err := sets.Register(set); err != nil {
		t.Fatal(err)
	}
	actions := action.NewRegistry(nil, nil)
	if err := actions.Register(action.Action{ID: "demo.complete", Handler: func(context.Context, json.RawMessage) (any, error) { return nil, nil }, Meta: action.Metadata{Capability: "demo"}}); err != nil {
		t.Fatal(err)
	}
	r := statemachine.NewRegistry()
	machine := statemachine.StateMachine{Key: "demo.lifecycle", CodeSetKey: "demo.status", Terminal: []string{"completed"}, Transitions: []statemachine.Transition{{From: "pending", To: "completed", Action: "demo.complete"}}}
	if err := r.Register(machine, sets, actions); err != nil {
		t.Fatal(err)
	}
	if !r.Can("demo.lifecycle", "pending", "completed", "demo.complete") {
		t.Fatal("registered transition must be allowed")
	}
	machine.Key = "demo.invalid"
	machine.Transitions = []statemachine.Transition{{From: "completed", To: "pending", Action: "demo.complete"}}
	if err := r.Register(machine, sets, actions); err == nil {
		t.Fatal("terminal outgoing transition must fail")
	}
}
