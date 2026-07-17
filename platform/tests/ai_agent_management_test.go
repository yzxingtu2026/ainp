package tests

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/aiagent"
	"github.com/yzxingtu2026/ainp/platform/modules/ai"
)

func validAgentContract() aiagent.Contract {
	return aiagent.Contract{Key: "demo.assistant", Version: "1.0.0", Name: "Demo", BaseInstructions: "Keep tenant boundaries.", RequiredModelCapabilities: []string{"chat", "tools"}, RequiredToolSelectors: []string{"meta:tool.search"}, ConfigSchema: json.RawMessage(`{"type":"object"}`), SingletonScope: aiagent.SingletonNone, ManagedFields: []string{"source", "contract_key"}, UpgradePolicy: "managed-fields"}
}

func TestAgentActionRequestsAcceptSnakeCaseJSON(t *testing.T) {
	create := action.JSON(func(_ context.Context, request ai.AgentCreateRequest) (any, error) { return request, nil })
	input := json.RawMessage(`{"code":"test","name":"测试","description":"","source":"personal","visibility":"private","model_alias":"test","user_instructions":"hhh","generation_config":{},"tool_policy":{}}`)
	result, err := create(context.Background(), input)
	if err != nil {
		t.Fatalf("decode create request: %v", err)
	}
	if result.(ai.AgentCreateRequest).ModelAlias != "test" {
		t.Fatalf("model_alias was not decoded: %#v", result)
	}

	empty := action.JSON(func(_ context.Context, request ai.AgentEmptyRequest) (any, error) { return request, nil })
	if _, err := empty(context.Background(), json.RawMessage(`{}`)); err != nil {
		t.Fatalf("decode reconcile request: %v", err)
	}
	status := action.JSON(func(_ context.Context, request ai.AgentStatusRequest) (any, error) { return request, nil })
	if _, err := status(context.Background(), json.RawMessage(`{"id":"agent-1","status":"enabled","revision":2}`)); err != nil {
		t.Fatalf("decode status request: %v", err)
	}
	search := action.JSON(func(_ context.Context, request ai.AgentToolSearchRequest) (any, error) { return request, nil })
	result, err = search(context.Background(), json.RawMessage(`{"query":"","limit":10}`))
	if err != nil {
		t.Fatalf("empty agent tool query must list available tools: %v", err)
	}
	if result.(ai.AgentToolSearchRequest).Limit != 10 {
		t.Fatalf("tool search limit was not decoded: %#v", result)
	}
}

func TestAgentContractRegistryFailClosed(t *testing.T) {
	r := aiagent.NewRegistry()
	c := validAgentContract()
	if err := r.Register(c); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(c); err == nil {
		t.Fatal("duplicate contract must fail")
	}
	bad := c
	bad.Version = "next"
	if err := aiagent.NewRegistry().Register(bad); err == nil {
		t.Fatal("invalid version must fail")
	}
	bad = c
	bad.RequiredModelCapabilities = []string{"unknown"}
	if err := aiagent.NewRegistry().Register(bad); err == nil {
		t.Fatal("unknown capability must fail")
	}
	bad = c
	bad.RequiredToolSelectors = []string{"tool.search"}
	if err := aiagent.NewRegistry().Register(bad); err == nil {
		t.Fatal("non-namespaced selector must fail")
	}
	bad = c
	bad.ManagedFields = []string{"tenant_id"}
	if err := aiagent.NewRegistry().Register(bad); err == nil {
		t.Fatal("unsafe managed field must fail")
	}
	if got, ok := r.Get(c.Key, c.Version); !ok || got.Name != c.Name {
		t.Fatalf("registered contract missing: %#v", got)
	}
}
