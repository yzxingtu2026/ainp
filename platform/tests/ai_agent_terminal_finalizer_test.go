package tests

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/yzxingtu2026/ainp/platform/framework/aiagentruntime"
)

func TestTerminalDecisionMatrixIsDeterministicAndFailClosed(t *testing.T) {
	tests := []struct {
		name      string
		input     aiagentruntime.TerminalDecisionInput
		mode      aiagentruntime.FinalizationMode
		status    aiagentruntime.RunStatus
		retryable bool
	}{
		{"safe tool failure", aiagentruntime.TerminalDecisionInput{Status: aiagentruntime.RunFailed, ErrorCategory: "tool_failed", CanUseModel: true}, aiagentruntime.FinalizationModel, aiagentruntime.RunFailed, true},
		{"provider timeout", aiagentruntime.TerminalDecisionInput{Status: aiagentruntime.RunFailed, ErrorCategory: "timeout", CanUseModel: true}, aiagentruntime.FinalizationTemplate, aiagentruntime.RunFailed, true},
		{"model unavailable", aiagentruntime.TerminalDecisionInput{Status: aiagentruntime.RunFailed, ErrorCategory: "model_not_found", CanUseModel: true}, aiagentruntime.FinalizationTemplate, aiagentruntime.RunFailed, false},
		{"authorization change", aiagentruntime.TerminalDecisionInput{Status: aiagentruntime.RunFailed, ErrorCategory: "authorization_changed", CanUseModel: true}, aiagentruntime.FinalizationTemplate, aiagentruntime.RunFailed, false},
		{"cancelled", aiagentruntime.TerminalDecisionInput{Status: aiagentruntime.RunCancelled, ErrorCategory: "cancelled", CanUseModel: true}, aiagentruntime.FinalizationNoticeOnly, aiagentruntime.RunCancelled, false},
		{"refusal text", aiagentruntime.TerminalDecisionInput{Status: aiagentruntime.RunFailed, ErrorCategory: "content_filtered", HasSafeRefusalText: true, CanUseModel: true}, aiagentruntime.FinalizationNone, aiagentruntime.RunCompleted, false},
		{"unknown", aiagentruntime.TerminalDecisionInput{Status: aiagentruntime.RunFailed, ErrorCategory: "future_internal_error", CanUseModel: true}, aiagentruntime.FinalizationTemplate, aiagentruntime.RunFailed, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			first := aiagentruntime.DecideTerminal(tt.input)
			second := aiagentruntime.DecideTerminal(tt.input)
			if first != second || first.Mode != tt.mode || first.Status != tt.status || first.Retryable != tt.retryable {
				t.Fatalf("unexpected decision: %#v / %#v", first, second)
			}
			if first.ProtocolVersion != aiagentruntime.TerminalProtocolVersion || first.UserMessageKey == "" || first.SafeSummary == "" {
				t.Fatalf("terminal contract is incomplete: %#v", first)
			}
		})
	}
}

func TestAgentRunTerminalFinalizationMigrationContract(t *testing.T) {
	body, err := os.ReadFile("../migrations/0065_agent_run_terminal_finalization.sql")
	if err != nil {
		t.Fatal(err)
	}
	source := string(body)
	for _, required := range []string{
		"terminal_protocol_version", "terminal_reason", "finalization_mode", "finalization_attempted",
		"retryable", "user_message_key", "safe_args", "safe_summary", "terminal_message_id",
		"completed_operation_count", "failed_operation_count", "unfinished_operation_count",
		"CHECK (finalization_mode IN ('model', 'template', 'notice_only', 'none'))",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("terminal migration missing %q", required)
		}
	}
}

func TestAgentRunSafeTerminalRequestContract(t *testing.T) {
	body, err := os.ReadFile("../modules/ai/conversation/runtime_service.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(body)
	for _, required := range []string{
		"func safeTerminalRequest(",
		"EnableTools:  false",
		`ToolChoice:   &aimodelruntime.ToolChoice{Mode: "none"}`,
		"s.invocations.Generate(ctx, request)",
		"finalization_attempted",
		"hasToolCall || hasRefusal || response.FinishReason == aimodelruntime.FinishContentFilter",
		`"run-terminal-final:"+run.ID`,
		"recoverTerminalFinalization",
		"recoverRefusalFinalization",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("safe terminal runtime contract missing %q", required)
		}
	}
	if strings.Contains(source, "safeTerminalRequest(modelAlias, run.ID, envelope).EnableTools = true") {
		t.Fatal("safe terminal request re-enabled tools")
	}
}

func TestFailureEnvelopeOnlyContainsWhitelistedSafeFacts(t *testing.T) {
	decision := aiagentruntime.DecideTerminal(aiagentruntime.TerminalDecisionInput{Status: aiagentruntime.RunFailed, ErrorCategory: "tool_failed", CanUseModel: true})
	envelope := aiagentruntime.BuildFailureEnvelope(decision, aiagentruntime.TerminalOperationSummary{Completed: 1000, Failed: -1, Unfinished: 2}, "zh-cn")
	data, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	serialized := string(data)
	for _, forbidden := range []string{"sql", "stack", "secret", "token", "handle", "attachment", "message", "provider_response", "tool_arguments"} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("failure envelope leaked forbidden field %q: %s", forbidden, serialized)
		}
	}
	if envelope.CompletedOperations != 100 || envelope.FailedOperations != 0 || envelope.UnfinishedOperations != 2 || envelope.Locale != "zh-CN" || envelope.SchemaVersion != "v1" {
		t.Fatalf("failure envelope bounds/version mismatch: %#v", envelope)
	}
}
