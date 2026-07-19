package aimemory

import (
	"errors"
	"fmt"
)

const SettingKeyPolicyDefaults = "ai.memory_policy"

type WriteApprovalMode string

const (
	ApprovalAlways        WriteApprovalMode = "always"
	ApprovalSensitiveOnly WriteApprovalMode = "sensitive_only"
	ApprovalNever         WriteApprovalMode = "never"
)

type Policy struct {
	Enabled                 bool              `json:"enabled"`
	ExtractionModelAlias    string            `json:"extraction_model_alias"`
	ConsolidationModelAlias string            `json:"consolidation_model_alias,omitempty"`
	AutoSaveLowSensitive    bool              `json:"auto_save_low_sensitive"`
	WriteApprovalMode       WriteApprovalMode `json:"write_approval_mode"`
	AgentSpecificScope      bool              `json:"agent_specific_scope"`
	MaxCandidatesPerTurn    int               `json:"max_candidates_per_turn"`
	MaxConsolidationItems   int               `json:"max_consolidation_items"`
	MaxRecallItems          int               `json:"max_recall_items"`
	MaxRecallTokens         int               `json:"max_recall_tokens"`
	PolicyVersion           string            `json:"policy_version"`
	PromptVersion           string            `json:"prompt_version"`
	SchemaVersion           string            `json:"schema_version"`
}

func SystemDefaultPolicy() Policy {
	return Policy{Enabled: false, ExtractionModelAlias: "default", AutoSaveLowSensitive: false, WriteApprovalMode: ApprovalAlways, MaxCandidatesPerTurn: 8, MaxConsolidationItems: 50, MaxRecallItems: 10, MaxRecallTokens: 2000, PolicyVersion: "1", PromptVersion: "1", SchemaVersion: "1"}
}
func (p Policy) Validate() error {
	if p.WriteApprovalMode != ApprovalAlways && p.WriteApprovalMode != ApprovalSensitiveOnly && p.WriteApprovalMode != ApprovalNever {
		return fmt.Errorf("%w: invalid approval mode", ErrInvalidMemory)
	}
	if p.Enabled && p.ExtractionModelAlias == "" {
		return fmt.Errorf("%w: extraction alias required", ErrInvalidMemory)
	}
	if p.MaxCandidatesPerTurn < 1 || p.MaxCandidatesPerTurn > 100 || p.MaxConsolidationItems < 1 || p.MaxConsolidationItems > 1000 || p.MaxRecallItems < 1 || p.MaxRecallItems > 100 || p.MaxRecallTokens < 128 || p.MaxRecallTokens > 100000 {
		return fmt.Errorf("%w: invalid budget", ErrInvalidMemory)
	}
	if p.PolicyVersion == "" || p.PromptVersion == "" || p.SchemaVersion == "" {
		return fmt.Errorf("%w: versions required", ErrInvalidMemory)
	}
	return nil
}

type ItemType string
type ItemStatus string
type SourceKind string
type DatePrecision string
type Sensitivity string
type ProjectionKind string
type ProjectionStatus string

const (
	TypePreference ItemType = "preference"
	TypeFact       ItemType = "fact"
	TypeContext    ItemType = "context"
	TypeEvent      ItemType = "event"

	StatusCandidate     ItemStatus = "candidate"
	StatusPendingReview ItemStatus = "pending_review"
	StatusActive        ItemStatus = "active"
	StatusSuperseded    ItemStatus = "superseded"
	StatusRevoked       ItemStatus = "revoked"
	StatusExpired       ItemStatus = "expired"
	StatusDeleted       ItemStatus = "deleted"
	StatusRejected      ItemStatus = "rejected"

	SourceUserStatement SourceKind = "user_statement"
	SourceAssistant     SourceKind = "assistant_inference"
	SourceTool          SourceKind = "tool"
	SourceKnowledge     SourceKind = "knowledge"
	SourceWeb           SourceKind = "web"

	DateExactTime   DatePrecision = "exact_time"
	DateExactDay    DatePrecision = "exact_day"
	DateRange       DatePrecision = "range"
	DateApproximate DatePrecision = "approximate"
	DateUnknown     DatePrecision = "unknown"

	SensitivityNormal     Sensitivity = "normal"
	SensitivitySensitive  Sensitivity = "sensitive"
	SensitivityRestricted Sensitivity = "restricted"

	ProjectionExact   ProjectionKind = "exact"
	ProjectionFTS     ProjectionKind = "fts"
	ProjectionVector  ProjectionKind = "vector"
	ProjectionDay     ProjectionKind = "day_summary"
	ProjectionProfile ProjectionKind = "current_profile"

	ProjectionPending ProjectionStatus = "pending"
	ProjectionReady   ProjectionStatus = "ready"
	ProjectionStale   ProjectionStatus = "stale"
	ProjectionDeleted ProjectionStatus = "deleted"
)

var (
	ErrInvalidMemory = errors.New("invalid memory item")
	ErrUnsafeSource  = errors.New("non-user source cannot become an active user fact")
)

func ValidItemType(v ItemType) bool {
	return v == TypePreference || v == TypeFact || v == TypeContext || v == TypeEvent
}
func ValidStatus(v ItemStatus) bool {
	switch v {
	case StatusCandidate, StatusPendingReview, StatusActive, StatusSuperseded, StatusRevoked, StatusExpired, StatusDeleted, StatusRejected:
		return true
	}
	return false
}
func ValidSource(v SourceKind) bool {
	switch v {
	case SourceUserStatement, SourceAssistant, SourceTool, SourceKnowledge, SourceWeb:
		return true
	}
	return false
}
func ValidDatePrecision(v DatePrecision) bool {
	switch v {
	case DateExactTime, DateExactDay, DateRange, DateApproximate, DateUnknown:
		return true
	}
	return false
}
