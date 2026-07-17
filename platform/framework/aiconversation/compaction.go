package aiconversation

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/yzxingtu2026/ainp/platform/framework/aimodelruntime"
)

const CompactionSchemaVersion = "1.0.0"

type Tokenizer interface {
	Count(context.Context, any) (int, error)
}

type JSONTokenizer struct{}

func (JSONTokenizer) Count(_ context.Context, value any) (int, error) {
	b, err := json.Marshal(value)
	if err != nil {
		return 0, err
	}
	return (len(b) + 3) / 4, nil
}

type CompactionSummary struct {
	Goal                 string   `json:"goal"`
	Decisions            []string `json:"decisions"`
	Constraints          []string `json:"constraints"`
	CompletedActions     []string `json:"completed_actions"`
	SideEffectResults    []string `json:"side_effect_results"`
	ActiveResources      []string `json:"active_resources"`
	UnresolvedQuestions  []string `json:"unresolved_questions"`
	PendingConfirmations []string `json:"pending_confirmations"`
	FailureState         string   `json:"failure_state"`
	SourceMessageIDs     []string `json:"source_message_ids"`
	SourceSequenceFrom   uint64   `json:"source_sequence_from"`
	SourceSequenceTo     uint64   `json:"source_sequence_to"`
}

type SummaryRequest struct {
	Messages      []Message
	MaxTokens     int
	SchemaVersion string
}
type CompactionProvider interface {
	Summarize(context.Context, SummaryRequest) (*CompactionSummary, error)
}
type DurableMemoryFlushHook interface {
	FlushBeforeCompaction(context.Context, string, uint64, uint64) error
}
type NoopMemoryFlushHook struct{}

func (NoopMemoryFlushHook) FlushBeforeCompaction(context.Context, string, uint64, uint64) error {
	return nil
}

type SnapshotPayload struct {
	SchemaVersion       string             `json:"schema_version"`
	PolicyVersion       string             `json:"policy_version"`
	PromptVersion       string             `json:"prompt_version"`
	Summary             *CompactionSummary `json:"summary,omitempty"`
	RetainedMessages    []Message          `json:"retained_messages"`
	ProtectedMessageIDs []string           `json:"protected_message_ids"`
	Reason              string             `json:"reason"`
	InputTokenEstimate  int                `json:"input_token_estimate"`
	OutputTokenEstimate int                `json:"output_token_estimate"`
	ProtectedFacts      map[string]any     `json:"protected_facts"`
}

type EngineConfig struct {
	PolicyVersion, PromptVersion string
	MaxDepth, RecentMessages     int
}
type Engine struct {
	Tokenizer Tokenizer
	Provider  CompactionProvider
	Memory    DurableMemoryFlushHook
	Config    EngineConfig
}

func (e Engine) Compact(ctx context.Context, conversationID string, assembly AssemblyResult, availableTokens, depth int) (*SnapshotPayload, string, error) {
	if e.Tokenizer == nil || availableTokens <= 0 {
		return nil, "failed", fmt.Errorf("token budget unavailable")
	}
	if e.Config.MaxDepth <= 0 {
		e.Config.MaxDepth = 3
	}
	if e.Config.RecentMessages <= 0 {
		e.Config.RecentMessages = 6
	}
	if depth >= e.Config.MaxDepth {
		return nil, "cannot_fit", nil
	}
	inputTokens, err := e.Tokenizer.Count(ctx, assembly.Messages)
	if err != nil {
		return nil, "failed", err
	}
	if inputTokens <= availableTokens {
		return &SnapshotPayload{SchemaVersion: CompactionSchemaVersion, PolicyVersion: e.Config.PolicyVersion, PromptVersion: e.Config.PromptVersion, RetainedMessages: assembly.Messages, Reason: "within_budget", InputTokenEstimate: inputTokens, OutputTokenEstimate: inputTokens}, "within_budget", nil
	}
	keep := e.Config.RecentMessages
	if keep > len(assembly.Messages) {
		keep = len(assembly.Messages)
	}
	start := toolSafeTailStart(assembly.Messages, len(assembly.Messages)-keep)
	prefix := assembly.Messages[:start]
	tail := append([]Message(nil), assembly.Messages[start:]...)
	if len(prefix) == 0 {
		return nil, "cannot_fit", nil
	}
	if e.Memory == nil {
		e.Memory = NoopMemoryFlushHook{}
	}
	if err = e.Memory.FlushBeforeCompaction(ctx, conversationID, prefix[0].Sequence, prefix[len(prefix)-1].Sequence); err != nil {
		return nil, "failed", err
	}
	if e.Provider == nil {
		return nil, "failed", fmt.Errorf("compaction provider unavailable")
	}
	summary, err := e.Provider.Summarize(ctx, SummaryRequest{Messages: projectForSummary(prefix), MaxTokens: availableTokens / 3, SchemaVersion: CompactionSchemaVersion})
	if err != nil {
		return nil, "failed", err
	}
	if err = validateSummary(summary, prefix); err != nil {
		return nil, "failed", err
	}
	protected := make([]string, 0, len(tail))
	for _, m := range tail {
		protected = append(protected, m.ID)
	}
	payload := &SnapshotPayload{SchemaVersion: CompactionSchemaVersion, PolicyVersion: e.Config.PolicyVersion, PromptVersion: e.Config.PromptVersion, Summary: summary, RetainedMessages: tail, ProtectedMessageIDs: protected, Reason: "context_budget_exceeded", InputTokenEstimate: inputTokens}
	payload.OutputTokenEstimate, err = e.Tokenizer.Count(ctx, payload)
	if err != nil {
		return nil, "failed", err
	}
	if payload.OutputTokenEstimate >= inputTokens || payload.OutputTokenEstimate > availableTokens {
		return nil, "cannot_fit", nil
	}
	return payload, "compacted", nil
}

func toolSafeTailStart(messages []Message, start int) int {
	if start <= 0 || start >= len(messages) {
		return start
	}
	needed := map[string]bool{}
	for _, part := range messages[start].Content {
		if part.Type == aimodelruntime.PartToolResult && part.ToolCallID != "" {
			needed[part.ToolCallID] = true
		}
	}
	if len(needed) == 0 {
		return start
	}
	for i := start - 1; i >= 0; i-- {
		for _, part := range messages[i].Content {
			if part.Type == aimodelruntime.PartToolCall && needed[part.ToolCallID] {
				delete(needed, part.ToolCallID)
			}
		}
		if len(needed) == 0 {
			return i
		}
	}
	return 0
}

func projectForSummary(messages []Message) []Message {
	out := make([]Message, len(messages))
	for i, message := range messages {
		out[i] = message
		out[i].Content = append([]aimodelruntime.ContentPart(nil), message.Content...)
		for j, part := range out[i].Content {
			switch part.Type {
			case aimodelruntime.PartToolCall:
				if len(part.Arguments) > 0 {
					hash := sha256.Sum256(part.Arguments)
					part.Arguments, _ = json.Marshal(map[string]any{"arguments_sha256": fmt.Sprintf("%x", hash[:]), "size": len(part.Arguments)})
				}
			case aimodelruntime.PartToolResult:
				hash := sha256.Sum256(part.Result)
				safe := map[string]any{"result_sha256": fmt.Sprintf("%x", hash[:]), "size": len(part.Result)}
				var value map[string]any
				if json.Unmarshal(part.Result, &value) == nil {
					for _, key := range []string{"status", "summary", "resource_refs", "truncated", "side_effect"} {
						if v, ok := value[key]; ok {
							safe[key] = v
						}
					}
				}
				part.Result, _ = json.Marshal(safe)
			}
			out[i].Content[j] = part
		}
	}
	return out
}

func validateSummary(s *CompactionSummary, messages []Message) error {
	if s == nil || s.Goal == "" || s.Decisions == nil || s.Constraints == nil || s.CompletedActions == nil || s.SideEffectResults == nil || s.ActiveResources == nil || s.UnresolvedQuestions == nil || s.PendingConfirmations == nil || s.SourceMessageIDs == nil {
		return fmt.Errorf("invalid compaction summary schema")
	}
	if len(messages) == 0 || s.SourceSequenceFrom != messages[0].Sequence || s.SourceSequenceTo != messages[len(messages)-1].Sequence {
		return fmt.Errorf("summary source range mismatch")
	}
	want := make([]string, len(messages))
	for i, m := range messages {
		want[i] = m.ID
	}
	got := append([]string(nil), s.SourceMessageIDs...)
	sort.Strings(want)
	sort.Strings(got)
	if len(want) != len(got) {
		return fmt.Errorf("summary identifiers mismatch")
	}
	for i := range want {
		if want[i] != got[i] {
			return fmt.Errorf("summary identifiers mismatch")
		}
	}
	return nil
}

type FakeCompactionProvider struct {
	Summary *CompactionSummary
	Err     error
}

func (f FakeCompactionProvider) Summarize(_ context.Context, r SummaryRequest) (*CompactionSummary, error) {
	if f.Err != nil {
		return nil, f.Err
	}
	if f.Summary != nil {
		return f.Summary, nil
	}
	s := &CompactionSummary{Goal: "preserve conversation state", Decisions: []string{}, Constraints: []string{}, CompletedActions: []string{}, SideEffectResults: []string{}, ActiveResources: []string{}, UnresolvedQuestions: []string{}, PendingConfirmations: []string{}, SourceMessageIDs: []string{}, SourceSequenceFrom: r.Messages[0].Sequence, SourceSequenceTo: r.Messages[len(r.Messages)-1].Sequence}
	for _, m := range r.Messages {
		s.SourceMessageIDs = append(s.SourceMessageIDs, m.ID)
	}
	return s, nil
}
