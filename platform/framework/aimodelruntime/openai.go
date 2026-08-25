package aimodelruntime

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type OpenAIAdapter struct{ Client HTTPClient }

func NewOpenAIAdapter(client HTTPClient) *OpenAIAdapter {
	if client == nil {
		client = &http.Client{Timeout: 0}
	}
	return &OpenAIAdapter{Client: client}
}
func (a *OpenAIAdapter) StructuredOutputStrategies() []StructuredOutputStrategy {
	return []StructuredOutputStrategy{StructuredOutputOpenAIJSONSchema, StructuredOutputResponseFunction}
}
func (a *OpenAIAdapter) headers(key string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + key}
}
func (a *OpenAIAdapter) payload(in AdapterRequest, stream bool) (map[string]any, error) {
	if err := rejectUnexpandedFiles(in.Request.Messages); err != nil {
		return nil, err
	}
	if err := validateAdapterImages(in, 20*1024*1024); err != nil {
		return nil, err
	}
	r := in.Request
	p := map[string]any{"model": in.Model.RemoteModelID, "input": openAIInput(r.Messages), "stream": stream}
	if in.DisableReasoning {
		p["enable_thinking"] = false
	}
	if r.Instructions != "" {
		p["instructions"] = r.Instructions
	}
	if r.MaxOutputTokens != nil {
		p["max_output_tokens"] = *r.MaxOutputTokens
	}
	if r.Temperature != nil {
		p["temperature"] = *r.Temperature
	}
	if len(r.Tools) > 0 {
		tools := make([]any, 0, len(r.Tools))
		for _, t := range r.Tools {
			var schema any
			_ = json.Unmarshal(t.Parameters, &schema)
			tools = append(tools, map[string]any{"type": "function", "name": t.Name, "description": t.Description, "parameters": schema})
		}
		p["tools"] = tools
	}
	if r.ToolChoice != nil {
		if r.ToolChoice.Name != "" {
			p["tool_choice"] = map[string]any{"type": "function", "name": r.ToolChoice.Name}
		} else {
			p["tool_choice"] = r.ToolChoice.Mode
		}
	}
	if r.StructuredOutput != nil {
		var schema any
		_ = json.Unmarshal(r.StructuredOutput.Schema, &schema)
		switch in.StructuredOutputStrategy {
		case StructuredOutputOpenAIJSONSchema:
			p["text"] = map[string]any{"format": map[string]any{"type": "json_schema", "name": r.StructuredOutput.Name, "description": r.StructuredOutput.Description, "schema": schema, "strict": r.StructuredOutput.Strict}}
		case StructuredOutputResponseFunction:
			if len(r.Tools) > 0 {
				return nil, &RuntimeError{Category: ErrorInvalidRequest, Message: "structured output cannot be combined with executable tools"}
			}
			p["tools"] = []any{map[string]any{"type": "function", "name": r.StructuredOutput.Name, "description": r.StructuredOutput.Description, "parameters": schema}}
			// A single private response function plus "required" is supported by
			// both OpenAI Responses and compatible providers such as DashScope.
			p["tool_choice"] = "required"
		default:
			return nil, &RuntimeError{Category: ErrorCapabilityNotSupported, Message: "selected structured output strategy is not supported by openai adapter"}
		}
	}
	return p, nil
}
func openAIInput(messages []Message) []any {
	out := make([]any, 0, len(messages))
	for _, m := range messages {
		content := make([]any, 0, len(m.Content))
		var functionCalls []any
		var functionOutputs []any
		for _, part := range m.Content {
			switch part.Type {
			case PartInputText, PartDerivedText:
				content = append(content, map[string]any{"type": "input_text", "text": part.Text})
			case PartInputImage:
				if part.ResolvedImage != nil {
					detail := part.ImageDetail
					if detail == "" {
						detail = ImageDetailAuto
					}
					content = append(content, map[string]any{"type": "input_image", "image_url": "data:" + part.ResolvedImage.MIME + ";base64," + base64.StdEncoding.EncodeToString(part.ResolvedImage.Data), "detail": detail})
				}
			case PartOutputText:
				content = append(content, map[string]any{"type": "output_text", "text": part.Text})
			case PartToolCall:
				args := string(part.Arguments)
				if args == "" {
					args = "{}"
				}
				functionCalls = append(functionCalls, map[string]any{
					"type": "function_call", "call_id": part.ToolCallID, "name": part.ToolName, "arguments": args,
				})
			case PartToolResult:
				output := string(part.Result)
				if output == "" {
					output = "{}"
				}
				functionOutputs = append(functionOutputs, map[string]any{
					"type": "function_call_output", "call_id": part.ToolCallID, "output": output,
				})
			}
		}
		if len(content) > 0 {
			out = append(out, map[string]any{"role": m.Role, "content": content})
		}
		out = append(out, functionCalls...)
		out = append(out, functionOutputs...)
	}
	return out
}

type openAIResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Output []struct {
		ID        string `json:"id"`
		Type      string `json:"type"`
		CallID    string `json:"call_id"`
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
		Content   []struct{ Type, Text, Refusal string }
	}
	Usage *struct {
		InputTokens        int64 `json:"input_tokens"`
		OutputTokens       int64 `json:"output_tokens"`
		TotalTokens        int64 `json:"total_tokens"`
		InputTokensDetails *struct {
			CachedTokens int64 `json:"cached_tokens"`
		} `json:"input_tokens_details"`
		OutputTokensDetails *struct {
			ReasoningTokens int64 `json:"reasoning_tokens"`
		} `json:"output_tokens_details"`
	}
	IncompleteDetails *struct{ Reason string }
}

func (a *OpenAIAdapter) Generate(ctx context.Context, in AdapterRequest) (*GenerateResponse, error) {
	payload, err := a.payload(in, false)
	if err != nil {
		return nil, err
	}
	resp, err := postJSON(ctx, a.Client, endpoint(defaultBaseURL(in.Model.BaseURL, "https://api.openai.com"), "/v1/responses"), a.headers(in.Model.Credential), payload)
	if err != nil {
		return nil, err
	}
	var raw openAIResponse
	if err := decodeBody(resp, &raw); err != nil {
		return nil, err
	}
	out := &GenerateResponse{ID: raw.ID, Model: modelSummary(in), FinishReason: FinishCompleted, Output: []OutputItem{}}
	for _, item := range raw.Output {
		switch item.Type {
		case "message":
			for _, c := range item.Content {
				if c.Type == "output_text" {
					out.Output = append(out.Output, OutputItem{ID: item.ID, Type: PartOutputText, Text: c.Text})
				} else if c.Type == "refusal" {
					out.Output = append(out.Output, OutputItem{ID: item.ID, Type: PartRefusal, Text: c.Refusal})
					out.FinishReason = FinishContentFilter
				}
			}
		case "function_call":
			if in.Request.StructuredOutput != nil && in.StructuredOutputStrategy == StructuredOutputResponseFunction {
				if item.Name != in.Request.StructuredOutput.Name {
					return nil, &RuntimeError{Category: ErrorResponseMismatch, Message: "structured response function name mismatch"}
				}
				out.Output = append(out.Output, OutputItem{ID: item.ID, Type: PartOutputText, Text: item.Arguments})
			} else {
				out.Output = append(out.Output, OutputItem{ID: item.ID, Type: PartToolCall, ToolCallID: item.CallID, ToolName: item.Name, Arguments: json.RawMessage(item.Arguments)})
				out.FinishReason = FinishToolCall
			}
		}
	}
	if raw.Status == "incomplete" {
		out.FinishReason = FinishLength
		if raw.IncompleteDetails != nil && raw.IncompleteDetails.Reason == "content_filter" {
			out.FinishReason = FinishContentFilter
		}
	}
	if raw.Usage != nil {
		out.Usage = Usage{InputTokens: int64ptr(raw.Usage.InputTokens), OutputTokens: int64ptr(raw.Usage.OutputTokens), TotalTokens: int64ptr(raw.Usage.TotalTokens), Reported: true}
		if raw.Usage.InputTokensDetails != nil {
			out.Usage.CachedInputTokens = int64ptr(raw.Usage.InputTokensDetails.CachedTokens)
		}
		if raw.Usage.OutputTokensDetails != nil {
			out.Usage.ReasoningTokens = int64ptr(raw.Usage.OutputTokensDetails.ReasoningTokens)
		}
	}
	return out, nil
}
func modelSummary(in AdapterRequest) ModelSummary {
	return ModelSummary{Alias: in.Request.ModelAlias, DeploymentID: in.Model.DeploymentID, Protocol: in.Model.Protocol, RemoteModelID: in.Model.RemoteModelID, ToolScopeID: in.Request.ToolScopeID}
}
func (a *OpenAIAdapter) Stream(ctx context.Context, in AdapterRequest) (EventStream, error) {
	payload, err := a.payload(in, true)
	if err != nil {
		return nil, err
	}
	resp, err := postJSON(ctx, a.Client, endpoint(defaultBaseURL(in.Model.BaseURL, "https://api.openai.com"), "/v1/responses"), a.headers(in.Model.Credential), payload)
	if err != nil {
		return nil, err
	}
	seq := int64(0)
	responseID := ""
	refused := false
	toolArguments := map[string]string{}
	mapper := func(data json.RawMessage) ([]StreamEvent, error) {
		var e struct {
			Type      string `json:"type"`
			Sequence  int64  `json:"sequence_number"`
			Delta     string `json:"delta"`
			Arguments string `json:"arguments"`
			Response  struct {
				ID    string `json:"id"`
				Usage *struct {
					InputTokens  int64 `json:"input_tokens"`
					OutputTokens int64 `json:"output_tokens"`
					TotalTokens  int64 `json:"total_tokens"`
				}
			} `json:"response"`
			Item struct {
				ID        string `json:"id"`
				Type      string `json:"type"`
				CallID    string `json:"call_id"`
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			} `json:"item"`
			ItemID string `json:"item_id"`
			Error  any    `json:"error"`
		}
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		if e.Sequence > 0 {
			seq = e.Sequence
		} else {
			seq++
		}
		if e.Response.ID != "" {
			responseID = e.Response.ID
		}
		itemID := e.ItemID
		if itemID == "" {
			itemID = e.Item.ID
		}
		now := time.Now().UTC()
		event := StreamEvent{Sequence: seq, ResponseID: responseID, ItemID: itemID, At: now}
		switch e.Type {
		case "response.created":
			event.Type = EventResponseStarted
		case "response.in_progress":
			return nil, nil
		case "response.output_item.added":
			if e.Item.Type == "function_call" {
				event.Type = EventToolCallStarted
				event.Item = &OutputItem{ID: itemID, Type: PartToolCall, ToolCallID: e.Item.CallID, ToolName: e.Item.Name}
			} else {
				event.Type = EventOutputItemStarted
				event.Item = &OutputItem{ID: itemID, Type: PartOutputText}
			}
		case "response.output_text.delta":
			event.Type = EventOutputTextDelta
			event.Delta = e.Delta
		case "response.refusal.delta":
			refused = true
			event.Type = EventOutputTextDelta
			event.Delta = e.Delta
			event.Item = &OutputItem{ID: itemID, Type: PartRefusal}
		case "response.refusal.done":
			refused = true
			return nil, nil
		case "response.function_call_arguments.delta":
			event.Type = EventToolArgumentsDelta
			event.Delta = e.Delta
			toolArguments[itemID] += e.Delta
		case "response.function_call_arguments.done":
			event.Type = EventToolCallCompleted
			arguments := e.Arguments
			if arguments == "" {
				arguments = e.Item.Arguments
			}
			if arguments == "" {
				arguments = toolArguments[itemID]
			}
			event.Item = &OutputItem{ID: itemID, Type: PartToolCall, Arguments: json.RawMessage(arguments)}
		case "response.output_item.done":
			event.Type = EventOutputItemCompleted
		case "response.completed":
			event.Type = EventResponseCompleted
			event.FinishReason = FinishCompleted
			if refused {
				event.FinishReason = FinishContentFilter
			}
			if e.Response.Usage != nil {
				event.Usage = &Usage{InputTokens: int64ptr(e.Response.Usage.InputTokens), OutputTokens: int64ptr(e.Response.Usage.OutputTokens), TotalTokens: int64ptr(e.Response.Usage.TotalTokens), Reported: true}
			}
		case "response.failed", "error":
			event.Type = EventResponseFailed
			event.FinishReason = FinishError
			event.Error = &RuntimeError{Category: ErrorProviderUnavailable, Message: "model provider stream failed"}
		default:
			return nil, nil
		}
		return []StreamEvent{event}, nil
	}
	return ValidateStream(newHTTPEventStream(resp.Body, mapper)), nil
}

var _ ModelAdapter = (*OpenAIAdapter)(nil)
var _ = fmt.Sprintf
