package aimemory

import "errors"

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
	case StatusCandidate, StatusPendingReview, StatusActive, StatusSuperseded, StatusRevoked, StatusExpired, StatusDeleted:
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
