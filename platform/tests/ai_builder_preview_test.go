package tests

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/modules/ai"
)

func TestBuilderPreviewMigrationConstraints(t *testing.T) {
	body, err := os.ReadFile("../migrations/0023_builder_preview.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(body)
	for _, expected := range []string{
		"kind",
		"content_hash",
		"ck_ai_agent_versions_kind",
		"release",
		"preview",
		"purpose",
		"ck_ai_conversations_purpose",
		"workspace",
		"builder_preview",
		"idx_ai_agent_versions_preview_hash",
		"idx_ai_conversations_purpose",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("migration missing %q", expected)
		}
	}
}

func TestAgentPreviewMaterializeRequestSnakeCase(t *testing.T) {
	handler := action.JSON(func(_ context.Context, request ai.AgentPreviewMaterializeRequest) (any, error) {
		return request, nil
	})
	value, err := handler(context.Background(), json.RawMessage(`{
		"agent_id":"a1",
		"model_alias":"chat-main",
		"tenant_instructions":"hello",
		"generation_config":{"temperature":0.2},
		"tool_policy":{},
		"revision":3
	}`))
	if err != nil {
		t.Fatal(err)
	}
	req := value.(ai.AgentPreviewMaterializeRequest)
	if req.AgentID != "a1" || req.ModelAlias != "chat-main" || req.Revision != 3 || req.TenantInstructions != "hello" {
		t.Fatalf("unexpected materialize request: %#v", req)
	}
}

func TestConversationPurposeDefaultsAndIsolationContract(t *testing.T) {
	if ai.ConversationPurposeWorkspace != "workspace" || ai.ConversationPurposeBuilderPreview != "builder_preview" {
		t.Fatalf("unexpected purpose constants: %s %s", ai.ConversationPurposeWorkspace, ai.ConversationPurposeBuilderPreview)
	}
	if ai.AgentVersionKindRelease != "release" || ai.AgentVersionKindPreview != "preview" {
		t.Fatalf("unexpected version kind constants: %s %s", ai.AgentVersionKindRelease, ai.AgentVersionKindPreview)
	}

	create := action.JSON(func(_ context.Context, request ai.ConversationCreateRequest) (any, error) {
		return request, nil
	})
	value, err := create(context.Background(), json.RawMessage(`{
		"agent_id":"a1",
		"agent_version_id":"v1",
		"purpose":"builder_preview",
		"visibility":"private",
		"title":"debug"
	}`))
	if err != nil {
		t.Fatal(err)
	}
	req := value.(ai.ConversationCreateRequest)
	if req.Purpose != ai.ConversationPurposeBuilderPreview || req.AgentVersionID != "v1" {
		t.Fatalf("unexpected create request: %#v", req)
	}

	list := action.JSON(func(_ context.Context, request ai.ConversationListRequest) (any, error) {
		return request, nil
	})
	value, err = list(context.Background(), json.RawMessage(`{"purpose":"workspace","page":1,"page_size":20}`))
	if err != nil {
		t.Fatal(err)
	}
	listReq := value.(ai.ConversationListRequest)
	if listReq.Purpose != ai.ConversationPurposeWorkspace {
		t.Fatalf("list purpose not decoded: %#v", listReq)
	}
}

func TestAgentDraftConfigMigration(t *testing.T) {
	body, err := os.ReadFile("../migrations/0024_agent_draft_config.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(body)
	for _, expected := range []string{
		"draft_tenant_instructions",
		"draft_model_alias",
		"draft_generation_config",
		"draft_tool_policy",
		"has_draft",
		"idx_ai_agents_tenant_has_draft",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("migration missing %q", expected)
		}
	}
}

func TestAgentUpdateRequestAcceptsDraftConfig(t *testing.T) {
	handler := action.JSON(func(_ context.Context, request ai.AgentUpdateRequest) (any, error) {
		return request, nil
	})
	value, err := handler(context.Background(), json.RawMessage(`{
		"id":"a1",
		"name":"Demo",
		"description":"d",
		"save_draft_config":true,
		"model_alias":"chat-main",
		"tenant_instructions":"hello draft",
		"generation_config":{"temperature":0.3},
		"tool_policy":{},
		"revision":4
	}`))
	if err != nil {
		t.Fatal(err)
	}
	req := value.(ai.AgentUpdateRequest)
	if !req.SaveDraftConfig || req.ModelAlias != "chat-main" || req.TenantInstructions != "hello draft" || req.Revision != 4 {
		t.Fatalf("unexpected update request: %#v", req)
	}
}

func TestAgentJSONExposesDraftFields(t *testing.T) {
	row := ai.Agent{
		ID: "a1", Code: "demo", Name: "Demo", Status: "enabled", Revision: 2,
		HasDraft: true, DraftTenantInstructions: "draft prompt", DraftModelAlias: "chat-main",
		DraftGenerationConfig: json.RawMessage(`{"temperature":0.2}`), DraftToolPolicy: json.RawMessage(`{}`),
	}
	encoded, err := json.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, want := range []string{`"has_draft":true`, `"draft_tenant_instructions":"draft prompt"`, `"draft_model_alias":"chat-main"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %s in %s", want, text)
		}
	}
}

func TestAgentVersionJSONExposesKindAndContentHash(t *testing.T) {
	version := ai.AgentVersion{
		ID: "v1", AgentID: "a1", Version: 2, Kind: ai.AgentVersionKindPreview,
		ContentHash: "abc", ModelAlias: "chat-main", CreatedBy: "u1",
		GenerationConfig: json.RawMessage(`{}`), ToolPolicy: json.RawMessage(`{}`),
	}
	encoded, err := json.Marshal(version)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, want := range []string{`"kind":"preview"`, `"content_hash":"abc"`, `"model_alias":"chat-main"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %s in %s", want, text)
		}
	}
}
