package aimodelruntime

import (
	"context"
	"encoding/json"
	"net/http"
)

// OpenAICompatibleAdapter maps the canonical protocol to Chat Completions.
// Structured output controls are private adapter details and never become
// executable Tool Runtime definitions.
type OpenAICompatibleAdapter struct{ Client HTTPClient }

func NewOpenAICompatibleAdapter(client HTTPClient) *OpenAICompatibleAdapter {
	if client == nil {
		client = &http.Client{Timeout: 0}
	}
	return &OpenAICompatibleAdapter{Client: client}
}
func (a *OpenAICompatibleAdapter) StructuredOutputStrategies() []StructuredOutputStrategy {
	return []StructuredOutputStrategy{StructuredOutputOpenAIJSONSchema, StructuredOutputResponseFunction, StructuredOutputJSONMode, StructuredOutputTextJSON}
}

func (a *OpenAICompatibleAdapter) payload(in AdapterRequest, stream bool) (map[string]any, StructuredOutputStrategy, error) {
	r := in.Request
	p := map[string]any{"model": in.Model.RemoteModelID, "messages": compatibleMessages(r), "stream": stream}
	if r.MaxOutputTokens != nil {
		p["max_tokens"] = *r.MaxOutputTokens
	}
	if r.Temperature != nil {
		p["temperature"] = *r.Temperature
	}
	if len(r.Tools) > 0 {
		tools := make([]any, 0, len(r.Tools))
		for _, tool := range r.Tools {
			var schema any
			_ = json.Unmarshal(tool.Parameters, &schema)
			tools = append(tools, map[string]any{"type": "function", "function": map[string]any{"name": tool.Name, "description": tool.Description, "parameters": schema}})
		}
		p["tools"] = tools
	}
	if r.ToolChoice != nil {
		if r.ToolChoice.Name != "" {
			p["tool_choice"] = map[string]any{"type": "function", "function": map[string]any{"name": r.ToolChoice.Name}}
		} else {
			p["tool_choice"] = r.ToolChoice.Mode
		}
	}
	strategy := in.StructuredOutputStrategy
	if r.StructuredOutput != nil {
		var schema any
		_ = json.Unmarshal(r.StructuredOutput.Schema, &schema)
		switch strategy {
		case StructuredOutputOpenAIJSONSchema:
			p["response_format"] = map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": r.StructuredOutput.Name, "description": r.StructuredOutput.Description, "schema": schema, "strict": r.StructuredOutput.Strict}}
		case StructuredOutputResponseFunction:
			if len(r.Tools) > 0 {
				return nil, "", &RuntimeError{Category: ErrorInvalidRequest, Message: "structured output cannot be combined with executable tools"}
			}
			p["tools"] = []any{map[string]any{"type": "function", "function": map[string]any{"name": r.StructuredOutput.Name, "description": r.StructuredOutput.Description, "parameters": schema}}}
			p["tool_choice"] = map[string]any{"type": "function", "function": map[string]any{"name": r.StructuredOutput.Name}}
		case StructuredOutputJSONMode:
			p["response_format"] = map[string]any{"type": "json_object"}
			p["messages"] = append([]any{map[string]any{"role": "system", "content": "Return only JSON matching this schema: " + string(r.StructuredOutput.Schema)}}, p["messages"].([]any)...)
		case StructuredOutputTextJSON:
			p["messages"] = append([]any{map[string]any{"role": "system", "content": "Return only JSON matching this schema: " + string(r.StructuredOutput.Schema)}}, p["messages"].([]any)...)
		default:
			return nil, "", &RuntimeError{Category: ErrorCapabilityNotSupported, Message: "selected structured output strategy is not supported by openai-compatible adapter"}
		}
	}
	return p, strategy, nil
}

func compatibleMessages(r GenerateRequest) []any {
	out := []any{}
	if r.Instructions != "" {
		out = append(out, map[string]any{"role": "system", "content": r.Instructions})
	}
	for _, message := range r.Messages {
		text := ""
		toolCalls := []any{}
		for _, part := range message.Content {
			if part.Type == PartInputText || part.Type == PartOutputText {
				text += part.Text
			}
			if part.Type == PartToolCall {
				toolCalls = append(toolCalls, map[string]any{"id": part.ToolCallID, "type": "function", "function": map[string]any{"name": part.ToolName, "arguments": string(part.Arguments)}})
			}
			if part.Type == PartToolResult {
				out = append(out, map[string]any{"role": "tool", "tool_call_id": part.ToolCallID, "content": string(part.Result)})
			}
		}
		if text != "" || len(toolCalls) > 0 {
			item := map[string]any{"role": message.Role, "content": text}
			if len(toolCalls) > 0 {
				item["tool_calls"] = toolCalls
			}
			out = append(out, item)
		}
	}
	return out
}

type compatibleResponse struct {
	ID      string `json:"id"`
	Choices []struct {
		FinishReason string `json:"finish_reason"`
		Message      struct {
			Content   string `json:"content"`
			ToolCalls []struct {
				ID       string                           `json:"id"`
				Function struct{ Name, Arguments string } `json:"function"`
			} `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int64 `json:"prompt_tokens"`
		CompletionTokens int64 `json:"completion_tokens"`
		TotalTokens      int64 `json:"total_tokens"`
	} `json:"usage"`
}

func (a *OpenAICompatibleAdapter) Generate(ctx context.Context, in AdapterRequest) (*GenerateResponse, error) {
	payload, _, err := a.payload(in, false)
	if err != nil {
		return nil, err
	}
	resp, err := postJSON(ctx, a.Client, endpoint(in.Model.BaseURL, "/v1/chat/completions"), map[string]string{"Authorization": "Bearer " + in.Model.Credential}, payload)
	if err != nil {
		return nil, err
	}
	var raw compatibleResponse
	if err = decodeBody(resp, &raw); err != nil {
		return nil, err
	}
	if len(raw.Choices) == 0 {
		return nil, &RuntimeError{Category: ErrorResponseMismatch, Message: "provider returned no choices"}
	}
	choice := raw.Choices[0]
	text := choice.Message.Content
	if in.Request.StructuredOutput != nil {
		strategy := in.StructuredOutputStrategy
		if strategy == StructuredOutputResponseFunction {
			if len(choice.Message.ToolCalls) != 1 || choice.Message.ToolCalls[0].Function.Name != in.Request.StructuredOutput.Name {
				return nil, &RuntimeError{Category: ErrorResponseMismatch, Message: "structured response function mismatch"}
			}
			text = choice.Message.ToolCalls[0].Function.Arguments
		}
	}
	out := &GenerateResponse{ID: raw.ID, Model: modelSummary(in), FinishReason: FinishCompleted, Output: []OutputItem{}}
	if text != "" {
		out.Output = append(out.Output, OutputItem{ID: raw.ID + "-0", Type: PartOutputText, Text: text})
	}
	if in.Request.StructuredOutput == nil {
		for _, call := range choice.Message.ToolCalls {
			out.Output = append(out.Output, OutputItem{ID: call.ID, Type: PartToolCall, ToolCallID: call.ID, ToolName: call.Function.Name, Arguments: json.RawMessage(call.Function.Arguments)})
			out.FinishReason = FinishToolCall
		}
	}
	out.Usage = Usage{InputTokens: int64ptr(raw.Usage.PromptTokens), OutputTokens: int64ptr(raw.Usage.CompletionTokens), TotalTokens: int64ptr(raw.Usage.TotalTokens), Reported: true}
	if choice.FinishReason == "length" {
		out.FinishReason = FinishLength
	}
	return out, nil
}

func (a *OpenAICompatibleAdapter) Stream(context.Context, AdapterRequest) (EventStream, error) {
	return nil, &RuntimeError{Category: ErrorCapabilityNotSupported, Message: "openai-compatible streaming is not implemented"}
}
