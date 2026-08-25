package aiagentruntime

import "strings"

const TerminalProtocolVersion = "v1"

type FinalizationMode string

const (
	FinalizationModel      FinalizationMode = "model"
	FinalizationTemplate   FinalizationMode = "template"
	FinalizationNoticeOnly FinalizationMode = "notice_only"
	FinalizationNone       FinalizationMode = "none"
)

type TerminalDecisionInput struct {
	Status              RunStatus
	ErrorCategory       string
	HasSafeRefusalText  bool
	CanUseModel         bool
	CompletedOperations int
}

type TerminalDecision struct {
	ProtocolVersion  string
	Status           RunStatus
	TerminalReason   string
	CompletionReason string
	Mode             FinalizationMode
	Retryable        bool
	UserMessageKey   string
	SafeSummary      string
}

type FailureEnvelope struct {
	SchemaVersion        string `json:"schema_version"`
	Category             string `json:"category"`
	SafeSummary          string `json:"safe_summary"`
	Retryable            bool   `json:"retryable"`
	NextAction           string `json:"next_action"`
	Locale               string `json:"locale,omitempty"`
	CompletedOperations  int    `json:"completed_operations"`
	FailedOperations     int    `json:"failed_operations"`
	UnfinishedOperations int    `json:"unfinished_operations"`
}

type TerminalOperationSummary struct {
	Completed  int
	Failed     int
	Unfinished int
}

var modelFinalizationCategories = map[string]bool{
	"business_validation_failed":  true,
	"resource_not_found":          true,
	"tool_failed":                 true,
	"partial_result":              true,
	"page_context_invalid":        true,
	"runtime_context_failed":      true,
	"document_unavailable":        true,
	"document_unsupported_format": true,
	"document_too_large":          true,
	"document_corrupt":            true,
	"document_empty":              true,
	"document_parse_failed":       true,
}

var providerFailureCategories = map[string]bool{
	"authentication_failed":    true,
	"model_not_found":          true,
	"capability_not_supported": true,
	"rate_limited":             true,
	"provider_unavailable":     true,
	"timeout":                  true,
	"stream_protocol_error":    true,
	"response_mismatch":        true,
	"model_failed":             true,
}

var securityFailureCategories = map[string]bool{
	"authorization_unavailable":    true,
	"authorization_changed":        true,
	"principal_context_invalid":    true,
	"permission_denied":            true,
	"tenant_context_unavailable":   true,
	"preview_version_forbidden":    true,
	"document_unsafe_content":      true,
	"document_unsafe_docx_content": true,
	"document_unsafe_xlsx_content": true,
}

func DecideTerminal(in TerminalDecisionInput) TerminalDecision {
	category := strings.TrimSpace(in.ErrorCategory)
	decision := TerminalDecision{
		ProtocolVersion: TerminalProtocolVersion,
		Status:          in.Status,
		TerminalReason:  category,
		Mode:            FinalizationTemplate,
		UserMessageKey:  "ai.run.terminal.unknown",
		SafeSummary:     "unknown_failure",
	}
	if decision.Status == "" {
		decision.Status = RunFailed
	}
	if category == "" {
		decision.TerminalReason = "unknown"
	}
	if decision.Status == RunCancelled || category == "cancelled" {
		decision.Status = RunCancelled
		decision.TerminalReason = "user_cancelled"
		decision.Mode = FinalizationNoticeOnly
		decision.UserMessageKey = "ai.run.terminal.cancelled"
		decision.SafeSummary = "run_cancelled"
		return decision
	}
	if category == "content_filtered" || category == "safety_refusal" {
		decision.TerminalReason = "safety_refusal"
		decision.UserMessageKey = "ai.run.terminal.safety_refusal"
		decision.SafeSummary = "request_refused"
		if in.HasSafeRefusalText {
			decision.Status = RunCompleted
			decision.CompletionReason = "safety_refusal"
			decision.Mode = FinalizationNone
		}
		return decision
	}
	if securityFailureCategories[category] {
		decision.TerminalReason = "security_boundary"
		decision.UserMessageKey = "ai.run.terminal.authorization_changed"
		decision.SafeSummary = "authorization_or_security_changed"
		return decision
	}
	if category == "authentication_failed" || category == "model_not_found" || category == "capability_not_supported" {
		decision.TerminalReason = "model_unavailable"
		decision.UserMessageKey = "ai.run.terminal.model_unavailable"
		decision.SafeSummary = "model_or_capability_unavailable"
		return decision
	}
	if providerFailureCategories[category] {
		decision.TerminalReason = "provider_failure"
		decision.Retryable = true
		decision.UserMessageKey = "ai.run.terminal.provider_unavailable"
		decision.SafeSummary = "model_service_unavailable"
		return decision
	}
	if strings.Contains(category, "budget") || category == "step_budget_exhausted" || category == "token_budget_exhausted" {
		decision.TerminalReason = "budget_limit"
		decision.UserMessageKey = "ai.run.terminal.budget_limit"
		decision.SafeSummary = "run_budget_exhausted"
		return decision
	}
	if strings.HasPrefix(category, "document_") || strings.Contains(category, "context") || category == "input_too_large" || category == "context_length_exceeded" {
		decision.TerminalReason = "context_unavailable"
		decision.Retryable = true
		decision.UserMessageKey = "ai.run.terminal.context_unavailable"
		decision.SafeSummary = "context_could_not_be_processed"
	}
	if category == "finalize_failed" || category == "conversation_unavailable" || category == "tenant_context_unavailable" {
		decision.TerminalReason = "persistence_unavailable"
		decision.Mode = FinalizationNoticeOnly
		decision.Retryable = true
		decision.UserMessageKey = "ai.run.terminal.persistence_unavailable"
		decision.SafeSummary = "terminal_result_could_not_be_saved"
		return decision
	}
	if modelFinalizationCategories[category] && in.CanUseModel {
		decision.Mode = FinalizationModel
		decision.Retryable = true
		decision.UserMessageKey = "ai.run.terminal.operation_failed"
		decision.SafeSummary = "operation_could_not_be_completed"
		if in.CompletedOperations > 0 {
			decision.TerminalReason = "partial_result"
			decision.SafeSummary = "partial_result_available"
		}
		return decision
	}
	if category == "tool_failed" || category == "business_validation_failed" || category == "resource_not_found" {
		decision.Retryable = true
		decision.UserMessageKey = "ai.run.terminal.operation_failed"
		decision.SafeSummary = "operation_could_not_be_completed"
	}
	return decision
}

func BuildFailureEnvelope(decision TerminalDecision, operations TerminalOperationSummary, locale string) FailureEnvelope {
	operations.Completed = boundedOperationCount(operations.Completed)
	operations.Failed = boundedOperationCount(operations.Failed)
	operations.Unfinished = boundedOperationCount(operations.Unfinished)
	locale = strings.TrimSpace(locale)
	switch strings.ToLower(locale) {
	case "zh-cn":
		locale = "zh-CN"
	case "en-us":
		locale = "en-US"
	default:
		locale = ""
	}
	nextAction := "contact_administrator"
	if decision.Retryable {
		nextAction = "retry"
	}
	return FailureEnvelope{
		SchemaVersion:        TerminalProtocolVersion,
		Category:             decision.TerminalReason,
		SafeSummary:          decision.SafeSummary,
		Retryable:            decision.Retryable,
		NextAction:           nextAction,
		Locale:               locale,
		CompletedOperations:  operations.Completed,
		FailedOperations:     operations.Failed,
		UnfinishedOperations: operations.Unfinished,
	}
}

func boundedOperationCount(value int) int {
	if value < 0 {
		return 0
	}
	if value > 100 {
		return 100
	}
	return value
}
