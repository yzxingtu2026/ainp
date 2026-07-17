package ai

import (
	"context"
	"encoding/json"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/aiagentruntime"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/codeset"
	"github.com/yzxingtu2026/ainp/platform/framework/job"
	"github.com/yzxingtu2026/ainp/platform/framework/logging"
	"github.com/yzxingtu2026/ainp/platform/framework/outbox"
	"github.com/yzxingtu2026/ainp/platform/framework/statemachine"
)

const AgentRunJobType = "ai.agent_run.execute"

type agentRunJobPayload struct {
	RunID string `json:"run_id"`
	Mode  string `json:"mode,omitempty"`
}

func RegisterAgentRuntime(a *app.App, runtime *AgentRuntimeService) error {
	auditOn := true
	actions := []action.Action{
		{ID: ActionAgentRunStart, Handler: action.JSON(runtime.Start), Meta: action.Metadata{Capability: "ai", Name: "启动 Agent Run", NameKey: "action." + ActionAgentRunStart + ".name", Description: "幂等创建并排队 Agent Run", DescriptionKey: "action." + ActionAgentRunStart + ".description", PermissionGroup: "ai.agent_runtime", PermissionGroupName: "Agent Runtime", PermissionGroupNameKey: "permission_group.ai.agent_runtime", Audit: &auditOn, Idempotency: &action.IdempotencyMeta{Mode: "required"}, HTTP: &action.HTTPMeta{Method: "POST", Path: "/api/v1/ai/agent-runs/start"}}},
		{ID: ActionAgentRunGet, Handler: action.JSON(runtime.Get), Meta: runActionMeta(ActionAgentRunGet, "Agent Run 详情", "/api/v1/ai/agent-runs/get", &auditOn)},
		{ID: ActionAgentRunList, Handler: action.JSON(runtime.List), Meta: runActionMeta(ActionAgentRunList, "Agent Run 列表", "/api/v1/ai/agent-runs/list", &auditOn)},
		{ID: ActionAgentRunCancel, Handler: action.JSON(runtime.Cancel), Meta: runActionMeta(ActionAgentRunCancel, "取消 Agent Run", "/api/v1/ai/agent-runs/cancel", &auditOn)},
		{ID: ActionAgentRunResume, Handler: action.JSON(runtime.Resume), Meta: runActionMeta(ActionAgentRunResume, "恢复 Agent Run", "/api/v1/ai/agent-runs/resume", &auditOn)},
		{ID: ActionAgentRunRetry, Handler: action.JSON(runtime.Retry), Meta: runActionMeta(ActionAgentRunRetry, "重试 Agent Run", "/api/v1/ai/agent-runs/retry", &auditOn)},
		{ID: ActionAgentRunEvents, Handler: action.JSON(runtime.Events), Meta: runActionMeta(ActionAgentRunEvents, "Agent Run 事件", "/api/v1/ai/agent-runs/events", &auditOn)},
		{ID: ActionAgentRunSubscribe, Handler: action.JSON(runtime.Subscribe), Meta: runActionMeta(ActionAgentRunSubscribe, "订阅 Agent Run", "/api/v1/ai/agent-runs/subscribe", &auditOn)},
	}
	for _, item := range actions {
		if err := a.RegisterAction(item); err != nil {
			return err
		}
	}
	for _, set := range []codeset.CodeSet{agentRunStatusCodeSet(), agentStepStatusCodeSet()} {
		if err := a.RegisterCodeSet(set); err != nil {
			return err
		}
	}
	if err := a.RegisterStateMachine(statemachine.StateMachine{
		Key: StateMachineAgentRun, CodeSetKey: CodeSetAgentRunStatus,
		Terminal: []string{string(aiagentruntime.RunCompleted), string(aiagentruntime.RunCancelled)},
		Transitions: []statemachine.Transition{
			{From: string(aiagentruntime.RunQueued), To: string(aiagentruntime.RunCancelled), Action: ActionAgentRunCancel},
			{From: string(aiagentruntime.RunRunning), To: string(aiagentruntime.RunCancelled), Action: ActionAgentRunCancel},
			{From: string(aiagentruntime.RunWaitingConfirmation), To: string(aiagentruntime.RunCancelled), Action: ActionAgentRunCancel},
			{From: string(aiagentruntime.RunPaused), To: string(aiagentruntime.RunCancelled), Action: ActionAgentRunCancel},
			{From: string(aiagentruntime.RunWaitingConfirmation), To: string(aiagentruntime.RunQueued), Action: ActionAgentRunResume},
			{From: string(aiagentruntime.RunPaused), To: string(aiagentruntime.RunQueued), Action: ActionAgentRunResume},
			{From: string(aiagentruntime.RunFailed), To: string(aiagentruntime.RunQueued), Action: ActionAgentRunRetry},
		},
	}); err != nil {
		return err
	}
	if a.Outbox != nil {
		for _, def := range []outbox.Definition{{EventType: agentRunRequestedEvent, SchemaVersion: 1, Description: "Agent Run requested"}, {EventType: agentRunResumeEvent, SchemaVersion: 1, Description: "Agent Run resume requested"}, {EventType: agentRunTerminalEvent, SchemaVersion: 1, Description: "Agent Run terminal"}, {EventType: agentRunConfirmationTimeoutEvent, SchemaVersion: 1, Description: "Agent Run confirmation timeout"}} {
			if err := a.Outbox.RegisterDefinition(def); err != nil {
				return err
			}
		}
		if err := a.Outbox.RegisterHandler(outbox.HandlerFunc{HandlerName: "ai.enqueue_agent_run", Types: []string{agentRunRequestedEvent, agentRunResumeEvent, agentRunConfirmationTimeoutEvent}, Fn: func(ctx context.Context, env outbox.Envelope) error {
			if a.Jobs == nil || !a.Jobs.Enabled {
				logging.Warn(ctx, "jobs disabled; Agent Run remains queued", "run_id", env.AggregateID)
				return nil
			}
			mode := ""
			delay := time.Duration(0)
			if env.EventType == agentRunConfirmationTimeoutEvent {
				mode = "confirmation_timeout"
				var p struct {
					ExpiresAt time.Time `json:"expires_at"`
				}
				_ = json.Unmarshal(env.Payload, &p)
				delay = time.Until(p.ExpiresAt)
				if delay < 0 {
					delay = 0
				}
			}
			_, err := a.Jobs.Enqueuer.Enqueue(ctx, job.EnqueueInput{Type: AgentRunJobType, IdempotencyKey: env.ID + ":" + AgentRunJobType, CausationID: env.ID, ProcessIn: delay, Payload: agentRunJobPayload{RunID: env.AggregateID, Mode: mode}})
			return err
		}}); err != nil {
			return err
		}
	}
	if a.Jobs != nil {
		if err := a.Jobs.Register(job.Definition{Type: AgentRunJobType, SchemaVersion: 1, Queue: job.QueueDefault, MaxAttempts: 3, Timeout: 10 * time.Minute, Description: "Execute an Agent Run with lease and recovery"}, job.HandlerFunc{JobType: AgentRunJobType, Fn: func(ctx context.Context, env job.Envelope, payload json.RawMessage, reporter job.Reporter) error {
			var p agentRunJobPayload
			if err := json.Unmarshal(payload, &p); err != nil {
				return job.Permanent(err)
			}
			if p.Mode == "confirmation_timeout" {
				woke, err := runtime.WakeExpiredConfirmation(ctx, p.RunID)
				if err != nil || !woke {
					return err
				}
			}
			if err := runtime.Execute(ctx, p.RunID, env.ID); err != nil {
				return err
			}
			_ = reporter.SetResultRef(ctx, "ai_agent_run:"+p.RunID)
			return nil
		}}); err != nil {
			return err
		}
	}
	return nil
}

func runActionMeta(id, name, path string, audit *bool) action.Metadata {
	return action.Metadata{Capability: "ai", Name: name, NameKey: "action." + id + ".name", Description: name, DescriptionKey: "action." + id + ".description", PermissionGroup: "ai.agent_runtime", PermissionGroupName: "Agent Runtime", PermissionGroupNameKey: "permission_group.ai.agent_runtime", Audit: audit, HTTP: &action.HTTPMeta{Method: "POST", Path: path}}
}
