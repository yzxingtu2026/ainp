package aitool

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const ConfirmationPresentationVersion = 1
const ConfirmationBatchItemLimit = 10

type ToolPresentation struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
}

type ConfirmationPresentation struct {
	Tool    ToolPresentation                 `json:"tool"`
	Summary string                           `json:"summary"`
	Details []ConfirmationPresentationDetail `json:"details,omitempty"`
}

type ConfirmationPresentationDetail struct {
	Label     string `json:"label"`
	Value     string `json:"value"`
	Sensitive bool   `json:"sensitive,omitempty"`
}

type ConfirmationPresentationRequest struct {
	Definition Definition
	Arguments  json.RawMessage
}

type ConfirmationBatchSummary struct {
	Total   int
	Items   []string
	Omitted int
}

// SummarizeBatchForConfirmation applies the shared confirmation-card item
// budget. Domain presenters remain responsible for safe, localized labels.
func SummarizeBatchForConfirmation(items []string, limit int) ConfirmationBatchSummary {
	if limit <= 0 || limit > ConfirmationBatchItemLimit {
		limit = ConfirmationBatchItemLimit
	}
	shown := len(items)
	if shown > limit {
		shown = limit
	}
	return ConfirmationBatchSummary{
		Total:   len(items),
		Items:   append([]string(nil), items[:shown]...),
		Omitted: len(items) - shown,
	}
}

type ConfirmationPresenter interface {
	PresentConfirmation(context.Context, ConfirmationPresentationRequest) (ConfirmationPresentation, error)
}

type ConfirmationPresenterFunc func(context.Context, ConfirmationPresentationRequest) (ConfirmationPresentation, error)

func (f ConfirmationPresenterFunc) PresentConfirmation(ctx context.Context, request ConfirmationPresentationRequest) (ConfirmationPresentation, error) {
	return f(ctx, request)
}

func PresentationFor(d Definition) ToolPresentation {
	return ToolPresentation{Title: d.TitleForHuman, Description: d.DescriptionForHuman}
}

func (r *Registry) RegisterConfirmationPresenter(toolID string, presenter ConfirmationPresenter) error {
	toolID = strings.TrimSpace(toolID)
	if !toolIDPattern.MatchString(toolID) || presenter == nil {
		return fmt.Errorf("valid tool id and confirmation presenter are required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.confirmationPresenters[toolID]; exists {
		return fmt.Errorf("confirmation presenter already registered: %s", toolID)
	}
	r.confirmationPresenters[toolID] = presenter
	return nil
}

// PresentConfirmation always returns a trusted, safe presentation. Domain
// presenter failures intentionally fall back without exposing raw arguments.
func (r *Registry) PresentConfirmation(ctx context.Context, d Definition, arguments json.RawMessage) ConfirmationPresentation {
	r.mu.RLock()
	presenter := r.confirmationPresenters[d.ID]
	r.mu.RUnlock()
	request := ConfirmationPresentationRequest{Definition: d, Arguments: append(json.RawMessage(nil), arguments...)}
	if presenter != nil {
		if presentation, err := presenter.PresentConfirmation(ctx, request); err == nil && validConfirmationPresentation(presentation) {
			return presentation
		}
	}
	return defaultConfirmationPresentation(request)
}

func defaultConfirmationPresentation(request ConfirmationPresentationRequest) ConfirmationPresentation {
	presentation := ConfirmationPresentation{
		Tool:    PresentationFor(request.Definition),
		Summary: fmt.Sprintf("请求执行「%s」", request.Definition.TitleForHuman),
	}
	var arguments map[string]any
	var schema struct {
		Properties map[string]struct {
			Title       string `json:"title"`
			Description string `json:"description"`
			Format      string `json:"format"`
			WriteOnly   bool   `json:"writeOnly"`
		} `json:"properties"`
	}
	if json.Unmarshal(request.Arguments, &arguments) != nil || json.Unmarshal(request.Definition.InputSchema, &schema) != nil {
		return presentation
	}
	keys := make([]string, 0, len(arguments))
	for key := range arguments {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		property, declared := schema.Properties[key]
		label := strings.TrimSpace(property.Title)
		if !declared || label == "" || sensitivePresentationField(key, property.Format, property.WriteOnly) {
			continue
		}
		value, ok := safePresentationScalar(arguments[key])
		if !ok {
			continue
		}
		presentation.Details = append(presentation.Details, ConfirmationPresentationDetail{Label: label, Value: value})
	}
	return presentation
}

func safePresentationScalar(value any) (string, bool) {
	switch typed := value.(type) {
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" || len([]rune(trimmed)) > 128 || looksLikeIdentifier(trimmed) {
			return "", false
		}
		return trimmed, true
	case bool:
		if typed {
			return "是", true
		}
		return "否", true
	case float64:
		return fmt.Sprintf("%v", typed), true
	default:
		return "", false
	}
}

func sensitivePresentationField(name, format string, writeOnly bool) bool {
	if writeOnly || strings.EqualFold(format, "password") {
		return true
	}
	lower := strings.ToLower(name)
	for _, marker := range []string{"password", "secret", "token", "credential", "api_key", "private_key"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func looksLikeIdentifier(value string) bool {
	if len(value) == 36 && strings.Count(value, "-") == 4 {
		return true
	}
	return len(value) >= 24 && !strings.ContainsAny(value, " \t\n")
}

func validConfirmationPresentation(presentation ConfirmationPresentation) bool {
	return strings.TrimSpace(presentation.Tool.Title) != "" && strings.TrimSpace(presentation.Summary) != ""
}
