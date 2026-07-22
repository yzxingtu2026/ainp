package ai

import (
	"context"
	"fmt"
	"math"

	"github.com/yzxingtu2026/ainp/platform/framework/aiagentruntime"
	"github.com/yzxingtu2026/ainp/platform/framework/setting"
)

const SettingKeyAgentRuntimeBudget = "ai.agent_runtime_budget"

const (
	RuntimeBudgetSourceTenant          = "tenant_setting"
	RuntimeBudgetSourceBuiltinFallback = "builtin_fallback"
)

type runtimeBudgetSettingProvider interface {
	Get(context.Context, string, string) (*setting.Effective, error)
}

type RuntimeBudgetProvider struct {
	settings runtimeBudgetSettingProvider
}

func NewRuntimeBudgetProvider(settings runtimeBudgetSettingProvider) *RuntimeBudgetProvider {
	return &RuntimeBudgetProvider{settings: settings}
}

func (p *RuntimeBudgetProvider) Resolve(ctx context.Context, tenantID string) (aiagentruntime.Budget, string, error) {
	if tenantID == "" {
		return aiagentruntime.Budget{}, "", fmt.Errorf("tenant id required")
	}
	if p == nil || p.settings == nil {
		budget := aiagentruntime.SystemDefaultBudget()
		return budget, RuntimeBudgetSourceBuiltinFallback, nil
	}
	effective, err := p.settings.Get(ctx, tenantID, SettingKeyAgentRuntimeBudget)
	if err != nil {
		return aiagentruntime.Budget{}, "", fmt.Errorf("resolve agent runtime budget: %w", err)
	}
	if effective == nil {
		return aiagentruntime.Budget{}, "", fmt.Errorf("agent runtime budget setting unavailable")
	}
	values := effective.Values
	budget := aiagentruntime.Budget{SchemaVersion: aiagentruntime.BudgetSchemaVersion}
	if budget.MaxSteps, err = settingInt(values, "max_steps"); err != nil {
		return aiagentruntime.Budget{}, "", err
	}
	if budget.MaxModelCalls, err = settingInt(values, "max_model_calls"); err != nil {
		return aiagentruntime.Budget{}, "", err
	}
	if budget.MaxToolCalls, err = settingInt(values, "max_tool_calls"); err != nil {
		return aiagentruntime.Budget{}, "", err
	}
	if budget.MaxCompactions, err = settingInt(values, "max_compactions"); err != nil {
		return aiagentruntime.Budget{}, "", err
	}
	if budget.MaxDurationSeconds, err = settingInt(values, "max_duration_seconds"); err != nil {
		return aiagentruntime.Budget{}, "", err
	}
	tokens, err := settingInt(values, "run_token_budget")
	if err != nil {
		return aiagentruntime.Budget{}, "", err
	}
	budget.RunTokenBudget = int64(tokens)
	if err = budget.Validate(); err != nil {
		return aiagentruntime.Budget{}, "", err
	}
	source := RuntimeBudgetSourceTenant
	if effective.Source == "default" {
		source = RuntimeBudgetSourceBuiltinFallback
	}
	return budget, source, nil
}

func settingInt(values map[string]any, key string) (int, error) {
	value, ok := values[key]
	if !ok {
		return 0, fmt.Errorf("agent runtime budget field missing: %s", key)
	}
	var number float64
	switch typed := value.(type) {
	case float64:
		number = typed
	case float32:
		number = float64(typed)
	case int:
		return typed, nil
	case int64:
		if typed > math.MaxInt || typed < math.MinInt {
			return 0, fmt.Errorf("agent runtime budget field out of range: %s", key)
		}
		return int(typed), nil
	case jsonNumber:
		parsed, err := typed.Int64()
		if err != nil || parsed > math.MaxInt || parsed < math.MinInt {
			return 0, fmt.Errorf("agent runtime budget field must be an integer: %s", key)
		}
		return int(parsed), nil
	default:
		return 0, fmt.Errorf("agent runtime budget field must be a number: %s", key)
	}
	if math.IsNaN(number) || math.IsInf(number, 0) || math.Trunc(number) != number || number > math.MaxInt || number < math.MinInt {
		return 0, fmt.Errorf("agent runtime budget field must be an integer: %s", key)
	}
	return int(number), nil
}

type jsonNumber interface {
	Int64() (int64, error)
}
