package tests

import (
	"context"
	"errors"
	"testing"

	. "github.com/yzxingtu2026/ainp/platform/framework/setting"
)

func TestChangeRegistryRoutesBySettingKey(t *testing.T) {
	registry := NewChangeRegistry()
	called := 0
	if err := registry.Register("ai.navigation_search", func(_ context.Context, change Change) error {
		called++
		if change.Operation != ChangeUpdated || change.TenantID != "tenant-1" {
			t.Fatalf("unexpected change: %#v", change)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Notify(context.Background(), Change{TenantID: "tenant-1", ActorID: "user-1", Key: "other", Operation: ChangeUpdated}); err != nil {
		t.Fatal(err)
	}
	if called != 0 {
		t.Fatal("unrelated setting invoked handler")
	}
	if err := registry.Notify(context.Background(), Change{TenantID: "tenant-1", ActorID: "user-1", Key: "ai.navigation_search", Operation: ChangeUpdated}); err != nil {
		t.Fatal(err)
	}
	if called != 1 {
		t.Fatalf("handler calls = %d", called)
	}
}

func TestChangeRegistryPropagatesHandlerFailure(t *testing.T) {
	registry := NewChangeRegistry()
	expected := errors.New("reconcile failed")
	if err := registry.Register("key", func(context.Context, Change) error { return expected }); err != nil {
		t.Fatal(err)
	}
	if err := registry.Notify(context.Background(), Change{TenantID: "tenant-1", Key: "key", Operation: ChangeReset}); !errors.Is(err, expected) {
		t.Fatalf("unexpected error: %v", err)
	}
}
