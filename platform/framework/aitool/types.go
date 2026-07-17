package aitool

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

type Source string
type Risk string
type SideEffect string

const (
	SourceMeta            Source     = "meta"
	SourceAction          Source     = "action"
	SourceBuiltin         Source     = "builtin"
	SourcePlugin          Source     = "plugin"
	SourceHTTP            Source     = "http"
	SourceMCP             Source     = "mcp"
	RiskLow               Risk       = "low"
	RiskMedium            Risk       = "medium"
	RiskHigh              Risk       = "high"
	RiskCritical          Risk       = "critical"
	SideEffectNone        SideEffect = "none"
	SideEffectRead        SideEffect = "read"
	SideEffectWrite       SideEffect = "write"
	SideEffectExternal    SideEffect = "external"
	SideEffectPaid        SideEffect = "paid"
	SideEffectDestructive SideEffect = "destructive"
)

var toolIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]*:[a-z][a-z0-9_.-]*$`)

type Definition struct {
	ID                   string          `json:"id"`
	Source               Source          `json:"source"`
	Name                 string          `json:"name"`
	DescriptionForLLM    string          `json:"description_for_llm"`
	DescriptionForHuman  string          `json:"description_for_human"`
	Capability           string          `json:"capability"`
	Tags                 []string        `json:"tags,omitempty"`
	InputSchema          json.RawMessage `json:"input_schema"`
	OutputSchema         json.RawMessage `json:"output_schema,omitempty"`
	Risk                 Risk            `json:"risk"`
	SideEffect           SideEffect      `json:"side_effect"`
	RequiresConfirmation bool            `json:"requires_confirmation"`
	PermissionRefs       []string        `json:"permission_refs,omitempty"`
	RuntimeRef           string          `json:"-"`
	SchemaVersion        int             `json:"schema_version"`
	SchemaHash           string          `json:"schema_hash"`
	Status               string          `json:"status"`
}

func (d Definition) Validate() error {
	if !slices.Contains([]Source{SourceMeta, SourceAction, SourceBuiltin, SourcePlugin, SourceHTTP, SourceMCP}, d.Source) {
		return fmt.Errorf("unknown tool source")
	}
	if !toolIDPattern.MatchString(d.ID) || !strings.HasPrefix(d.ID, string(d.Source)+":") {
		return fmt.Errorf("invalid namespaced tool id")
	}
	if d.Name == "" || d.DescriptionForLLM == "" || d.DescriptionForHuman == "" || d.Capability == "" || d.RuntimeRef == "" {
		return fmt.Errorf("tool definition fields are required")
	}
	if !json.Valid(d.InputSchema) || (len(d.OutputSchema) > 0 && !json.Valid(d.OutputSchema)) {
		return fmt.Errorf("invalid tool schema")
	}
	if !slices.Contains([]Risk{RiskLow, RiskMedium, RiskHigh, RiskCritical}, d.Risk) || !slices.Contains([]SideEffect{SideEffectNone, SideEffectRead, SideEffectWrite, SideEffectExternal, SideEffectPaid, SideEffectDestructive}, d.SideEffect) {
		return fmt.Errorf("invalid risk or side effect")
	}
	if d.SchemaVersion < 1 || (d.Status != "enabled" && d.Status != "disabled") {
		return fmt.Errorf("invalid schema version or status")
	}
	return nil
}

type Candidate struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Summary    string     `json:"summary"`
	Source     Source     `json:"source"`
	Capability string     `json:"capability"`
	Risk       Risk       `json:"risk"`
	SideEffect SideEffect `json:"side_effect"`
	InputHints []string   `json:"input_hints"`
	Score      float64    `json:"score"`
}
type LoadedTool struct {
	Definition Definition `json:"definition"`
	Handle     string     `json:"handle"`
	ExpiresAt  int64      `json:"expires_at"`
}
type Result struct {
	CallID        string   `json:"call_id"`
	Status        string   `json:"status"`
	Summary       string   `json:"summary"`
	Data          any      `json:"data,omitempty"`
	ResourceRefs  []string `json:"resource_refs,omitempty"`
	Truncated     bool     `json:"truncated"`
	ErrorCategory string   `json:"error_category,omitempty"`
}

type Provider interface {
	Source() Source
	List(context.Context) ([]Definition, error)
	Get(context.Context, string) (*Definition, error)
}
type Executor interface {
	Execute(context.Context, LoadedTool, json.RawMessage) (*Result, error)
}
type EmbeddingService interface {
	Scores(context.Context, string, []Definition) (map[string]float64, error)
}

type MetaSearchInput struct {
	Query      string `json:"query" validate:"required,max=512"`
	Capability string `json:"capability,omitempty"`
	Source     Source `json:"source,omitempty"`
	Limit      int    `json:"limit,omitempty"`
}
type MetaLoadInput struct {
	ToolIDs []string `json:"tool_ids" validate:"required,min=1,max=3"`
}

type scopeContextKey struct{}

func WithScopeID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, scopeContextKey{}, id)
}
func ScopeIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(scopeContextKey{}).(string)
	return v
}
