package aiagentruntime

import "fmt"

type ExecutionMode string

const (
	ExecutionModeNormal         ExecutionMode = "normal"
	ExecutionModeCompletionOnly ExecutionMode = "completion_only"
	MinNormalOutputTokens                     = int64(256)
	MinCompletionOutputTokens                 = int64(128)
)

type CompletionReserve struct {
	InputTokens  int64
	OutputTokens int64
	SafetyTokens int64
}

type TurnBudgetInput struct {
	HardLimit           int64
	ConsumedInput       int64
	ConsumedOutput      int64
	EstimatedInput      int64
	DeploymentMaxOutput int64
	Reserve             CompletionReserve
	CurrentMode         ExecutionMode
	CompletionAttempted bool
}

type TurnBudgetPlan struct {
	Mode            ExecutionMode
	MaxOutputTokens int64
	RemainingTokens int64
	SoftRemaining   int64
	Reason          string
	Allowed         bool
}

type TurnResultInput struct {
	HasUsableText    bool
	ToolCallCount    int
	CompletionOnly   bool
	BudgetCompletion bool
	HardExceeded     bool
}

type TurnResultPlan struct {
	CommitFinal      bool
	ExecuteTools     bool
	IgnoreToolCalls  bool
	CompletionReason string
	ErrorCategory    string
}

func DefaultCompletionReserve(hardLimit, deploymentMaxOutput, estimatedInput int64) CompletionReserve {
	safety := clamp(hardLimit/100, 64, 512)
	output := clamp(hardLimit/10, 256, 2048)
	if deploymentMaxOutput > 0 && output > deploymentMaxOutput {
		output = deploymentMaxOutput
	}
	return CompletionReserve{InputTokens: estimatedInput, OutputTokens: output, SafetyTokens: safety}
}

func PlanTurn(in TurnBudgetInput) (TurnBudgetPlan, error) {
	if in.HardLimit <= 0 || in.EstimatedInput <= 0 || in.DeploymentMaxOutput <= 0 {
		return TurnBudgetPlan{}, fmt.Errorf("invalid turn budget input")
	}
	if in.CurrentMode == "" {
		in.CurrentMode = ExecutionModeNormal
	}
	if in.CurrentMode != ExecutionModeNormal && in.CurrentMode != ExecutionModeCompletionOnly {
		return TurnBudgetPlan{}, fmt.Errorf("invalid execution mode")
	}
	if in.Reserve.InputTokens < 0 || in.Reserve.OutputTokens < 0 || in.Reserve.SafetyTokens < 0 {
		return TurnBudgetPlan{}, fmt.Errorf("invalid completion reserve")
	}

	remaining := in.HardLimit - in.ConsumedInput - in.ConsumedOutput
	plan := TurnBudgetPlan{Mode: in.CurrentMode, RemainingTokens: remaining}
	if remaining <= 0 {
		plan.Reason = "hard_budget_exhausted"
		return plan, nil
	}

	if in.CurrentMode == ExecutionModeCompletionOnly {
		if in.CompletionAttempted {
			plan.Reason = "completion_already_attempted"
			return plan, nil
		}
		return completionPlan(plan, in)
	}

	plan.SoftRemaining = remaining - in.Reserve.InputTokens - in.Reserve.OutputTokens - in.Reserve.SafetyTokens
	normalOutput := min64(in.DeploymentMaxOutput, plan.SoftRemaining-in.EstimatedInput)
	if normalOutput >= MinNormalOutputTokens {
		plan.Mode = ExecutionModeNormal
		plan.MaxOutputTokens = normalOutput
		plan.Reason = "within_soft_budget"
		plan.Allowed = true
		return plan, nil
	}
	plan.Mode = ExecutionModeCompletionOnly
	plan.Reason = "run_budget_pressure"
	return completionPlan(plan, in)
}

func PlanTurnResult(in TurnResultInput) TurnResultPlan {
	if in.ToolCallCount == 0 {
		if !in.HasUsableText {
			category := "model_empty_output"
			if in.CompletionOnly || in.HardExceeded {
				category = "budget_completion_invalid_output"
			}
			return TurnResultPlan{ErrorCategory: category}
		}
		plan := TurnResultPlan{CommitFinal: true}
		if in.BudgetCompletion || in.HardExceeded {
			plan.CompletionReason = "budget_limit"
		}
		return plan
	}
	if in.CompletionOnly {
		if !in.HasUsableText {
			return TurnResultPlan{IgnoreToolCalls: true, ErrorCategory: "budget_completion_invalid_output"}
		}
		plan := TurnResultPlan{CommitFinal: true, IgnoreToolCalls: true}
		if in.BudgetCompletion || in.HardExceeded {
			plan.CompletionReason = "budget_limit"
		}
		return plan
	}
	if in.HardExceeded {
		return TurnResultPlan{IgnoreToolCalls: true, ErrorCategory: "budget_hard_limit_exceeded"}
	}
	return TurnResultPlan{ExecuteTools: true}
}

func completionPlan(plan TurnBudgetPlan, in TurnBudgetInput) (TurnBudgetPlan, error) {
	availableOutput := plan.RemainingTokens - in.EstimatedInput - in.Reserve.SafetyTokens
	plan.MaxOutputTokens = min64(in.DeploymentMaxOutput, availableOutput)
	if plan.MaxOutputTokens < MinCompletionOutputTokens {
		plan.MaxOutputTokens = 0
		plan.Allowed = false
		plan.Reason = "completion_cannot_fit"
		return plan, nil
	}
	plan.Allowed = true
	return plan, nil
}

func clamp(value, minimum, maximum int64) int64 {
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
