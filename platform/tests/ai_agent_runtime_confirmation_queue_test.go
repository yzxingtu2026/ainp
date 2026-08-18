package tests

import (
	"encoding/json"
	"testing"

	"github.com/yzxingtu2026/ainp/platform/framework/aiconversation"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodelruntime"
	conversation "github.com/yzxingtu2026/ainp/platform/modules/ai/conversation"
)

func confirmationQueueMessage(role string, parts ...aimodelruntime.ContentPart) conversation.ConversationMessage {
	content, _ := json.Marshal(parts)
	return conversation.ConversationMessage{Role: role, Content: content}
}

func confirmationQueueCall(callID string) aimodelruntime.ContentPart {
	return aimodelruntime.ContentPart{Type: aimodelruntime.PartToolCall, ToolCallID: callID, ToolName: "action__demo_write", Arguments: json.RawMessage(`{"id":"` + callID + `"}`)}
}

func confirmationQueueResult(callID, status, category string) aimodelruntime.ContentPart {
	result, _ := json.Marshal(map[string]any{"call_id": callID, "status": status, "error_category": category})
	return aimodelruntime.ContentPart{Type: aimodelruntime.PartToolResult, ToolCallID: callID, Result: result}
}

func TestBuildDeferredConfirmationQueueAdvancesSequentially(t *testing.T) {
	runID := "run-a"
	retryB := conversation.DeferredConfirmationRetryCallID(runID, "call-b")
	retryC := conversation.DeferredConfirmationRetryCallID(runID, "call-c")
	messages := []conversation.ConversationMessage{
		confirmationQueueMessage("assistant", confirmationQueueCall("call-a"), confirmationQueueCall("call-b"), confirmationQueueCall("call-c")),
		confirmationQueueMessage("tool", confirmationQueueResult("call-b", "failed", "deferred_for_confirmation")),
		confirmationQueueMessage("tool", confirmationQueueResult("call-c", "failed", "deferred_for_confirmation")),
		confirmationQueueMessage("tool", confirmationQueueResult("call-a", "completed", "")),
	}
	queue := conversation.BuildDeferredConfirmationQueue(runID, messages)
	if queue.Blocked || queue.Next == nil || queue.Next.OriginalCallID != "call-b" || queue.Next.RetryCallID != retryB || queue.Next.RetryPersisted || queue.ExecutedCalls != 1 {
		t.Fatalf("unexpected first queue item: %#v", queue)
	}
	messages = append(messages,
		confirmationQueueMessage("assistant", aimodelruntime.ContentPart{Type: aimodelruntime.PartToolCall, ToolCallID: retryB, RetryOfToolCallID: "call-b", ToolName: "action__demo_write", Arguments: json.RawMessage(`{"id":"call-b"}`)}),
		confirmationQueueMessage("tool", confirmationQueueResult(retryB, "completed", "")),
	)
	queue = conversation.BuildDeferredConfirmationQueue(runID, messages)
	if queue.Blocked || queue.Next == nil || queue.Next.OriginalCallID != "call-c" || queue.Next.RetryCallID != retryC || queue.ExecutedCalls != 2 {
		t.Fatalf("unexpected second queue item: %#v", queue)
	}
	messages = append(messages,
		confirmationQueueMessage("assistant", aimodelruntime.ContentPart{Type: aimodelruntime.PartToolCall, ToolCallID: retryC, RetryOfToolCallID: "call-c", ToolName: "action__demo_write", Arguments: json.RawMessage(`{"id":"call-c"}`)}),
		confirmationQueueMessage("tool", confirmationQueueResult(retryC, "completed", "")),
	)
	queue = conversation.BuildDeferredConfirmationQueue(runID, messages)
	if queue.Blocked || queue.Next != nil || queue.ExecutedCalls != 3 {
		t.Fatalf("completed queue must be empty: %#v", queue)
	}
}

func TestBuildDeferredConfirmationQueueFailsClosed(t *testing.T) {
	runID := "run-a"
	retry := conversation.DeferredConfirmationRetryCallID(runID, "call-b")
	base := []conversation.ConversationMessage{
		confirmationQueueMessage("assistant", confirmationQueueCall("call-a"), confirmationQueueCall("call-b")),
		confirmationQueueMessage("tool", confirmationQueueResult("call-b", "failed", "deferred_for_confirmation")),
		confirmationQueueMessage("tool", confirmationQueueResult("call-a", "completed", "")),
	}
	for _, test := range []struct {
		name     string
		messages []conversation.ConversationMessage
	}{
		{name: "ambiguous lineage", messages: append(append([]conversation.ConversationMessage{}, base...),
			confirmationQueueMessage("assistant",
				aimodelruntime.ContentPart{Type: aimodelruntime.PartToolCall, ToolCallID: retry, RetryOfToolCallID: "call-b", ToolName: "action__demo_write", Arguments: json.RawMessage(`{"id":"call-b"}`)},
				aimodelruntime.ContentPart{Type: aimodelruntime.PartToolCall, ToolCallID: "other-retry", RetryOfToolCallID: "call-b", ToolName: "action__demo_write", Arguments: json.RawMessage(`{"id":"call-b"}`)}))},
		{name: "lineage mismatch", messages: append(append([]conversation.ConversationMessage{}, base...),
			confirmationQueueMessage("assistant", aimodelruntime.ContentPart{Type: aimodelruntime.PartToolCall, ToolCallID: retry, RetryOfToolCallID: "call-a", ToolName: "action__demo_write", Arguments: json.RawMessage(`{"id":"call-b"}`)}))},
		{name: "orphan lineage", messages: append(append([]conversation.ConversationMessage{}, base...),
			confirmationQueueMessage("assistant", aimodelruntime.ContentPart{Type: aimodelruntime.PartToolCall, ToolCallID: "orphan-retry", RetryOfToolCallID: "missing-call", ToolName: "action__demo_write", Arguments: json.RawMessage(`{}`)}))},
		{name: "invalid result", messages: append(append([]conversation.ConversationMessage{}, base...),
			confirmationQueueMessage("tool", aimodelruntime.ContentPart{Type: aimodelruntime.PartToolResult, ToolCallID: retry, Result: json.RawMessage(`{`)}))},
	} {
		t.Run(test.name, func(t *testing.T) {
			queue := conversation.BuildDeferredConfirmationQueue(runID, test.messages)
			if !queue.Blocked || queue.Next != nil {
				t.Fatalf("unsafe queue must be blocked: %#v", queue)
			}
		})
	}
}

func TestBuildDeferredConfirmationQueueStopsAfterExecutionFailure(t *testing.T) {
	runID := "run-a"
	retry := conversation.DeferredConfirmationRetryCallID(runID, "call-b")
	for _, messages := range [][]conversation.ConversationMessage{
		{
			confirmationQueueMessage("assistant", confirmationQueueCall("call-a"), confirmationQueueCall("call-b")),
			confirmationQueueMessage("tool", confirmationQueueResult("call-a", "failed", "conflict")),
			confirmationQueueMessage("tool", confirmationQueueResult("call-b", "failed", "deferred_for_confirmation")),
		},
		{
			confirmationQueueMessage("assistant", confirmationQueueCall("call-a"), confirmationQueueCall("call-b")),
			confirmationQueueMessage("tool", confirmationQueueResult("call-a", "completed", "")),
			confirmationQueueMessage("tool", confirmationQueueResult("call-b", "failed", "deferred_for_confirmation")),
			confirmationQueueMessage("assistant", aimodelruntime.ContentPart{Type: aimodelruntime.PartToolCall, ToolCallID: retry, RetryOfToolCallID: "call-b", ToolName: "action__demo_write", Arguments: json.RawMessage(`{"id":"call-b"}`)}),
			confirmationQueueMessage("tool", confirmationQueueResult(retry, "failed", "conflict")),
		},
	} {
		queue := conversation.BuildDeferredConfirmationQueue(runID, messages)
		if queue.Blocked || queue.Next != nil {
			t.Fatalf("known execution failure must stop without structural failure: %#v", queue)
		}
	}
}

func TestBuildDeferredConfirmationQueueSkipsTerminatedHistoricalBatch(t *testing.T) {
	runID := "run-a"
	messages := []conversation.ConversationMessage{
		confirmationQueueMessage("assistant", confirmationQueueCall("old-a"), confirmationQueueCall("old-b")),
		confirmationQueueMessage("tool", confirmationQueueResult("old-a", "failed", "conflict")),
		confirmationQueueMessage("tool", confirmationQueueResult("old-b", "failed", "deferred_for_confirmation")),
		confirmationQueueMessage("assistant", confirmationQueueCall("new-a"), confirmationQueueCall("new-b")),
		confirmationQueueMessage("tool", confirmationQueueResult("new-a", "completed", "")),
		confirmationQueueMessage("tool", confirmationQueueResult("new-b", "failed", "deferred_for_confirmation")),
	}
	queue := conversation.BuildDeferredConfirmationQueue(runID, messages)
	if queue.Blocked || queue.Next == nil || queue.Next.OriginalCallID != "new-b" {
		t.Fatalf("current queue was hidden by terminated history: %#v", queue)
	}
}

func TestDeferredConfirmationRetryCallIDIsStableAndDistinct(t *testing.T) {
	first := conversation.DeferredConfirmationRetryCallID("run-a", "call-a")
	if first != conversation.DeferredConfirmationRetryCallID("run-a", "call-a") {
		t.Fatal("retry call id is not stable")
	}
	if first == conversation.DeferredConfirmationRetryCallID("run-a", "call-b") || first == conversation.DeferredConfirmationRetryCallID("run-b", "call-a") {
		t.Fatal("retry call id does not isolate run and original call")
	}
	if len(first) > 128 {
		t.Fatalf("retry call id too long: %d", len(first))
	}
}

func TestProjectToolCallRetriesPreservesOneBusinessOperation(t *testing.T) {
	messages := []aiconversation.Message{
		{Role: aimodelruntime.RoleAssistant, Content: []aimodelruntime.ContentPart{confirmationQueueCall("call-b")}},
		{Role: aimodelruntime.RoleTool, Content: []aimodelruntime.ContentPart{confirmationQueueResult("call-b", "failed", "deferred_for_confirmation")}},
		{Role: aimodelruntime.RoleAssistant, Content: []aimodelruntime.ContentPart{{Type: aimodelruntime.PartToolCall, ToolCallID: "retry-b", RetryOfToolCallID: "call-b", ToolName: "action__demo_write", Arguments: json.RawMessage(`{"id":"call-b"}`)}}},
		{Role: aimodelruntime.RoleTool, Content: []aimodelruntime.ContentPart{confirmationQueueResult("retry-b", "completed", "")}},
	}
	projected := aiconversation.ProjectToolCallRetries(messages)
	if len(projected) != 2 || projected[1].Content[0].ToolCallID != "call-b" {
		t.Fatalf("unexpected projection: %#v", projected)
	}
	var result map[string]any
	if err := json.Unmarshal(projected[1].Content[0].Result, &result); err != nil || result["call_id"] != "call-b" {
		t.Fatalf("retry result was not normalized: %#v %v", result, err)
	}
	if messages[2].Content[0].RetryOfToolCallID != "call-b" {
		t.Fatal("canonical messages were mutated")
	}
}

func TestProjectToolCallRetriesKeepsUnsafeRelationsCanonical(t *testing.T) {
	for _, messages := range [][]aiconversation.Message{
		{
			{Role: aimodelruntime.RoleAssistant, Content: []aimodelruntime.ContentPart{confirmationQueueCall("call-b")}},
			{Role: aimodelruntime.RoleTool, Content: []aimodelruntime.ContentPart{confirmationQueueResult("call-b", "completed", "")}},
			{Role: aimodelruntime.RoleAssistant, Content: []aimodelruntime.ContentPart{{Type: aimodelruntime.PartToolCall, ToolCallID: "retry-b", RetryOfToolCallID: "call-b", ToolName: "action__demo_write"}}},
			{Role: aimodelruntime.RoleTool, Content: []aimodelruntime.ContentPart{confirmationQueueResult("retry-b", "completed", "")}},
		},
		{
			{Role: aimodelruntime.RoleAssistant, Content: []aimodelruntime.ContentPart{confirmationQueueCall("call-b")}},
			{Role: aimodelruntime.RoleTool, Content: []aimodelruntime.ContentPart{confirmationQueueResult("call-b", "failed", "deferred_for_confirmation")}},
			{Role: aimodelruntime.RoleAssistant, Content: []aimodelruntime.ContentPart{
				{Type: aimodelruntime.PartToolCall, ToolCallID: "retry-1", RetryOfToolCallID: "call-b", ToolName: "action__demo_write"},
				{Type: aimodelruntime.PartToolCall, ToolCallID: "retry-2", RetryOfToolCallID: "call-b", ToolName: "action__demo_write"},
			}},
		},
		{
			{Role: aimodelruntime.RoleAssistant, Content: []aimodelruntime.ContentPart{confirmationQueueCall("call-b")}},
			{Role: aimodelruntime.RoleTool, Content: []aimodelruntime.ContentPart{confirmationQueueResult("call-b", "failed", "deferred_for_confirmation")}},
			{Role: aimodelruntime.RoleAssistant, Content: []aimodelruntime.ContentPart{{Type: aimodelruntime.PartToolCall, ToolCallID: "retry-b", RetryOfToolCallID: "call-b", ToolName: "action__demo_write", Arguments: json.RawMessage(`{"id":"different"}`)}}},
			{Role: aimodelruntime.RoleTool, Content: []aimodelruntime.ContentPart{confirmationQueueResult("retry-b", "completed", "")}},
		},
	} {
		projected := aiconversation.ProjectToolCallRetries(messages)
		if len(projected) != len(messages) {
			t.Fatalf("unsafe relation was collapsed: %#v", projected)
		}
	}
}
