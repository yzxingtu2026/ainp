package tests

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/aitool"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/capability"
	"github.com/yzxingtu2026/ainp/platform/modules/ai"
)

func TestMemoryManagementActionsAreSelfServiceContracts(t *testing.T) {
	capabilities := capability.NewRegistry()
	if err := capabilities.Register(capability.Capability{ID: "ai", Name: "AI"}); err != nil {
		t.Fatal(err)
	}
	application := &app.App{Actions: action.NewRegistry(nil, nil), Capabilities: capabilities}
	if err := ai.RegisterMemoryManagement(application, nil); err != nil {
		t.Fatal(err)
	}
	readActions := []string{ai.ActionMemoryList, ai.ActionMemoryGet, ai.ActionMemoryProfile}
	mutationActions := []string{ai.ActionMemoryConfirm, ai.ActionMemoryReject, ai.ActionMemoryCorrect, ai.ActionMemoryDelete, ai.ActionMemoryClear}
	for _, id := range append(readActions, mutationActions...) {
		registered, ok := application.Actions.Get(id)
		if !ok {
			t.Fatalf("action %s was not registered", id)
		}
		if registered.Meta.Access != action.AccessAuthenticated || registered.Meta.Permission != "" {
			t.Fatalf("action %s could expose permission-based cross-user access: %#v", id, registered.Meta)
		}
		if registered.Meta.AI == nil || !registered.Meta.AI.ExposeTool || !json.Valid(registered.Meta.AI.InputSchema) {
			t.Fatalf("action %s must expose a valid AI contract: %#v", id, registered.Meta.AI)
		}
	}
	projected, err := (&aitool.ActionProvider{Actions: application.Actions}).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(projected) != len(readActions)+len(mutationActions) {
		t.Fatalf("projected memory actions = %d, want %d", len(projected), len(readActions)+len(mutationActions))
	}
	for _, id := range readActions {
		registered, _ := application.Actions.Get(id)
		if registered.Meta.AI.RequiresConfirmation || registered.Meta.AI.SideEffect != "read" {
			t.Fatalf("read memory action %s has unsafe metadata: %#v", id, registered.Meta.AI)
		}
	}
	for _, id := range mutationActions {
		registered, _ := application.Actions.Get(id)
		if registered.Meta.IdempotencyMode() != "required" {
			t.Fatalf("mutation %s must require idempotency", id)
		}
		if !registered.Meta.AI.RequiresConfirmation {
			t.Fatalf("mutation %s must require tool confirmation", id)
		}
	}
}
