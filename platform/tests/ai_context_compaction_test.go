package tests

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/yzxingtu2026/ainp/platform/framework/aiconversation"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodelruntime"
)

type captureSummaryProvider struct {
	request *aiconversation.SummaryRequest
}

func (p *captureSummaryProvider) Summarize(_ context.Context, r aiconversation.SummaryRequest) (*aiconversation.CompactionSummary, error) {
	p.request = &r
	return aiconversation.FakeCompactionProvider{}.Summarize(context.Background(), r)
}

func compactMessages(count int, text string) []aiconversation.Message {
	rows := make([]aiconversation.Message, 0, count)
	for i := 0; i < count; i++ {
		rows = append(rows, aiconversation.Message{ID: string(rune('a' + i)), Sequence: uint64(i + 1), Role: aimodelruntime.RoleUser, Content: []aimodelruntime.ContentPart{{Type: aimodelruntime.PartInputText, Text: text}}})
	}
	return rows
}

func TestCompactionPreservesRecentMessageBoundaries(t *testing.T) {
	rows := compactMessages(9, strings.Repeat("x", 2000))
	assembly := aiconversation.AssemblyResult{Messages: rows, SourceSequenceFrom: 1, SourceSequenceTo: 9, ConversationRevision: 10, AgentVersionID: "v1"}
	engine := aiconversation.Engine{Tokenizer: aiconversation.JSONTokenizer{}, Provider: aiconversation.FakeCompactionProvider{}, Memory: aiconversation.NoopMemoryFlushHook{}, Config: aiconversation.EngineConfig{PolicyVersion: "p1", PromptVersion: "s1", RecentMessages: 3, MaxDepth: 3}}
	payload, status, err := engine.Compact(context.Background(), "c1", assembly, 2000, 0)
	if err != nil {
		t.Fatal(err)
	}
	if status != "compacted" {
		t.Fatalf("status=%s", status)
	}
	if len(payload.RetainedMessages) != 3 || payload.RetainedMessages[0].ID != "g" || payload.RetainedMessages[2].ID != "i" {
		t.Fatalf("tail changed: %#v", payload.RetainedMessages)
	}
	if payload.Summary.SourceSequenceFrom != 1 || payload.Summary.SourceSequenceTo != 6 {
		t.Fatalf("summary range=%d..%d", payload.Summary.SourceSequenceFrom, payload.Summary.SourceSequenceTo)
	}
}

func TestCompactionRejectsIdentifierDrift(t *testing.T) {
	rows := compactMessages(3, strings.Repeat("x", 4000))
	bad := &aiconversation.CompactionSummary{Goal: "x", Decisions: []string{}, Constraints: []string{}, CompletedActions: []string{}, SideEffectResults: []string{}, ActiveResources: []string{}, UnresolvedQuestions: []string{}, PendingConfirmations: []string{}, SourceMessageIDs: []string{"invented", "b"}, SourceSequenceFrom: 1, SourceSequenceTo: 2}
	engine := aiconversation.Engine{Tokenizer: aiconversation.JSONTokenizer{}, Provider: aiconversation.FakeCompactionProvider{Summary: bad}, Config: aiconversation.EngineConfig{RecentMessages: 1}}
	_, status, err := engine.Compact(context.Background(), "c1", aiconversation.AssemblyResult{Messages: rows}, 1000, 0)
	if err == nil || status != "failed" {
		t.Fatalf("status=%s err=%v", status, err)
	}
}

func TestCompactionDepthAndProviderFailuresAreStable(t *testing.T) {
	rows := compactMessages(4, strings.Repeat("x", 3000))
	engine := aiconversation.Engine{Tokenizer: aiconversation.JSONTokenizer{}, Provider: aiconversation.FakeCompactionProvider{Err: errors.New("timeout")}, Config: aiconversation.EngineConfig{RecentMessages: 1, MaxDepth: 2}}
	if _, status, err := engine.Compact(context.Background(), "c1", aiconversation.AssemblyResult{Messages: rows}, 1000, 2); status != "cannot_fit" || err != nil {
		t.Fatalf("depth status=%s err=%v", status, err)
	}
	if _, status, err := engine.Compact(context.Background(), "c1", aiconversation.AssemblyResult{Messages: rows}, 1000, 0); status != "failed" || err == nil {
		t.Fatalf("provider status=%s err=%v", status, err)
	}
}

func TestCompactionReturnsWithinBudgetWithoutProvider(t *testing.T) {
	rows := compactMessages(1, "short")
	engine := aiconversation.Engine{Tokenizer: aiconversation.JSONTokenizer{}}
	payload, status, err := engine.Compact(context.Background(), "c1", aiconversation.AssemblyResult{Messages: rows}, 1000, 0)
	if err != nil || status != "within_budget" || len(payload.RetainedMessages) != 1 {
		t.Fatalf("status=%s payload=%#v err=%v", status, payload, err)
	}
}

func TestCompactionProjectsSensitiveToolPayloads(t *testing.T) {
	secret := `{"api_key":"secret-value","query":"` + strings.Repeat("private", 3000) + `"}`
	result := `{"status":"completed","summary":"created record","resource_refs":["record:1"],"raw":"secret-result"}`
	rows := []aiconversation.Message{
		{ID: "call", Sequence: 1, Role: aimodelruntime.RoleAssistant, Content: []aimodelruntime.ContentPart{{Type: aimodelruntime.PartToolCall, ToolCallID: "tc1", ToolName: "records.create", Arguments: json.RawMessage(secret)}}},
		{ID: "result", Sequence: 2, Role: aimodelruntime.RoleTool, Content: []aimodelruntime.ContentPart{{Type: aimodelruntime.PartToolResult, ToolCallID: "tc1", Result: json.RawMessage(result)}}},
		{ID: "tail", Sequence: 3, Role: aimodelruntime.RoleUser, Content: []aimodelruntime.ContentPart{{Type: aimodelruntime.PartInputText, Text: strings.Repeat("x", 8000)}}},
	}
	provider := &captureSummaryProvider{}
	engine := aiconversation.Engine{Tokenizer: aiconversation.JSONTokenizer{}, Provider: provider, Config: aiconversation.EngineConfig{RecentMessages: 1}}
	_, status, err := engine.Compact(context.Background(), "c1", aiconversation.AssemblyResult{Messages: rows}, 3000, 0)
	if err != nil || status != "compacted" {
		t.Fatalf("status=%s err=%v", status, err)
	}
	encoded, _ := json.Marshal(provider.request.Messages)
	text := string(encoded)
	if strings.Contains(text, "secret-value") || strings.Contains(text, "secret-result") {
		t.Fatalf("sensitive tool payload reached provider: %s", text)
	}
	if !strings.Contains(text, "created record") || !strings.Contains(text, "record:1") {
		t.Fatalf("safe tool facts missing: %s", text)
	}
}

func TestContextCompactionMigrationIsVersioned(t *testing.T) {
	body, err := os.ReadFile("../migrations/0020_ai_context_compaction.sql")
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, expected := range []string{"'compacted'", "schema_version", "prompt_version", "summarizer_deployment_id", "compaction_depth", "quality_status"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("migration missing %s", expected)
		}
	}
}

func TestCompactionNeverSplitsToolCallAndResult(t *testing.T) {
	rows := compactMessages(4, strings.Repeat("x", 3000))
	rows[2] = aiconversation.Message{ID: "call", Sequence: 3, Role: aimodelruntime.RoleAssistant, Content: []aimodelruntime.ContentPart{{Type: aimodelruntime.PartToolCall, ToolCallID: "tc1", ToolName: "demo", Arguments: json.RawMessage(`{}`)}}}
	rows[3] = aiconversation.Message{ID: "result", Sequence: 4, Role: aimodelruntime.RoleTool, Content: []aimodelruntime.ContentPart{{Type: aimodelruntime.PartToolResult, ToolCallID: "tc1", Result: json.RawMessage(`{"status":"completed"}`)}}}
	engine := aiconversation.Engine{Tokenizer: aiconversation.JSONTokenizer{}, Provider: aiconversation.FakeCompactionProvider{}, Config: aiconversation.EngineConfig{RecentMessages: 1}}
	payload, status, err := engine.Compact(context.Background(), "c1", aiconversation.AssemblyResult{Messages: rows}, 1200, 0)
	if err != nil || status != "compacted" {
		t.Fatalf("status=%s err=%v", status, err)
	}
	if len(payload.RetainedMessages) != 2 || payload.RetainedMessages[0].ID != "call" || payload.RetainedMessages[1].ID != "result" {
		t.Fatalf("tool interaction split: %#v", payload.RetainedMessages)
	}
}

func TestConversationCompactionKeepsDocumentBodiesOutOfSnapshots(t *testing.T) {
	body, err := os.ReadFile("../modules/ai/conversation/compaction_service.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, expected := range []string{
		"retainsAllDocumentDerivations(assembly.Messages, payload.RetainedMessages)",
		"Reason: \"attachments_exceed_model_window\"",
		"payload.RetainedMessages = canonicalizeDocumentDerivations(payload.RetainedMessages)",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("document compaction safety gate missing %q", expected)
		}
	}
	serviceBody, err := os.ReadFile("../modules/ai/conversation/service.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"Type: aimodelruntime.PartInputFile", "FileAttachmentID: part.DerivedSource.AttachmentID", "AttachmentRevision: part.DerivedSource.Revision"} {
		if !strings.Contains(string(serviceBody), expected) {
			t.Fatalf("document snapshot canonicalization missing %q", expected)
		}
	}
}
