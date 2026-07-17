package aiagentruntime

import "fmt"

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

type Budget struct {
	MaxSteps, MaxModelCalls, MaxToolCalls, MaxCompactions int
	MaxDurationSeconds                                    int
	MaxTokens                                             int64
}

func (b Budget) Normalize() Budget {
	if b.MaxSteps <= 0 {
		b.MaxSteps = 24
	}
	if b.MaxModelCalls <= 0 {
		b.MaxModelCalls = 8
	}
	if b.MaxToolCalls <= 0 {
		b.MaxToolCalls = 12
	}
	if b.MaxCompactions <= 0 {
		b.MaxCompactions = 2
	}
	if b.MaxDurationSeconds <= 0 {
		b.MaxDurationSeconds = 300
	}
	if b.MaxTokens <= 0 {
		b.MaxTokens = 100000
	}
	return b
}
func (b Budget) Validate() error {
	if b.MaxSteps < 1 || b.MaxSteps > 100 || b.MaxModelCalls < 1 || b.MaxModelCalls > 32 || b.MaxToolCalls < 0 || b.MaxToolCalls > 64 || b.MaxCompactions < 0 || b.MaxCompactions > 3 || b.MaxDurationSeconds < 10 || b.MaxDurationSeconds > 1800 || b.MaxTokens < 1 || b.MaxTokens > 10000000 {
		return fmt.Errorf("invalid agent run budget")
	}
	return nil
}
