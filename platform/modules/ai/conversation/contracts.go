package conversation

import (
	"context"
	"time"

	"github.com/yzxingtu2026/ainp/platform/framework/aiagentruntime"
	"github.com/yzxingtu2026/ainp/platform/framework/aimemory"
)

const (
	ActionAgentRunStart     = "ai.agent_run_start"
	ActionAgentRunGet       = "ai.agent_run_get"
	ActionAgentRunList      = "ai.agent_run_list"
	ActionAgentRunCancel    = "ai.agent_run_cancel"
	ActionAgentRunResume    = "ai.agent_run_resume"
	ActionAgentRunRetry     = "ai.agent_run_retry"
	ActionAgentRunEvents    = "ai.agent_run_events"
	ActionAgentRunSubscribe = "ai.agent_run_subscribe"

	AgentRunTerminalEvent = aiagentruntime.EventRunTerminal
)

type MemoryRecallRequest struct {
	Query            string
	AgentID          string
	Types            []aimemory.ItemType
	DateFrom, DateTo *time.Time
	Limit, MaxTokens int
	NormalOnly       bool
}

type MemoryProvenanceRef struct {
	ItemID       string   `json:"item_id"`
	ItemRevision uint64   `json:"item_revision"`
	EvidenceIDs  []string `json:"evidence_ids"`
}

type MemoryRecallItem struct {
	Type             aimemory.ItemType      `json:"type"`
	Summary          string                 `json:"summary"`
	Content          string                 `json:"content"`
	Score            float64                `json:"score"`
	OccurredAt       *time.Time             `json:"occurred_at,omitempty"`
	OccurredEndAt    *time.Time             `json:"occurred_end_at,omitempty"`
	OccurredTimezone string                 `json:"occurred_timezone"`
	LocalDate        *time.Time             `json:"local_date,omitempty"`
	DatePrecision    aimemory.DatePrecision `json:"date_precision"`
	Provenance       MemoryProvenanceRef    `json:"provenance"`
}

type MemoryRecallResponse struct {
	Items []MemoryRecallItem `json:"items"`
}

type MemoryRecallFunc func(context.Context, MemoryRecallRequest) (MemoryRecallResponse, error)
