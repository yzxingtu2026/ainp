package aimodelruntime

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type AnthropicAdapter struct{ Client HTTPClient }

func NewAnthropicAdapter(client HTTPClient) *AnthropicAdapter {
	if client == nil {
		client = &http.Client{Timeout: 0}
	}
	return &AnthropicAdapter{Client: client}
}
func (a *AnthropicAdapter) payload(in AdapterRequest, stream bool) (map[string]any, error) {
	if err := validateAdapterImages(in, 5*1024*1024); err != nil {
		return nil, err
	}
	r := in.Request
	system := []string{}
	if r.Instructions != "" {
		system = append(system, r.Instructions)
	}
	messages := []any{}
	for _, m := range r.Messages {
		if m.Role == RoleSystem || m.Role == RoleDeveloper {
			for _, p := range m.Content {
				if p.Type == PartInputText {
					system = append(system, p.Text)
				}
			}
			continue
		}
		content := []any{}
		for _, p := range m.Content {
			switch p.Type {
			case PartInputText:
				content = append(content, map[string]any{"type": "text", "text": p.Text})
			case PartInputImage:
				if p.ResolvedImage != nil {
					content = append(content, map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": p.ResolvedImage.MIME, "data": base64.StdEncoding.EncodeToString(p.ResolvedImage.Data)}})
				}
			case PartOutputText:
				content = append(content, map[string]any{"type": "text", "text": p.Text})
			case PartToolResult:
				var result any
				_ = json.Unmarshal(p.Result, &result)
				content = append(content, map[string]any{"type": "tool_result", "tool_use_id": p.ToolCallID, "content": result})
			}
		}
		messages = append(messages, map[string]any{"role": m.Role, "content": content})
	}
	p := map[string]any{"model": in.Model.RemoteModelID, "messages": messages, "stream": stream, "max_tokens": *r.MaxOutputTokens}
	if len(system) > 0 {
		p["system"] = strings.Join(system, "\n\n")
	}
	if r.Temperature != nil {
		p["temperature"] = *r.Temperature
	}
	if len(r.Stop) > 0 {
		p["stop_sequences"] = r.Stop
	}
	if len(r.Tools) > 0 {
		tools := []any{}
		for _, t := range r.Tools {
			var schema any
			_ = json.Unmarshal(t.Parameters, &schema)
			tools = append(tools, map[string]any{"name": t.Name, "description": t.Description, "input_schema": schema})
		}
		p["tools"] = tools
	}
	if r.ToolChoice != nil {
		choice := map[string]any{"type": r.ToolChoice.Mode}
		if r.ToolChoice.Name != "" {
			choice = map[string]any{"type": "tool", "name": r.ToolChoice.Name}
		}
		p["tool_choice"] = choice
	}
	return p, nil
}

type anthropicResponse struct {
	ID         string `json:"id"`
	StopReason string `json:"stop_reason"`
	Content    []struct {
		Type  string `json:"type"`
		Text  string `json:"text"`
		ID    string `json:"id"`
		Name  string `json:"name"`
		Input json.RawMessage
	}
	Usage struct {
		InputTokens          int64  `json:"input_tokens"`
		OutputTokens         int64  `json:"output_tokens"`
		CacheReadInputTokens *int64 `json:"cache_read_input_tokens"`
	} `json:"usage"`
}

func (a *AnthropicAdapter) Generate(ctx context.Context, in AdapterRequest) (*GenerateResponse, error) {
	headers := map[string]string{"x-api-key": in.Model.Credential, "anthropic-version": "2023-06-01"}
	payload, err := a.payload(in, false)
	if err != nil {
		return nil, err
	}
	resp, err := postJSON(ctx, a.Client, endpoint(defaultBaseURL(in.Model.BaseURL, "https://api.anthropic.com"), "/v1/messages"), headers, payload)
	if err != nil {
		return nil, err
	}
	var raw anthropicResponse
	if err := decodeBody(resp, &raw); err != nil {
		return nil, err
	}
	out := &GenerateResponse{ID: raw.ID, Model: modelSummary(in), FinishReason: anthropicFinish(raw.StopReason), Output: []OutputItem{}, Usage: Usage{InputTokens: int64ptr(raw.Usage.InputTokens), OutputTokens: int64ptr(raw.Usage.OutputTokens), TotalTokens: int64ptr(raw.Usage.InputTokens + raw.Usage.OutputTokens), CachedInputTokens: raw.Usage.CacheReadInputTokens, Reported: true}}
	for i, c := range raw.Content {
		id := raw.ID + "-" + strconv.Itoa(i)
		if c.Type == "text" {
			out.Output = append(out.Output, OutputItem{ID: id, Type: PartOutputText, Text: c.Text})
		} else if c.Type == "tool_use" {
			out.Output = append(out.Output, OutputItem{ID: id, Type: PartToolCall, ToolCallID: c.ID, ToolName: c.Name, Arguments: c.Input})
		}
	}
	return out, nil
}
func anthropicFinish(v string) FinishReason {
	switch v {
	case "max_tokens":
		return FinishLength
	case "tool_use":
		return FinishToolCall
	case "refusal":
		return FinishContentFilter
	default:
		return FinishCompleted
	}
}
func (a *AnthropicAdapter) Stream(ctx context.Context, in AdapterRequest) (EventStream, error) {
	headers := map[string]string{"x-api-key": in.Model.Credential, "anthropic-version": "2023-06-01"}
	payload, err := a.payload(in, true)
	if err != nil {
		return nil, err
	}
	resp, err := postJSON(ctx, a.Client, endpoint(defaultBaseURL(in.Model.BaseURL, "https://api.anthropic.com"), "/v1/messages"), headers, payload)
	if err != nil {
		return nil, err
	}
	seq := int64(0)
	responseID := ""
	items := map[int]string{}
	toolItems := map[int]bool{}
	toolArguments := map[int]string{}
	finishReason := FinishCompleted
	mapper := func(data json.RawMessage) ([]StreamEvent, error) {
		var e struct {
			Type    string `json:"type"`
			Index   int    `json:"index"`
			Message struct {
				ID    string `json:"id"`
				Usage struct {
					InputTokens  int64 `json:"input_tokens"`
					OutputTokens int64 `json:"output_tokens"`
				}
			} `json:"message"`
			ContentBlock struct {
				Type string `json:"type"`
				ID   string `json:"id"`
				Name string `json:"name"`
				Text string `json:"text"`
			} `json:"content_block"`
			Delta struct {
				Type        string `json:"type"`
				Text        string `json:"text"`
				PartialJSON string `json:"partial_json"`
				StopReason  string `json:"stop_reason"`
			} `json:"delta"`
			Usage struct {
				OutputTokens int64 `json:"output_tokens"`
			}
		}
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, err
		}
		seq++
		now := time.Now().UTC()
		event := StreamEvent{Sequence: seq, ResponseID: responseID, At: now}
		switch e.Type {
		case "message_start":
			responseID = e.Message.ID
			event.ResponseID = responseID
			event.Type = EventResponseStarted
		case "content_block_start":
			id := responseID + "-" + strconv.Itoa(e.Index)
			items[e.Index] = id
			event.ItemID = id
			if e.ContentBlock.Type == "tool_use" {
				toolItems[e.Index] = true
				event.Type = EventToolCallStarted
				event.Item = &OutputItem{ID: id, Type: PartToolCall, ToolCallID: e.ContentBlock.ID, ToolName: e.ContentBlock.Name}
			} else {
				event.Type = EventOutputItemStarted
				event.Item = &OutputItem{ID: id, Type: PartOutputText}
			}
		case "content_block_delta":
			event.ItemID = items[e.Index]
			if e.Delta.Type == "input_json_delta" {
				event.Type = EventToolArgumentsDelta
				event.Delta = e.Delta.PartialJSON
				toolArguments[e.Index] += e.Delta.PartialJSON
			} else {
				event.Type = EventOutputTextDelta
				event.Delta = e.Delta.Text
			}
		case "content_block_stop":
			event.ItemID = items[e.Index]
			if toolItems[e.Index] {
				event.Type = EventToolCallCompleted
				event.Item = &OutputItem{ID: event.ItemID, Type: PartToolCall, Arguments: json.RawMessage(toolArguments[e.Index])}
			} else {
				event.Type = EventOutputItemCompleted
			}
		case "message_delta":
			event.Type = EventUsageUpdated
			event.Usage = &Usage{OutputTokens: int64ptr(e.Usage.OutputTokens), Reported: true}
			finishReason = anthropicFinish(e.Delta.StopReason)
			event.FinishReason = finishReason
		case "message_stop":
			event.Type = EventResponseCompleted
			event.FinishReason = finishReason
		case "error":
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

var _ ModelAdapter = (*AnthropicAdapter)(nil)
