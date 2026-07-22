package aiagentruntime

import (
	"encoding/json"
	"fmt"
)

type RunStatus string
type StepStatus string
type StepType string

const (
	RunQueued              RunStatus  = "queued"
	RunRunning             RunStatus  = "running"
	RunWaitingConfirmation RunStatus  = "waiting_confirmation"
	RunPaused              RunStatus  = "paused"
	RunCompleted           RunStatus  = "completed"
	RunFailed              RunStatus  = "failed"
	RunCancelled           RunStatus  = "cancelled"
	StepPending            StepStatus = "pending"
	StepRunning            StepStatus = "running"
	StepWaiting            StepStatus = "waiting"
	StepCompleted          StepStatus = "completed"
	StepFailed             StepStatus = "failed"
	StepSkipped            StepStatus = "skipped"
	StepCancelled          StepStatus = "cancelled"
	StepContextAssemble    StepType   = "context_assemble"
	StepContextCompact     StepType   = "context_compact"
	StepModel              StepType   = "model"
	StepToolSearch         StepType   = "tool_search"
	StepToolLoad           StepType   = "tool_load"
	StepToolExecute        StepType   = "tool_execute"
	StepConfirmation       StepType   = "confirmation"
	StepFinalize           StepType   = "finalize"
)

var runTransitions = map[RunStatus]map[RunStatus]bool{RunQueued: {RunRunning: true, RunCancelled: true}, RunRunning: {RunWaitingConfirmation: true, RunPaused: true, RunCompleted: true, RunFailed: true, RunCancelled: true}, RunWaitingConfirmation: {RunQueued: true, RunPaused: true, RunFailed: true, RunCancelled: true}, RunPaused: {RunQueued: true, RunCancelled: true}, RunFailed: {RunQueued: true}}
var stepTransitions = map[StepStatus]map[StepStatus]bool{StepPending: {StepRunning: true, StepSkipped: true, StepCancelled: true}, StepRunning: {StepWaiting: true, StepCompleted: true, StepFailed: true, StepCancelled: true}, StepWaiting: {StepRunning: true, StepCompleted: true, StepFailed: true, StepCancelled: true}}

func ValidateRunTransition(from, to RunStatus) error {
	if !runTransitions[from][to] {
		return fmt.Errorf("invalid run transition: %s -> %s", from, to)
	}
	return nil
}
func ValidateStepTransition(from, to StepStatus) error {
	if !stepTransitions[from][to] {
		return fmt.Errorf("invalid step transition: %s -> %s", from, to)
	}
	return nil
}
func ValidStepType(v StepType) bool {
	switch v {
	case StepContextAssemble, StepContextCompact, StepModel, StepToolSearch, StepToolLoad, StepToolExecute, StepConfirmation, StepFinalize:
		return true
	}
	return false
}
func RunTerminal(v RunStatus) bool { return v == RunCompleted || v == RunCancelled }

const BudgetSchemaVersion = 1

type Budget struct {
	SchemaVersion      int   `json:"schema_version"`
	MaxSteps           int   `json:"max_steps"`
	MaxModelCalls      int   `json:"max_model_calls"`
	MaxToolCalls       int   `json:"max_tool_calls"`
	MaxCompactions     int   `json:"max_compactions"`
	MaxDurationSeconds int   `json:"max_duration_seconds"`
	RunTokenBudget     int64 `json:"run_token_budget"`
}

func SystemDefaultBudget() Budget {
	return Budget{SchemaVersion: BudgetSchemaVersion, MaxSteps: 24, MaxModelCalls: 8, MaxToolCalls: 12, MaxCompactions: 2, MaxDurationSeconds: 300, RunTokenBudget: 100000}
}

func (b Budget) Normalize() Budget {
	d := SystemDefaultBudget()
	if b.SchemaVersion <= 0 {
		b.SchemaVersion = d.SchemaVersion
	}
	if b.MaxSteps <= 0 {
		b.MaxSteps = d.MaxSteps
	}
	if b.MaxModelCalls <= 0 {
		b.MaxModelCalls = d.MaxModelCalls
	}
	if b.MaxToolCalls <= 0 {
		b.MaxToolCalls = d.MaxToolCalls
	}
	if b.MaxCompactions <= 0 {
		b.MaxCompactions = d.MaxCompactions
	}
	if b.MaxDurationSeconds <= 0 {
		b.MaxDurationSeconds = d.MaxDurationSeconds
	}
	if b.RunTokenBudget <= 0 {
		b.RunTokenBudget = d.RunTokenBudget
	}
	return b
}
func (b Budget) Validate() error {
	if b.SchemaVersion != BudgetSchemaVersion || b.MaxSteps < 1 || b.MaxSteps > 100 || b.MaxModelCalls < 1 || b.MaxModelCalls > 32 || b.MaxToolCalls < 0 || b.MaxToolCalls > 64 || b.MaxCompactions < 0 || b.MaxCompactions > 3 || b.MaxDurationSeconds < 10 || b.MaxDurationSeconds > 1800 || b.RunTokenBudget < 1 || b.RunTokenBudget > 10000000 {
		return fmt.Errorf("invalid agent run budget")
	}
	return nil
}

func (b *Budget) UnmarshalJSON(data []byte) error {
	type current Budget
	var modern current
	if err := json.Unmarshal(data, &modern); err != nil {
		return err
	}
	var legacy struct {
		MaxSteps, MaxModelCalls, MaxToolCalls, MaxCompactions int
		MaxDurationSeconds                                    int
		MaxTokens                                             int64
	}
	if err := json.Unmarshal(data, &legacy); err != nil {
		return err
	}
	*b = Budget(modern)
	if b.MaxSteps == 0 {
		b.MaxSteps = legacy.MaxSteps
	}
	if b.MaxModelCalls == 0 {
		b.MaxModelCalls = legacy.MaxModelCalls
	}
	if b.MaxToolCalls == 0 {
		b.MaxToolCalls = legacy.MaxToolCalls
	}
	if b.MaxCompactions == 0 {
		b.MaxCompactions = legacy.MaxCompactions
	}
	if b.MaxDurationSeconds == 0 {
		b.MaxDurationSeconds = legacy.MaxDurationSeconds
	}
	if b.RunTokenBudget == 0 {
		b.RunTokenBudget = legacy.MaxTokens
	}
	if b.SchemaVersion == 0 {
		b.SchemaVersion = BudgetSchemaVersion
	}
	return nil
}
