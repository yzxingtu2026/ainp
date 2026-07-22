package ai

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/aimodelruntime"
)

func TestPersonalizationAcceptsInvocationRuntimeValueResponse(t *testing.T) {
	want := aimodelruntime.GenerateResponse{ID: "response-1"}
	got, ok := asGenerateResponse(any(want))
	if !ok {
		t.Fatal("InvocationService.Generate value response must be accepted")
	}
	if got.ID != want.ID {
		t.Fatalf("unexpected response id: %q", got.ID)
	}
}

func TestProjectConversationEndingKeepsBoundedTextInSequenceOrder(t *testing.T) {
	content := func(parts ...aimodelruntime.ContentPart) []byte { value, _ := json.Marshal(parts); return value }
	records := []ConversationMessage{
		{Role: "assistant", Sequence: 4, Content: content(aimodelruntime.ContentPart{Type: aimodelruntime.PartOutputText, Text: "最后回复"}, aimodelruntime.ContentPart{Type: aimodelruntime.PartToolCall, ToolName: "secret.tool"})},
		{Role: "user", Sequence: 3, Content: content(aimodelruntime.ContentPart{Type: aimodelruntime.PartInputText, Text: strings.Repeat("待继续", 200)})},
	}
	projected := projectConversationEnding(records, 450)
	if len(projected) != 2 || projected[0].Role != "user" || projected[1].Role != "assistant" {
		t.Fatalf("unexpected projection order: %#v", projected)
	}
	if len([]rune(projected[0].Text)) != 400 {
		t.Fatalf("message limit not applied: %d", len([]rune(projected[0].Text)))
	}
	if strings.Contains(projected[1].Text, "secret.tool") {
		t.Fatal("tool metadata leaked into conversation projection")
	}
}

func TestPersonalizationValidationAllowsNaturalSlashButRejectsReferences(t *testing.T) {
	base := []PersonalizationSuggestion{{Title: "继续讨论", Prompt: "继续梳理首页/侧边栏的交互", SourceType: "recent_conversation"}}
	if err := validatePersonalization("杨总，继续上次的工作吗？", base); err != nil {
		t.Fatalf("natural-language slash should be valid: %v", err)
	}
	withURL := append([]PersonalizationSuggestion(nil), base...)
	withURL[0].Prompt = "打开 https://example.com"
	if err := validatePersonalization("继续吗？", withURL); err == nil {
		t.Fatal("URL must be rejected")
	}
	target := "menu.internal"
	withTarget := append([]PersonalizationSuggestion(nil), base...)
	withTarget[0].TargetRef = &target
	if err := validatePersonalization("继续吗？", withTarget); err == nil {
		t.Fatal("target_ref must be rejected without trusted candidates")
	}
}

func TestPersonalizationSchemaRequiresNullTargetRef(t *testing.T) {
	response := &aimodelruntime.GenerateResponse{Output: []aimodelruntime.OutputItem{{Type: aimodelruntime.PartOutputText, Text: `{"greeting":"继续吗？","suggestions":[{"title":"继续","prompt":"继续上次工作","source_type":"recent_conversation","target_ref":"menu.internal"}]}`}}}
	contract := &aimodelruntime.StructuredOutput{Name: "assistant_personalization", Strict: true, Schema: personalizationSchema()}
	if err := aimodelruntime.ValidateStructuredResponse(response, contract); err == nil {
		t.Fatal("schema must reject non-null target_ref")
	}
}

func TestRenderTimeGreetingUsesReadTimeAndStoredTimezone(t *testing.T) {
	morning := time.Date(2026, 7, 21, 23, 0, 0, 0, time.UTC) // 07:00 in Shanghai
	evening := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC) // 20:00 in Shanghai
	stored := "早上好，杨总。"
	if got := renderTimeGreeting(stored, "zh-CN", "Asia/Shanghai", morning); got != "早上好，杨总。" {
		t.Fatalf("unexpected morning greeting: %q", got)
	}
	if got := renderTimeGreeting(stored, "zh-CN", "Asia/Shanghai", evening); got != "晚上好，杨总。" {
		t.Fatalf("stale period was not replaced: %q", got)
	}
}

func TestRenderTimeGreetingFallsBackToUTCForInvalidTimezone(t *testing.T) {
	now := time.Date(2026, 7, 22, 15, 0, 0, 0, time.UTC)
	if got := renderTimeGreeting("Ready to continue?", "en-US", "invalid", now); got != "Good afternoon, Ready to continue?" {
		t.Fatalf("unexpected fallback greeting: %q", got)
	}
}

func TestSafeProfileAppliesFieldAndLengthBudgets(t *testing.T) {
	raw := []byte(`{"preferred_address":"杨老师","facts":{"project":"AINP","long":"` + strings.Repeat("中", 200) + `"},"ignored":["raw"]}`)
	profile := safeProfile(raw)
	if profile["preferred_address"] != "杨老师" {
		t.Fatalf("unexpected preferred address: %#v", profile)
	}
	facts, ok := profile["facts"].(map[string]any)
	if !ok || facts["project"] != "AINP" {
		t.Fatalf("unexpected facts projection: %#v", profile)
	}
	if len([]rune(facts["long"].(string))) != 160 {
		t.Fatal("nested profile value was not truncated")
	}
	if _, exists := profile["ignored"]; exists {
		t.Fatal("unsupported raw profile values must not be projected")
	}
}
