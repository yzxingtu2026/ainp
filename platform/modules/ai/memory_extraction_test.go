package ai

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/yzxingtu2026/ainp/platform/framework/aimemory"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodelruntime"
)

func TestMemoryEligibilityIsStructuralOnly(t *testing.T) {
	content, _ := json.Marshal([]aimodelruntime.ContentPart{{Type: aimodelruntime.PartInputText, Text: "好的，谢谢"}})
	text, ok := eligibleText(ConversationMessage{Content: content}, aimodelruntime.PartInputText)
	if !ok || text != "好的，谢谢" {
		t.Fatalf("ordinary text must remain eligible: %q %v", text, ok)
	}
	thirdParty, _ := json.Marshal([]aimodelruntime.ContentPart{{Type: aimodelruntime.PartDerivedText, Text: "web", DerivedSource: &aimodelruntime.DerivedTextSource{Untrusted: true}}})
	if _, ok = eligibleText(ConversationMessage{Content: thirdParty}, aimodelruntime.PartInputText); ok {
		t.Fatal("derived third-party content must be ineligible")
	}
	tool, _ := json.Marshal([]aimodelruntime.ContentPart{{Type: aimodelruntime.PartToolResult, ToolCallID: "call", Result: json.RawMessage(`{}`)}})
	if _, ok = eligibleText(ConversationMessage{Content: tool}, aimodelruntime.PartInputText); ok {
		t.Fatal("pure tool result must be ineligible")
	}
}

func TestMemoryCandidateStrictSchemaAndBoundedSources(t *testing.T) {
	var empty memoryExtractionOutput
	if err := strictJSON([]byte(`{"candidates":[]}`), &empty); err != nil || len(empty.Candidates) != 0 {
		t.Fatalf("empty candidates must be valid: %v", err)
	}
	valid := memoryCandidate{Kind: aimemory.TypePreference, Summary: "称呼偏好", Content: "以后称呼用户为老板", DatePrecision: aimemory.DateExactTime, Confidence: .9, Sensitivity: aimemory.SensitivityNormal, Importance: 80, ExplicitUserIntent: true, SourceMessageIDs: []string{"m1"}, Reason: "preference"}
	if err := validateCandidate(valid, map[string]bool{"m1": true}); err != nil {
		t.Fatal(err)
	}
	valid.SourceMessageIDs = []string{"outside"}
	if err := validateCandidate(valid, map[string]bool{"m1": true}); err == nil {
		t.Fatal("out-of-range source must fail")
	}
	var output memoryExtractionOutput
	// Unknown fields are now tolerated for LLM compatibility (strict mode relaxed).
	if err := strictJSON([]byte(`{"candidates":[],"tenant_id":"invented"}`), &output); err != nil {
		t.Fatalf("unknown fields must be tolerated: %v", err)
	}
	if schema := string(memoryCandidateSchema()); strings.Contains(schema, "tenant_id") || strings.Contains(schema, "user_id") || strings.Contains(schema, "run_id") {
		t.Fatal("model schema must not accept authority identifiers")
	}
}
