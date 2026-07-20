package aimodelruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v5"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodel"
)

type Role string
type PartType string
type FinishReason string
type EventType string
type ErrorCategory string

type ImageDetail string

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
	PartDerivedText             PartType      = "derived_text"
	PartInputAudio              PartType      = "input_audio"
	PartOutputAudio             PartType      = "output_audio"
	PartInputFile               PartType      = "input_file"
	ImageDetailAuto             ImageDetail   = "auto"
	ImageDetailLow              ImageDetail   = "low"
	ImageDetailHigh             ImageDetail   = "high"
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
	ErrorInputTooLarge          ErrorCategory = "input_too_large"
	ErrorBatchTooLarge          ErrorCategory = "batch_too_large"
	ErrorDimensionsUnsupported  ErrorCategory = "dimensions_unsupported"
	ErrorInvalidVector          ErrorCategory = "invalid_vector"
	ErrorResponseMismatch       ErrorCategory = "response_mismatch"
)

type ContentPart struct {
	Type               PartType           `json:"type"`
	Text               string             `json:"text,omitempty"`
	ImageAttachmentID  string             `json:"image_attachment_id,omitempty"`
	AttachmentRevision string             `json:"attachment_revision,omitempty"`
	ImageDetail        ImageDetail        `json:"detail,omitempty"`
	ToolCallID         string             `json:"tool_call_id,omitempty"`
	ToolName           string             `json:"tool_name,omitempty"`
	Arguments          json.RawMessage    `json:"arguments,omitempty"`
	Result             json.RawMessage    `json:"result,omitempty"`
	DerivedSource      *DerivedTextSource `json:"derived_source,omitempty"`
	ResolvedImage      *ResolvedImage     `json:"-"`
}

func (p *ContentPart) UnmarshalJSON(data []byte) error {
	type alias ContentPart
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	allowed := map[string]bool{"type": true, "text": true, "image_attachment_id": true, "attachment_revision": true, "detail": true, "tool_call_id": true, "tool_name": true, "arguments": true, "result": true, "derived_source": true}
	for key := range fields {
		if !allowed[key] {
			return fmt.Errorf("unsupported content part field: %s", key)
		}
	}
	var value alias
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*p = ContentPart(value)
	return nil
}

type DerivedTextSource struct {
	AttachmentID       string    `json:"attachment_id"`
	Revision           string    `json:"revision"`
	DerivationType     string    `json:"derivation_type"`
	VisionModelAlias   string    `json:"vision_model_alias"`
	VisionDeploymentID string    `json:"vision_deployment_id"`
	PromptVersion      string    `json:"prompt_version"`
	SchemaVersion      string    `json:"schema_version"`
	GeneratedAt        time.Time `json:"generated_at"`
	Untrusted          bool      `json:"untrusted"`
}

type ResolvedImage struct {
	AttachmentID string `json:"-"`
	Revision     string `json:"-"`
	MIME         string `json:"-"`
	Size         int64  `json:"-"`
	Width        int    `json:"-"`
	Height       int    `json:"-"`
	SHA256       string `json:"-"`
	Data         []byte `json:"-"`
}

type AssetResolveRequest struct {
	AttachmentID string
	Purpose      string
}
type AssetResolver interface {
	Resolve(context.Context, AssetResolveRequest) (*ResolvedImage, error)
}

type MediaPolicy string

const (
	MediaPolicyNativeOnly                    MediaPolicy = "native_only"
	MediaPolicyAllowDerivation               MediaPolicy = "allow_derivation"
	MediaPolicyRequireDerivationConfirmation MediaPolicy = "require_derivation_confirmation"
)

type MediaUnderstandingRequest struct{ SourceAttachmentID, SourceRevision, VisionModelAlias, VisionDeploymentID, PromptVersion, SchemaVersion, Locale string }
type MediaUnderstandingService interface {
	DeriveText(context.Context, MediaUnderstandingRequest) (ContentPart, error)
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
type StructuredOutputStrategy = aimodel.StructuredOutputStrategy

const (
	StructuredOutputOpenAIJSONSchema      = aimodel.StructuredOutputOpenAIJSONSchema
	StructuredOutputAnthropicOutputConfig = aimodel.StructuredOutputAnthropicOutputConfig
	StructuredOutputConstrainedDecoding   = aimodel.StructuredOutputConstrainedDecoding
	StructuredOutputResponseFunction      = aimodel.StructuredOutputResponseOnlyFunction
	StructuredOutputJSONMode              = aimodel.StructuredOutputJSONMode
	StructuredOutputTextJSON              = aimodel.StructuredOutputTextJSON
)

type StructuredOutputStrategyProvider interface {
	StructuredOutputStrategies() []StructuredOutputStrategy
}

func SelectStructuredOutputStrategy(model *aimodel.ResolvedModel, adapter ModelAdapter, contract *StructuredOutput) (StructuredOutputStrategy, error) {
	if contract == nil {
		return "", nil
	}
	provider, ok := adapter.(StructuredOutputStrategyProvider)
	if !ok {
		return "", &RuntimeError{Category: ErrorCapabilityNotSupported, Message: "adapter does not support structured output"}
	}
	supported := map[StructuredOutputStrategy]bool{}
	for _, strategy := range provider.StructuredOutputStrategies() {
		supported[strategy] = true
	}
	for _, strategy := range model.Capabilities.StructuredOutputStrategies {
		if supported[strategy] {
			if contract.Strict && strategy == StructuredOutputTextJSON {
				continue
			}
			return strategy, nil
		}
	}
	return "", &RuntimeError{Category: ErrorCapabilityNotSupported, Message: "deployment and adapter have no compatible structured output strategy"}
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
	MediaPolicy      MediaPolicy                `json:"media_policy,omitempty"`
}

func (r *GenerateRequest) Normalize() {
	if r.TimeoutMS == 0 {
		r.TimeoutMS = 60000
	}
	if r.MediaPolicy == "" {
		r.MediaPolicy = MediaPolicyNativeOnly
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
	if r.MediaPolicy != MediaPolicyNativeOnly {
		return fmt.Errorf("media policy is not implemented: %s", r.MediaPolicy)
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
			if err := validatePart(m.Role, p); err != nil {
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

// ValidateMessage validates a canonical message without requiring a model request.
func ValidateMessage(m Message) error {
	if !validRole(m.Role) || len(m.Content) == 0 {
		return fmt.Errorf("invalid message")
	}
	for _, part := range m.Content {
		if err := validatePart(m.Role, part); err != nil {
			return err
		}
	}
	return nil
}
func validatePart(role Role, p ContentPart) error {
	switch p.Type {
	case PartInputText:
		if p.Text == "" {
			return fmt.Errorf("input_text requires text")
		}
		if role != RoleUser && role != RoleSystem && role != RoleDeveloper {
			return fmt.Errorf("input_text is invalid for role %s", role)
		}
	case PartInputImage:
		if p.ImageAttachmentID == "" {
			return fmt.Errorf("input_image requires attachment id")
		}
		if p.Text != "" || p.ToolCallID != "" || p.ToolName != "" || len(p.Arguments) > 0 || len(p.Result) > 0 || p.DerivedSource != nil {
			return fmt.Errorf("input_image only accepts attachment reference and detail")
		}
		if role != RoleUser {
			return fmt.Errorf("input_image is only allowed for user messages")
		}
		if p.ImageDetail != "" && p.ImageDetail != ImageDetailAuto && p.ImageDetail != ImageDetailLow && p.ImageDetail != ImageDetailHigh {
			return fmt.Errorf("invalid image detail")
		}
	case PartOutputText, PartRefusal:
		if role != RoleAssistant || p.Text == "" {
			return fmt.Errorf("%s is only allowed for assistant messages", p.Type)
		}
	case PartToolCall:
		if role != RoleAssistant || p.ToolCallID == "" || p.ToolName == "" || !json.Valid(p.Arguments) {
			return fmt.Errorf("invalid tool_call")
		}
	case PartToolResult:
		if role != RoleTool || p.ToolCallID == "" || !json.Valid(p.Result) {
			return fmt.Errorf("invalid tool_result")
		}
	case PartDerivedText:
		if role != RoleUser || p.Text == "" || p.DerivedSource == nil || !p.DerivedSource.Untrusted || p.DerivedSource.AttachmentID == "" || p.DerivedSource.Revision == "" {
			return fmt.Errorf("invalid derived_text")
		}
	case PartInputAudio, PartOutputAudio, PartInputFile:
		return fmt.Errorf("content part is not implemented: %s", p.Type)
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
	InvocationID      string       `json:"invocation_id,omitempty"`
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
	Request                  GenerateRequest
	Model                    *aimodel.ResolvedModel
	StructuredOutputStrategy StructuredOutputStrategy
}
type ModelAdapter interface {
	Generate(context.Context, AdapterRequest) (*GenerateResponse, error)
	Stream(context.Context, AdapterRequest) (EventStream, error)
}

func ValidateStructuredResponse(response *GenerateResponse, contract *StructuredOutput) error {
	if response.FinishReason == FinishLength {
		return &RuntimeError{Category: ErrorResponseMismatch, Message: "structured response was truncated"}
	}
	text := ""
	for _, item := range response.Output {
		if item.Type == PartRefusal {
			return &RuntimeError{Category: ErrorContentFiltered, Message: "provider refused structured response"}
		}
		if item.Type != PartOutputText {
			return &RuntimeError{Category: ErrorResponseMismatch, Message: "structured response contained a non-text item"}
		}
		text += item.Text
	}
	if !json.Valid([]byte(text)) {
		return &RuntimeError{Category: ErrorResponseMismatch, Message: "provider returned invalid structured JSON"}
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("output.json", bytes.NewReader(contract.Schema)); err != nil {
		return &RuntimeError{Category: ErrorInvalidRequest, Message: "structured output schema is invalid", Cause: err}
	}
	schema, err := compiler.Compile("output.json")
	if err != nil {
		return &RuntimeError{Category: ErrorInvalidRequest, Message: "structured output schema is invalid", Cause: err}
	}
	var value any
	decoder := json.NewDecoder(bytes.NewBufferString(text))
	decoder.UseNumber()
	if err = decoder.Decode(&value); err != nil || schema.Validate(value) != nil {
		return &RuntimeError{Category: ErrorResponseMismatch, Message: "provider response does not match structured output schema"}
	}
	return nil
}
