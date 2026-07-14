package tests

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
)

func TestRegistryDefaultsSurfaceAndRejectsWrongRealm(t *testing.T) {
	r := action.NewRegistry(nil, nil)
	called := false
	err := r.Register(action.Action{ID: "demo.run", Meta: action.Metadata{Capability: "demo", HTTP: &action.HTTPMeta{Public: true}}, Handler: func(context.Context, json.RawMessage) (any, error) {
		called = true
		return nil, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	a, _ := r.Get("demo.run")
	if a.Meta.Surface != auth.RealmTenant {
		t.Fatalf("expected tenant default, got %q", a.Meta.Surface)
	}
	ctx := auth.WithContext(context.Background(), &auth.Context{Realm: auth.RealmPlatform, Authenticated: true})
	if _, err := r.Invoke(ctx, "demo.run", nil); err == nil || called {
		t.Fatal("platform realm must not invoke tenant action")
	}
}

func TestRegistryRejectsInvalidSurface(t *testing.T) {
	r := action.NewRegistry(nil, nil)
	err := r.Register(action.Action{ID: "demo.run", Meta: action.Metadata{Capability: "demo", Surface: "other"}, Handler: func(context.Context, json.RawMessage) (any, error) { return nil, nil }})
	if err == nil {
		t.Fatal("expected invalid surface registration error")
	}
}
