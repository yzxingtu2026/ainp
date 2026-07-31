package conversation

import (
	"github.com/yzxingtu2026/ainp/platform/framework/aiagentruntime"
	"github.com/yzxingtu2026/ainp/platform/framework/codeset"
)

const (
	CodeSetAgentRunStatus  = "ai.agent_run_status"
	CodeSetAgentStepStatus = "ai.agent_step_status"
	StateMachineAgentRun   = "ai.agent_run_lifecycle"
)

func agentRunStatusCodeSet() codeset.CodeSet {
	return codeset.CodeSet{
		Key: CodeSetAgentRunStatus, NameKey: "codeset.ai.agent_run_status", Order: 220,
		Items: []codeset.CodeItem{
			{Value: string(aiagentruntime.RunQueued), LabelKey: "codeset.ai.agent_run_status.queued", Semantic: codeset.SemanticInfo, Icon: "mdi:clock-outline", Order: 10},
			{Value: string(aiagentruntime.RunRunning), LabelKey: "codeset.ai.agent_run_status.running", Semantic: codeset.SemanticInfo, Icon: "mdi:play-circle-outline", Order: 20},
			{Value: string(aiagentruntime.RunWaitingConfirmation), LabelKey: "codeset.ai.agent_run_status.waiting_confirmation", Semantic: codeset.SemanticWarning, Icon: "mdi:account-check-outline", Order: 30},
			{Value: string(aiagentruntime.RunPaused), LabelKey: "codeset.ai.agent_run_status.paused", Semantic: codeset.SemanticWarning, Icon: "mdi:pause-circle-outline", Order: 40},
			{Value: string(aiagentruntime.RunCompleted), LabelKey: "codeset.ai.agent_run_status.completed", Semantic: codeset.SemanticSuccess, Icon: "mdi:check-circle-outline", Order: 50},
			{Value: string(aiagentruntime.RunFailed), LabelKey: "codeset.ai.agent_run_status.failed", Semantic: codeset.SemanticDanger, Icon: "mdi:alert-circle-outline", Order: 60},
			{Value: string(aiagentruntime.RunCancelled), LabelKey: "codeset.ai.agent_run_status.cancelled", Semantic: codeset.SemanticMuted, Icon: "mdi:cancel", Order: 70},
		},
	}
}

func agentStepStatusCodeSet() codeset.CodeSet {
	return codeset.CodeSet{
		Key: CodeSetAgentStepStatus, NameKey: "codeset.ai.agent_step_status", Order: 221,
		Items: []codeset.CodeItem{
			{Value: string(aiagentruntime.StepPending), LabelKey: "codeset.ai.agent_step_status.pending", Semantic: codeset.SemanticMuted, Icon: "mdi:clock-outline", Order: 10},
			{Value: string(aiagentruntime.StepRunning), LabelKey: "codeset.ai.agent_step_status.running", Semantic: codeset.SemanticInfo, Icon: "mdi:play-circle-outline", Order: 20},
			{Value: string(aiagentruntime.StepWaiting), LabelKey: "codeset.ai.agent_step_status.waiting", Semantic: codeset.SemanticWarning, Icon: "mdi:pause-circle-outline", Order: 30},
			{Value: string(aiagentruntime.StepCompleted), LabelKey: "codeset.ai.agent_step_status.completed", Semantic: codeset.SemanticSuccess, Icon: "mdi:check-circle-outline", Order: 40},
			{Value: string(aiagentruntime.StepFailed), LabelKey: "codeset.ai.agent_step_status.failed", Semantic: codeset.SemanticDanger, Icon: "mdi:alert-circle-outline", Order: 50},
			{Value: string(aiagentruntime.StepSkipped), LabelKey: "codeset.ai.agent_step_status.skipped", Semantic: codeset.SemanticMuted, Icon: "mdi:skip-next-outline", Order: 60},
			{Value: string(aiagentruntime.StepCancelled), LabelKey: "codeset.ai.agent_step_status.cancelled", Semantic: codeset.SemanticMuted, Icon: "mdi:cancel", Order: 70},
		},
	}
}
