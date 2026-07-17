package aiconversation

import (
	"context"

	"github.com/yzxingtu2026/ainp/platform/framework/aimodelruntime"
)

type Budget struct {
	MaxTokens       int `json:"max_tokens"`
	MaxBytes        int `json:"max_bytes"`
	EstimatedTokens int `json:"estimated_tokens"`
	UsedBytes       int `json:"used_bytes"`
}

type Message struct {
	ID       string                       `json:"id"`
	Sequence uint64                       `json:"sequence"`
	Role     aimodelruntime.Role          `json:"role"`
	Content  []aimodelruntime.ContentPart `json:"content"`
}

type AssemblyRequest struct {
	ConversationID string
	MaxTokens      int
	MaxBytes       int
}

type AssemblyResult struct {
	Status               string    `json:"status"`
	Messages             []Message `json:"messages"`
	Budget               Budget    `json:"budget"`
	SourceSequenceFrom   uint64    `json:"source_sequence_from"`
	SourceSequenceTo     uint64    `json:"source_sequence_to"`
	ConversationRevision uint64    `json:"conversation_revision"`
	AgentVersionID       string    `json:"agent_version_id"`
	Reason               string    `json:"reason,omitempty"`
}

type ContextAssembler interface {
	Assemble(context.Context, AssemblyRequest) (*AssemblyResult, error)
}
type ContextCompactor interface {
	Compact(context.Context, CompactionRequest) (*CompactionResult, error)
}

type CompactionRequest struct {
	ConversationID       string  `json:"conversation_id"`
	ModelAlias           string  `json:"model_alias"`
	ReservedOutputTokens int     `json:"reserved_output_tokens"`
	SafetyMarginTokens   int     `json:"safety_margin_tokens"`
	ToolSchemaBytes      int     `json:"tool_schema_bytes"`
	AttachmentTokens     int     `json:"attachment_tokens"`
	IdempotencyKey       string  `json:"idempotency_key"`
	ParentSnapshotID     *string `json:"parent_snapshot_id,omitempty"`
}

type CompactionResult struct {
	Status          string `json:"status"`
	SnapshotID      string `json:"snapshot_id,omitempty"`
	Reason          string `json:"reason,omitempty"`
	InputTokens     int    `json:"input_tokens"`
	OutputTokens    int    `json:"output_tokens"`
	RetainedFrom    uint64 `json:"retained_from,omitempty"`
	RetainedTo      uint64 `json:"retained_to,omitempty"`
	CompactionDepth int    `json:"compaction_depth"`
}

type StreamDelta struct {
	Cursor uint64 `json:"cursor"`
	Type   string `json:"type"`
	Data   []byte `json:"data"`
}

// AssistantStreamStore is the internal replay contract. Commit must publish one
// canonical assistant message for a stream, even after reconnect or retry.
type AssistantStreamStore interface {
	Begin(context.Context, string, string) (streamID string, err error)
	Append(context.Context, string, StreamDelta) error
	Replay(context.Context, string, uint64) ([]StreamDelta, error)
	Commit(context.Context, string, []aimodelruntime.ContentPart) (messageID string, err error)
}
