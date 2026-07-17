package tests

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yzxingtu2026/ainp/platform/framework/aimodel"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodelruntime"
	"github.com/yzxingtu2026/ainp/platform/modules/ai"
)

type runtimeFakeAdapter struct{}

func (runtimeFakeAdapter) Generate(context.Context, aimodelruntime.AdapterRequest) (*aimodelruntime.GenerateResponse, error) {
	return &aimodelruntime.GenerateResponse{}, nil
}
func (runtimeFakeAdapter) Stream(context.Context, aimodelruntime.AdapterRequest) (aimodelruntime.EventStream, error) {
	return nil, nil
}

type runtimeSliceStream struct {
	events []aimodelruntime.StreamEvent
	index  int
}

func (s *runtimeSliceStream) Recv(context.Context) (aimodelruntime.StreamEvent, error) {
	if s.index >= len(s.events) {
		return aimodelruntime.StreamEvent{}, io.EOF
	}
	event := s.events[s.index]
	s.index++
	return event, nil
}
func (s *runtimeSliceStream) Close() error { return nil }

func TestModelAdapterRegistryFailsClosed(t *testing.T) {
	r := aimodelruntime.NewAdapterRegistry()
	if _, ok := r.Get("missing"); ok {
		t.Fatal("unknown protocol must not resolve")
	}
	if err := r.Register("fixture", runtimeFakeAdapter{}); err != nil {
		t.Fatal(err)
	}
	if err := r.Register("fixture", runtimeFakeAdapter{}); err == nil {
		t.Fatal("duplicate protocol must fail")
	}
}

func TestGenerateRequestValidation(t *testing.T) {
	r := aimodelruntime.GenerateRequest{ModelAlias: "chat", Messages: []aimodelruntime.Message{{Role: aimodelruntime.RoleUser, Content: []aimodelruntime.ContentPart{{Type: aimodelruntime.PartInputText, Text: "hello"}}}}}
	r.Normalize()
	if err := r.Validate(context.Background()); err != nil {
		t.Fatal(err)
	}
	r.Messages[0].Content[0].Text = ""
	if err := r.Validate(context.Background()); err == nil {
		t.Fatal("empty input text must fail")
	}
}

func TestOpenAIInputPairsFunctionCallWithOutput(t *testing.T) {
	var gotInput []any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(body["input"])
		if err := json.Unmarshal(raw, &gotInput); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"resp_2","status":"completed","output":[{"id":"msg_2","type":"message","content":[{"type":"output_text","text":"done"}]}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`)
	}))
	defer server.Close()

	req := fixtureAdapterRequest(server.URL, aimodel.ProtocolOpenAI)
	req.Request.Messages = []aimodelruntime.Message{
		{Role: aimodelruntime.RoleUser, Content: []aimodelruntime.ContentPart{{Type: aimodelruntime.PartInputText, Text: "list users"}}},
		{Role: aimodelruntime.RoleAssistant, Content: []aimodelruntime.ContentPart{{
			Type: aimodelruntime.PartToolCall, ToolCallID: "fc_1", ToolName: "tool_search", Arguments: json.RawMessage(`{"query":"user"}`),
		}}},
		{Role: aimodelruntime.RoleTool, Content: []aimodelruntime.ContentPart{{
			Type: aimodelruntime.PartToolResult, ToolCallID: "fc_1", Result: json.RawMessage(`{"items":[]}`),
		}}},
	}
	if _, err := aimodelruntime.NewOpenAIAdapter(server.Client()).Generate(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if len(gotInput) < 3 {
		t.Fatalf("expected paired tool turn input, got %#v", gotInput)
	}
	types := make([]string, 0, len(gotInput))
	for _, item := range gotInput {
		m, _ := item.(map[string]any)
		if typ, ok := m["type"].(string); ok {
			types = append(types, typ)
			continue
		}
		if role, ok := m["role"].(string); ok {
			types = append(types, "role:"+role)
		}
	}
	joined := strings.Join(types, ",")
	if !strings.Contains(joined, "function_call") || !strings.Contains(joined, "function_call_output") {
		t.Fatalf("missing function_call pair in input types %s (%#v)", joined, gotInput)
	}
	callIdx, outIdx := -1, -1
	for i, typ := range types {
		if typ == "function_call" {
			callIdx = i
		}
		if typ == "function_call_output" {
			outIdx = i
		}
	}
	if callIdx < 0 || outIdx < 0 || callIdx > outIdx {
		t.Fatalf("function_call must precede function_call_output: %s", joined)
	}
}

func TestOpenAIResponsesFixtureMapping(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatal("missing auth header")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["model"] != "gpt-test" || body["stream"] != false {
			t.Fatalf("unexpected request: %#v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"resp_1","status":"completed","output":[{"id":"msg_1","type":"message","content":[{"type":"output_text","text":"hello"}]},{"id":"call_1","type":"function_call","call_id":"fc_1","name":"lookup","arguments":"{\"id\":1}"}],"usage":{"input_tokens":7,"output_tokens":3,"total_tokens":10,"input_tokens_details":{"cached_tokens":2},"output_tokens_details":{"reasoning_tokens":1}}}`)
	}))
	defer server.Close()
	adapter := aimodelruntime.NewOpenAIAdapter(server.Client())
	resp, err := adapter.Generate(context.Background(), fixtureAdapterRequest(server.URL, aimodel.ProtocolOpenAI))
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Output) != 2 || resp.Output[0].Text != "hello" || resp.Output[1].ToolName != "lookup" {
		t.Fatalf("unexpected output: %#v", resp.Output)
	}
	if resp.Usage.TotalTokens == nil || *resp.Usage.TotalTokens != 10 || resp.Usage.CachedInputTokens == nil || *resp.Usage.CachedInputTokens != 2 {
		t.Fatalf("unexpected usage: %#v", resp.Usage)
	}
}

func TestAnthropicMessagesFixtureMapping(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "secret" {
			t.Fatal("missing API key")
		}
		_, _ = io.WriteString(w, `{"id":"msg_1","stop_reason":"tool_use","content":[{"type":"text","text":"checking"},{"type":"tool_use","id":"tool_1","name":"lookup","input":{"id":1}}],"usage":{"input_tokens":8,"output_tokens":4,"cache_read_input_tokens":3}}`)
	}))
	defer server.Close()
	resp, err := aimodelruntime.NewAnthropicAdapter(server.Client()).Generate(context.Background(), fixtureAdapterRequest(server.URL, aimodel.ProtocolAnthropic))
	if err != nil {
		t.Fatal(err)
	}
	if resp.FinishReason != aimodelruntime.FinishToolCall || len(resp.Output) != 2 || resp.Output[1].ToolCallID != "tool_1" {
		t.Fatalf("unexpected response: %#v", resp)
	}
	if resp.Usage.CachedInputTokens == nil || *resp.Usage.CachedInputTokens != 3 {
		t.Fatalf("unexpected usage: %#v", resp.Usage)
	}
}

func TestOpenAIStreamIgnoresInProgressAndValidatesSequence(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, strings.Join([]string{
			`data: {"type":"response.created","sequence_number":1,"response":{"id":"resp_1"}}`, "",
			`data: {"type":"response.in_progress","sequence_number":2,"response":{"id":"resp_1"}}`, "",
			`data: {"type":"response.output_item.added","sequence_number":3,"item":{"id":"msg_1","type":"message"}}`, "",
			`data: {"type":"response.output_text.delta","sequence_number":4,"item_id":"msg_1","delta":"hi"}`, "",
			`data: {"type":"response.output_item.done","sequence_number":5,"item":{"id":"msg_1"}}`, "",
			`data: {"type":"response.completed","sequence_number":6,"response":{"id":"resp_1","usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}}`, "", "",
		}, "\n"))
	}))
	defer server.Close()
	stream, err := aimodelruntime.NewOpenAIAdapter(server.Client()).Stream(context.Background(), fixtureAdapterRequest(server.URL, aimodel.ProtocolOpenAI))
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	types := []aimodelruntime.EventType{}
	for {
		event, err := stream.Recv(context.Background())
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		types = append(types, event.Type)
	}
	if len(types) != 5 || types[0] != aimodelruntime.EventResponseStarted || types[4] != aimodelruntime.EventResponseCompleted {
		t.Fatalf("unexpected events: %#v", types)
	}
}

func TestValidatedStreamRejectsEventAfterTerminal(t *testing.T) {
	source := &runtimeSliceStream{events: []aimodelruntime.StreamEvent{{Type: aimodelruntime.EventResponseStarted, Sequence: 1}, {Type: aimodelruntime.EventResponseCompleted, Sequence: 2}, {Type: aimodelruntime.EventUsageUpdated, Sequence: 3}}}
	stream := aimodelruntime.ValidateStream(source)
	if _, err := stream.Recv(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := stream.Recv(context.Background()); err == nil || !strings.Contains(err.Error(), "event after terminal") {
		t.Fatalf("expected post-terminal protocol error, got %v", err)
	}
}

func TestInvocationJSONContainsNoContentOrCredential(t *testing.T) {
	encoded, err := json.Marshal(ai.ModelInvocation{ID: "inv", ModelAlias: "chat", Status: "completed"})
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"prompt", "output", "credential", "arguments", "secret"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("sensitive field leaked: %s", encoded)
		}
	}
}

func fixtureAdapterRequest(baseURL, protocol string) aimodelruntime.AdapterRequest {
	max := int64(64)
	return aimodelruntime.AdapterRequest{Request: aimodelruntime.GenerateRequest{ModelAlias: "chat", MaxOutputTokens: &max, Messages: []aimodelruntime.Message{{Role: aimodelruntime.RoleUser, Content: []aimodelruntime.ContentPart{{Type: aimodelruntime.PartInputText, Text: "hello"}}}}}, Model: &aimodel.ResolvedModel{Protocol: protocol, BaseURL: baseURL, Credential: "secret", DeploymentID: "dep", RemoteModelID: "gpt-test"}}
}
