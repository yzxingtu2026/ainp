package tests

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/aiconversation"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodelruntime"
	"github.com/yzxingtu2026/ainp/platform/modules/ai"
)

var _ aiconversation.ContextAssembler = (*ai.ConversationService)(nil)

func TestConversationRequestsUseStableSnakeCaseContract(t *testing.T) {
	handler := action.JSON(func(_ context.Context, request ai.MessageCreateRequest) (any, error) { return request, nil })
	value, err := handler(context.Background(), json.RawMessage(`{"conversation_id":"c1","role":"user","content":[{"type":"input_image","image_attachment_id":"a1","attachment_revision":"sha256"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	request := value.(ai.MessageCreateRequest)
	if request.ConversationID != "c1" || request.Content[0].AttachmentRevision != "sha256" {
		t.Fatalf("unexpected request: %#v", request)
	}
}

func TestCanonicalUserContentValidation(t *testing.T) {
	valid := aimodelruntime.Message{Role: aimodelruntime.RoleUser, Content: []aimodelruntime.ContentPart{{Type: aimodelruntime.PartInputText, Text: "hello"}}}
	if err := aimodelruntime.ValidateMessage(valid); err != nil {
		t.Fatal(err)
	}
	invalid := aimodelruntime.Message{Role: aimodelruntime.RoleAssistant, Content: valid.Content}
	if err := aimodelruntime.ValidateMessage(invalid); err == nil {
		t.Fatal("assistant input_text must fail")
	}
	var part aimodelruntime.ContentPart
	if err := json.Unmarshal([]byte(`{"type":"input_text","text":"x","secret":"leak"}`), &part); err == nil {
		t.Fatal("unknown content fields must fail closed")
	}
}

func TestConversationMigrationContainsSafetyConstraints(t *testing.T) {
	body, err := os.ReadFile("../migrations/0019_ai_conversations.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(body)
	for _, expected := range []string{"UNIQUE (tenant_id, conversation_id, sequence)", "UNIQUE (tenant_id, conversation_id, idempotency_key)", "ai_assistant_stream_deltas", "UNIQUE (tenant_id, stream_id, cursor)", "CHECK (role IN ('user','assistant','system','tool'))", "CHECK (kind IN ('raw','assembled'))", "ON DELETE RESTRICT"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("migration missing %q", expected)
		}
	}
}
