package tests

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/yzxingtu2026/ainp/platform/framework/aiagentruntime"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodel"
	"github.com/yzxingtu2026/ainp/platform/framework/setting"
	"github.com/yzxingtu2026/ainp/platform/modules/ai"
)

func TestAgentRuntimeBudgetJSONUsesVersionedSnakeCase(t *testing.T) {
	budget := aiagentruntime.SystemDefaultBudget()
	data, err := json.Marshal(budget)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err = json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"schema_version", "max_steps", "max_model_calls", "max_tool_calls", "max_compactions", "max_duration_seconds", "run_token_budget"} {
		if _, ok := raw[key]; !ok {
			t.Fatalf("budget JSON missing %q: %s", key, data)
		}
	}
	if _, ok := raw["MaxTokens"]; ok {
		t.Fatalf("new budget JSON leaked legacy MaxTokens: %s", data)
	}
}

func TestAgentRuntimeBudgetReadsLegacyPascalCase(t *testing.T) {
	legacy := []byte(`{"MaxSteps":20,"MaxModelCalls":7,"MaxToolCalls":11,"MaxCompactions":1,"MaxDurationSeconds":240,"MaxTokens":76543}`)
	var budget aiagentruntime.Budget
	if err := json.Unmarshal(legacy, &budget); err != nil {
		t.Fatal(err)
	}
	if err := budget.Validate(); err != nil {
		t.Fatal(err)
	}
	if budget.SchemaVersion != aiagentruntime.BudgetSchemaVersion || budget.RunTokenBudget != 76543 || budget.MaxSteps != 20 {
		t.Fatalf("legacy budget decoded incorrectly: %+v", budget)
	}
}

func TestAgentRuntimeBudgetPreservesExplicitZeroLimits(t *testing.T) {
	data := []byte(`{"schema_version":1,"max_steps":24,"max_model_calls":8,"max_tool_calls":0,"max_compactions":0,"max_duration_seconds":300,"run_token_budget":100000}`)
	var budget aiagentruntime.Budget
	if err := json.Unmarshal(data, &budget); err != nil {
		t.Fatal(err)
	}
	if err := budget.Validate(); err != nil {
		t.Fatal(err)
	}
	if budget.MaxToolCalls != 0 || budget.MaxCompactions != 0 {
		t.Fatalf("explicit zero limits were changed: %+v", budget)
	}
}

type runtimeBudgetSettingsStub struct {
	values map[string]map[string]any
}

func (s runtimeBudgetSettingsStub) Get(_ context.Context, tenantID, key string) (*setting.Effective, error) {
	return &setting.Effective{Values: s.values[tenantID], Source: "tenant", SchemaVersion: 1}, nil
}

func runtimeBudgetValues(maxSteps float64) map[string]any {
	return map[string]any{
		"max_steps": maxSteps, "max_model_calls": float64(8), "max_tool_calls": float64(12),
		"max_compactions": float64(2), "max_duration_seconds": float64(300), "run_token_budget": float64(100000),
	}
}

func TestRuntimeBudgetProviderResolvesTenantValuesWithoutCrossTenantLeak(t *testing.T) {
	provider := ai.NewRuntimeBudgetProvider(runtimeBudgetSettingsStub{values: map[string]map[string]any{
		"tenant-a": runtimeBudgetValues(10),
		"tenant-b": runtimeBudgetValues(30),
	}})
	a, source, err := provider.Resolve(context.Background(), "tenant-a")
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := provider.Resolve(context.Background(), "tenant-b")
	if err != nil {
		t.Fatal(err)
	}
	if source != ai.RuntimeBudgetSourceTenant || a.MaxSteps != 10 || b.MaxSteps != 30 {
		t.Fatalf("unexpected tenant budgets: source=%s a=%+v b=%+v", source, a, b)
	}
}

func TestRuntimeBudgetProviderUsesBuiltinFallbackWithoutSettings(t *testing.T) {
	budget, source, err := ai.NewRuntimeBudgetProvider(nil).Resolve(context.Background(), "tenant-a")
	if err != nil {
		t.Fatal(err)
	}
	if source != ai.RuntimeBudgetSourceBuiltinFallback || budget != aiagentruntime.SystemDefaultBudget() {
		t.Fatalf("unexpected fallback budget: source=%s budget=%+v", source, budget)
	}
}

func TestRuntimeBudgetProviderFailsClosedForInvalidValues(t *testing.T) {
	values := runtimeBudgetValues(101)
	provider := ai.NewRuntimeBudgetProvider(runtimeBudgetSettingsStub{values: map[string]map[string]any{"tenant-a": values}})
	if _, _, err := provider.Resolve(context.Background(), "tenant-a"); err == nil {
		t.Fatal("out-of-range tenant budget was accepted")
	}
	values = runtimeBudgetValues(10.5)
	if _, _, err := provider.Resolve(context.Background(), "tenant-a"); err == nil {
		t.Fatal("fractional tenant budget was accepted")
	}
}

func TestAgentRunStartIgnoresLegacyClientBudget(t *testing.T) {
	var request ai.AgentRunStartRequest
	if err := json.Unmarshal([]byte(`{"conversation_id":"c1","trigger_message_id":"m1","budget":{"max_steps":100}}`), &request); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err = json.Unmarshal(encoded, &raw); err != nil {
		t.Fatal(err)
	}
	if _, ok := raw["budget"]; ok {
		t.Fatalf("client budget remains in start request: %s", encoded)
	}
}

func TestModelContextBudgetIsIndependentFromRunTokenBudget(t *testing.T) {
	budget, err := ai.ResolveModelContextBudget(aimodel.Limits{ContextWindow: 32000, MaxOutputTokens: 4000})
	if err != nil {
		t.Fatal(err)
	}
	if budget.ReservedOutputTokens != 4000 || budget.SafetyMarginTokens != 3200 || budget.AvailableTokens != 24800 {
		t.Fatalf("unexpected context budget: %+v", budget)
	}
	withDefaultOutput, err := ai.ResolveModelContextBudget(aimodel.Limits{ContextWindow: 10000})
	if err != nil {
		t.Fatal(err)
	}
	if withDefaultOutput.AvailableTokens != 6952 {
		t.Fatalf("unexpected default-output context budget: %+v", withDefaultOutput)
	}
}
