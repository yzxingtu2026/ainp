package tests

import (
	"testing"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
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
	}
	for _, id := range mutationActions {
		registered, _ := application.Actions.Get(id)
		if registered.Meta.IdempotencyMode() != "required" {
			t.Fatalf("mutation %s must require idempotency", id)
		}
	}
}
