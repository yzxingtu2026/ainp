package aimodelruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/aimodel"
)

type Role string
type PartType string
type FinishReason string
type EventType string
type ErrorCategory string

const (
	RoleSystem                  Role          = "system"
	RoleDeveloper               Role          = "developer"
	RoleUser                    Role          = "user"
	RoleAssistant               Role          = "assistant"
	RoleTool                    Role          = "tool"
	PartInputText               PartType      = "input_text"
	PartInputImage              PartType      = "input_image"
	PartOutputText              PartType      = "output_text"
	PartToolCall                PartType      = "tool_call"
	PartToolResult              PartType      = "tool_result"
	PartRefusal                 PartType      = "refusal"
	FinishCompleted             FinishReason  = "completed"
	FinishLength                FinishReason  = "length"
	FinishToolCall              FinishReason  = "tool_call"
	FinishContentFilter         FinishReason  = "content_filter"
	FinishCancelled             FinishReason  = "cancelled"
	FinishError                 FinishReason  = "error"
	EventResponseStarted        EventType     = "response.started"
	EventOutputItemStarted      EventType     = "output_item.started"
	EventOutputTextDelta        EventType     = "output_text.delta"
	EventToolCallStarted        EventType     = "tool_call.started"
	EventToolArgumentsDelta     EventType     = "tool_call.arguments.delta"
	EventToolCallCompleted      EventType     = "tool_call.completed"
	EventOutputItemCompleted    EventType     = "output_item.completed"
	EventUsageUpdated           EventType     = "usage.updated"
	EventResponseCompleted      EventType     = "response.completed"
	EventResponseFailed         EventType     = "response.failed"
	ErrorInvalidRequest         ErrorCategory = "invalid_request"
	ErrorAuthenticationFailed   ErrorCategory = "authentication_failed"
	ErrorPermissionDenied       ErrorCategory = "permission_denied"
	ErrorModelNotFound          ErrorCategory = "model_not_found"
	ErrorCapabilityNotSupported ErrorCategory = "capability_not_supported"
	ErrorRateLimited            ErrorCategory = "rate_limited"
	ErrorContextLength          ErrorCategory = "context_length_exceeded"
	ErrorContentFiltered        ErrorCategory = "content_filtered"
	ErrorProviderUnavailable    ErrorCategory = "provider_unavailable"
	ErrorTimeout                ErrorCategory = "timeout"
	ErrorCancelled              ErrorCategory = "cancelled"
	ErrorStreamProtocol         ErrorCategory = "stream_protocol_error"
)

type ContentPart struct {
	Type              PartType        `json:"type"`
	Text              string          `json:"text,omitempty"`
	ImageAttachmentID string          `json:"image_attachment_id,omitempty"`
	ToolCallID        string          `json:"tool_call_id,omitempty"`
	ToolName          string          `json:"tool_name,omitempty"`
	Arguments         json.RawMessage `json:"arguments,omitempty"`
	Result            json.RawMessage `json:"result,omitempty"`
}
type Message struct {
	Role    Role          `json:"role"`
	Content []ContentPart `json:"content" validate:"required,min=1,dive"`
}
type ToolDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
}
type ToolChoice struct {
	Mode string `json:"mode"`
	Name string `json:"name,omitempty"`
}
type StructuredOutput struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Schema      json.RawMessage `json:"schema"`
	Strict      bool            `json:"strict"`
}
type GenerateRequest struct {
	ModelAlias       string                     `json:"model_alias" validate:"required"`
	ToolScopeID      string                     `json:"tool_scope_id,omitempty"`
	EnableTools      bool                       `json:"enable_tools,omitempty"`
	Instructions     string                     `json:"instructions,omitempty"`
	Messages         []Message                  `json:"messages" validate:"required,min=1,dive"`
	Tools            []ToolDefinition           `json:"tools,omitempty" validate:"max=64,dive"`
	ToolChoice       *ToolChoice                `json:"tool_choice,omitempty"`
	StructuredOutput *StructuredOutput          `json:"structured_output,omitempty"`
	Temperature      *float64                   `json:"temperature,omitempty"`
	MaxOutputTokens  *int64                     `json:"max_output_tokens,omitempty"`
	Stop             []string                   `json:"stop,omitempty" validate:"max=8"`
	TimeoutMS        int64                      `json:"timeout_ms,omitempty"`
	Metadata         map[string]string          `json:"metadata,omitempty"`
	Extensions       map[string]json.RawMessage `json:"extensions,omitempty"`
}

func (r *GenerateRequest) Normalize() {
	if r.TimeoutMS == 0 {
		r.TimeoutMS = 60000
	}
	if r.MaxOutputTokens == nil {
		v := int64(2048)
		r.MaxOutputTokens = &v
	}
}
func (r GenerateRequest) Validate(context.Context) error {
	if len(r.Extensions) > 0 {
		return fmt.Errorf("adapter extensions are not enabled")
	}
	if r.TimeoutMS < 1000 || r.TimeoutMS > 300000 {
		return fmt.Errorf("timeout_ms must be between 1000 and 300000")
	}
	if r.Temperature != nil && (*r.Temperature < 0 || *r.Temperature > 2) {
		return fmt.Errorf("temperature must be between 0 and 2")
	}
	if r.MaxOutputTokens != nil && (*r.MaxOutputTokens < 1 || *r.MaxOutputTokens > 1000000) {
		return fmt.Errorf("max_output_tokens is invalid")
	}
	for _, m := range r.Messages {
		if !validRole(m.Role) {
			return fmt.Errorf("invalid role: %s", m.Role)
		}
		for _, p := range m.Content {
			if err := validatePart(p); err != nil {
				return err
			}
		}
	}
	for _, t := range r.Tools {
		if t.Name == "" || !json.Valid(t.Parameters) {
			return fmt.Errorf("invalid tool definition")
		}
	}
	if r.ToolChoice != nil {
		switch r.ToolChoice.Mode {
		case "auto", "none", "required":
		default:
			return fmt.Errorf("invalid tool choice mode")
		}
		if r.ToolChoice.Name != "" && r.ToolChoice.Mode != "required" {
			return fmt.Errorf("named tool choice must be required")
		}
	}
	if r.StructuredOutput != nil && !json.Valid(r.StructuredOutput.Schema) {
		return fmt.Errorf("invalid structured output schema")
	}
	return nil
}
func validRole(v Role) bool {
	return v == RoleSystem || v == RoleDeveloper || v == RoleUser || v == RoleAssistant || v == RoleTool
}
func validatePart(p ContentPart) error {
	switch p.Type {
	case PartInputText:
		if p.Text == "" {
			return fmt.Errorf("input_text requires text")
		}
	case PartInputImage:
		if p.ImageAttachmentID == "" {
			return fmt.Errorf("input_image requires attachment id")
		}
	case PartToolResult:
		if p.ToolCallID == "" || !json.Valid(p.Result) {
			return fmt.Errorf("invalid tool_result")
		}
	default:
		return fmt.Errorf("unsupported input part: %s", p.Type)
	}
	return nil
}

type Usage struct {
	InputTokens       *int64 `json:"input_tokens"`
	OutputTokens      *int64 `json:"output_tokens"`
	TotalTokens       *int64 `json:"total_tokens"`
	CachedInputTokens *int64 `json:"cached_input_tokens"`
	ReasoningTokens   *int64 `json:"reasoning_tokens"`
	Reported          bool   `json:"reported"`
}
type OutputItem struct {
	ID         string          `json:"id"`
	Type       PartType        `json:"type"`
	Text       string          `json:"text,omitempty"`
	ToolName   string          `json:"tool_name,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	Arguments  json.RawMessage `json:"arguments,omitempty"`
}
type ModelSummary struct {
	Alias         string `json:"alias"`
	DeploymentID  string `json:"deployment_id"`
	Protocol      string `json:"protocol"`
	RemoteModelID string `json:"remote_model_id"`
	ToolScopeID   string `json:"tool_scope_id,omitempty"`
}
type GenerateResponse struct {
	ID                string       `json:"id"`
	Model             ModelSummary `json:"model"`
	Output            []OutputItem `json:"output"`
	FinishReason      FinishReason `json:"finish_reason"`
	Usage             Usage        `json:"usage"`
	ProviderRequestID string       `json:"-"`
}
type RuntimeError struct {
	Category     ErrorCategory `json:"category"`
	Message      string        `json:"message"`
	Retryable    bool          `json:"retryable"`
	ProviderCode string        `json:"-"`
	Cause        error         `json:"-"`
}

func (e *RuntimeError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

type StreamEvent struct {
	Type         EventType     `json:"type"`
	Sequence     int64         `json:"sequence"`
	ResponseID   string        `json:"response_id"`
	ItemID       string        `json:"item_id,omitempty"`
	Delta        string        `json:"delta,omitempty"`
	Item         *OutputItem   `json:"item,omitempty"`
	Usage        *Usage        `json:"usage,omitempty"`
	FinishReason FinishReason  `json:"finish_reason,omitempty"`
	Error        *RuntimeError `json:"error,omitempty"`
	At           time.Time     `json:"at"`
}
type EventStream interface {
	Recv(context.Context) (StreamEvent, error)
	Close() error
}
type AdapterRequest struct {
	Request GenerateRequest
	Model   *aimodel.ResolvedModel
}
type ModelAdapter interface {
	Generate(context.Context, AdapterRequest) (*GenerateResponse, error)
	Stream(context.Context, AdapterRequest) (EventStream, error)
}
