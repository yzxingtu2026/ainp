package aiconversation

import (
	"context"
	"encoding/json"
	"reflect"

	"github.com/yzxingtu2026/ainp/platform/framework/aimodelruntime"
)

type projectedToolResult struct {
	CallID        string `json:"call_id"`
	ErrorCategory string `json:"error_category"`
}

func ProjectToolCallRetries(messages []Message) []Message {
	calls := make(map[string]aimodelruntime.ContentPart)
	callCounts := make(map[string]int)
	retryTargets := make(map[string]string)
	retryCounts := make(map[string]int)
	results := make(map[string][]json.RawMessage)
	for _, message := range messages {
		for _, part := range message.Content {
			if part.Type == aimodelruntime.PartToolCall && part.ToolCallID != "" {
				calls[part.ToolCallID] = part
				callCounts[part.ToolCallID]++
				if part.RetryOfToolCallID != "" {
					retryTargets[part.ToolCallID] = part.RetryOfToolCallID
					retryCounts[part.RetryOfToolCallID]++
				}
			}
			if part.Type == aimodelruntime.PartToolResult && part.ToolCallID != "" {
				results[part.ToolCallID] = append(results[part.ToolCallID], part.Result)
			}
		}
	}

	finalResults := make(map[string]json.RawMessage)
	collapsedRetries := make(map[string]bool)
	for retryCallID, originalCallID := range retryTargets {
		retryCall, retryExists := calls[retryCallID]
		originalCall, originalExists := calls[originalCallID]
		if retryCounts[originalCallID] != 1 || callCounts[retryCallID] != 1 || callCounts[originalCallID] != 1 || !retryExists || !originalExists || originalCall.RetryOfToolCallID != "" || retryCall.ToolName != originalCall.ToolName || !sameToolArguments(retryCall.Arguments, originalCall.Arguments) {
			continue
		}
		retryResults := results[retryCallID]
		originalResults := results[originalCallID]
		if len(retryResults) != 1 || len(originalResults) != 1 || !deferredToolResult(originalResults[0]) {
			continue
		}
		projected, ok := projectRetryResult(retryResults[0], originalCallID)
		if !ok {
			continue
		}
		finalResults[originalCallID] = projected
		collapsedRetries[retryCallID] = true
	}
	if len(collapsedRetries) == 0 {
		return messages
	}

	out := make([]Message, 0, len(messages))
	for _, message := range messages {
		projected := message
		projected.Content = make([]aimodelruntime.ContentPart, 0, len(message.Content))
		for _, part := range message.Content {
			if part.Type == aimodelruntime.PartToolCall && collapsedRetries[part.ToolCallID] {
				continue
			}
			if part.Type == aimodelruntime.PartToolResult && collapsedRetries[part.ToolCallID] {
				continue
			}
			if part.Type == aimodelruntime.PartToolResult {
				if result, ok := finalResults[part.ToolCallID]; ok {
					part.Result = result
				}
			}
			projected.Content = append(projected.Content, part)
		}
		if len(projected.Content) > 0 {
			out = append(out, projected)
		}
	}
	return out
}

func deferredToolResult(raw json.RawMessage) bool {
	var result projectedToolResult
	return json.Unmarshal(raw, &result) == nil && result.ErrorCategory == "deferred_for_confirmation"
}

func projectRetryResult(raw json.RawMessage, originalCallID string) (json.RawMessage, bool) {
	var value map[string]any
	if json.Unmarshal(raw, &value) != nil {
		return nil, false
	}
	value["call_id"] = originalCallID
	projected, err := json.Marshal(value)
	if err != nil {
		return nil, false
	}
	return projected, true
}

func sameToolArguments(left, right json.RawMessage) bool {
	var leftValue, rightValue any
	return json.Unmarshal(left, &leftValue) == nil && json.Unmarshal(right, &rightValue) == nil && reflect.DeepEqual(leftValue, rightValue)
}

type Budget struct {
	MaxTokens       int `json:"max_tokens"`
	MaxBytes        int `json:"max_bytes"`
	EstimatedTokens int `json:"estimated_tokens"`
	UsedBytes       int `json:"used_bytes"`
}

type Message struct {
	ID       string                       `json:"id"`
	Sequence uint64                       `json:"sequence"`
	Role     aimodelruntime.Role          `json:"role"`
	Content  []aimodelruntime.ContentPart `json:"content"`
}

type AssemblyRequest struct {
	ConversationID string
	MaxTokens      int
	MaxBytes       int
}

type AssemblyResult struct {
	Status               string    `json:"status"`
	Messages             []Message `json:"messages"`
	Budget               Budget    `json:"budget"`
	SourceSequenceFrom   uint64    `json:"source_sequence_from"`
	SourceSequenceTo     uint64    `json:"source_sequence_to"`
	ConversationRevision uint64    `json:"conversation_revision"`
	AgentVersionID       string    `json:"agent_version_id"`
	Reason               string    `json:"reason,omitempty"`
}

type ContextAssembler interface {
	Assemble(context.Context, AssemblyRequest) (*AssemblyResult, error)
}
type ContextCompactor interface {
	Compact(context.Context, CompactionRequest) (*CompactionResult, error)
}

type CompactionRequest struct {
	ConversationID       string  `json:"conversation_id"`
	ModelAlias           string  `json:"model_alias"`
	ReservedOutputTokens int     `json:"reserved_output_tokens"`
	SafetyMarginTokens   int     `json:"safety_margin_tokens"`
	ToolSchemaBytes      int     `json:"tool_schema_bytes"`
	DisableTools         bool    `json:"disable_tools,omitempty"`
	AttachmentTokens     int     `json:"attachment_tokens"`
	TargetInputTokens    int     `json:"target_input_tokens,omitempty"`
	Reason               string  `json:"reason,omitempty"`
	AgentRunID           string  `json:"agent_run_id,omitempty"`
	IdempotencyKey       string  `json:"idempotency_key"`
	ParentSnapshotID     *string `json:"parent_snapshot_id,omitempty"`
}

type CompactionResult struct {
	Status            string `json:"status"`
	SnapshotID        string `json:"snapshot_id,omitempty"`
	Reason            string `json:"reason,omitempty"`
	InputTokens       int    `json:"input_tokens"`
	OutputTokens      int    `json:"output_tokens"`
	RetainedFrom      uint64 `json:"retained_from,omitempty"`
	RetainedTo        uint64 `json:"retained_to,omitempty"`
	CompactionDepth   int    `json:"compaction_depth"`
	UsageInputTokens  int64  `json:"usage_input_tokens,omitempty"`
	UsageOutputTokens int64  `json:"usage_output_tokens,omitempty"`
}

type StreamDelta struct {
	Cursor uint64 `json:"cursor"`
	Type   string `json:"type"`
	Data   []byte `json:"data"`
}

// AssistantStreamStore is the internal replay contract. Commit must publish one
// canonical assistant message for a stream, even after reconnect or retry.
type AssistantStreamStore interface {
	Begin(context.Context, string, string) (streamID string, err error)
	Append(context.Context, string, StreamDelta) error
	Replay(context.Context, string, uint64) ([]StreamDelta, error)
	Commit(context.Context, string, []aimodelruntime.ContentPart) (messageID string, err error)
}
