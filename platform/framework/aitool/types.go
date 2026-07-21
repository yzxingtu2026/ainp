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
type BindingMode string
type LoadingMode string

const (
	SettingKeySearch                   = "ai.tool_search"
	SettingFieldModelAlias             = "embedding_model_alias"
	SourceMeta             Source      = "meta"
	SourceAction           Source      = "action"
	SourceBuiltin          Source      = "builtin"
	SourcePlugin           Source      = "plugin"
	SourceHTTP             Source      = "http"
	SourceMCP              Source      = "mcp"
	RiskLow                Risk        = "low"
	RiskMedium             Risk        = "medium"
	RiskHigh               Risk        = "high"
	RiskCritical           Risk        = "critical"
	SideEffectNone         SideEffect  = "none"
	SideEffectRead         SideEffect  = "read"
	SideEffectWrite        SideEffect  = "write"
	SideEffectExternal     SideEffect  = "external"
	SideEffectPaid         SideEffect  = "paid"
	SideEffectDestructive  SideEffect  = "destructive"
	BindingForbidden       BindingMode = "forbidden"
	BindingOptional        BindingMode = "optional"
	LoadingDirect          LoadingMode = "direct"
	LoadingConditional     LoadingMode = "conditional"
	LoadingProgressive     LoadingMode = "progressive"
)

var toolIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]*:[a-z][a-z0-9_.-]*$`)
var observationFieldPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

type Definition struct {
	ID                   string          `json:"id"`
	Source               Source          `json:"source"`
	Name                 string          `json:"name"`
	TitleForHuman        string          `json:"title_for_human"`
	DescriptionForLLM    string          `json:"description_for_llm"`
	DescriptionForHuman  string          `json:"description_for_human"`
	ResultSummary        string          `json:"result_summary"`
	Capability           string          `json:"capability"`
	Tags                 []string        `json:"tags,omitempty"`
	InputSchema          json.RawMessage `json:"input_schema"`
	OutputSchema         json.RawMessage `json:"output_schema,omitempty"`
	Risk                 Risk            `json:"risk"`
	SideEffect           SideEffect      `json:"side_effect"`
	RequiresConfirmation bool            `json:"requires_confirmation"`
	BindingMode          BindingMode     `json:"binding_mode"`
	LoadingMode          LoadingMode     `json:"loading_mode"`
	BindingGroup         string          `json:"binding_group,omitempty"`
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
	if d.Name == "" || d.TitleForHuman == "" || d.DescriptionForLLM == "" || d.DescriptionForHuman == "" || d.Capability == "" || d.RuntimeRef == "" {
		return fmt.Errorf("tool definition fields are required")
	}
	if !json.Valid(d.InputSchema) || (len(d.OutputSchema) > 0 && !json.Valid(d.OutputSchema)) {
		return fmt.Errorf("invalid tool schema")
	}
	if !slices.Contains([]Risk{RiskLow, RiskMedium, RiskHigh, RiskCritical}, d.Risk) || !slices.Contains([]SideEffect{SideEffectNone, SideEffectRead, SideEffectWrite, SideEffectExternal, SideEffectPaid, SideEffectDestructive}, d.SideEffect) {
		return fmt.Errorf("invalid risk or side effect")
	}
	if !slices.Contains([]BindingMode{BindingForbidden, BindingOptional}, d.BindingMode) || !slices.Contains([]LoadingMode{LoadingDirect, LoadingConditional, LoadingProgressive}, d.LoadingMode) {
		return fmt.Errorf("invalid binding or loading mode")
	}
	if d.BindingGroup != "" && !observationFieldPattern.MatchString(d.BindingGroup) {
		return fmt.Errorf("invalid binding group")
	}
	if d.SchemaVersion < 1 || (d.Status != "enabled" && d.Status != "disabled") {
		return fmt.Errorf("invalid schema version or status")
	}
	return nil
}

type Candidate struct {
	ID           string      `json:"id"`
	Name         string      `json:"name"`
	Summary      string      `json:"summary"`
	Source       Source      `json:"source"`
	Capability   string      `json:"capability"`
	Risk         Risk        `json:"risk"`
	SideEffect   SideEffect  `json:"side_effect"`
	InputHints   []string    `json:"input_hints"`
	BindingMode  BindingMode `json:"binding_mode"`
	LoadingMode  LoadingMode `json:"loading_mode"`
	BindingGroup string      `json:"binding_group,omitempty"`
	Score        float64     `json:"score"`
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
type DiscoveryMode string

const (
	DiscoveryProgressive DiscoveryMode = "progressive"
	DiscoveryDirect      DiscoveryMode = "direct"
)

// RuntimeScope only contains trusted runtime identifiers. Slots must not derive
// tenant, actor, run, or authorization facts from model arguments.
type RuntimeScope struct {
	ID          string
	TenantID    string
	UserID      string
	Realm       string
	SessionID   string
	AuthVersion uint64
	AgentRunID  string
}

type RuntimeMetadata struct {
	StepType          string
	InvocationClass   string
	BudgetCategory    string
	MaxCallsPerRun    int
	Observations      []RuntimeObservation
	FailureCategories []string
}

type RuntimeObservation struct {
	Field  string
	Source string
}

func (m RuntimeMetadata) Validate() error {
	if !slices.Contains([]string{"tool_search", "tool_load", "tool_execute"}, m.StepType) || strings.TrimSpace(m.InvocationClass) == "" || m.MaxCallsPerRun < 0 {
		return fmt.Errorf("invalid runtime metadata")
	}
	if m.MaxCallsPerRun > 0 && strings.TrimSpace(m.BudgetCategory) == "" {
		return fmt.Errorf("budget category is required")
	}
	seen := map[string]bool{}
	for _, observation := range m.Observations {
		if !observationFieldPattern.MatchString(observation.Field) || seen[observation.Field] || !slices.Contains([]string{"result_data", "resource_ref_count"}, observation.Source) {
			return fmt.Errorf("invalid observation field")
		}
		seen[observation.Field] = true
	}
	seen = map[string]bool{}
	for _, category := range m.FailureCategories {
		if !observationFieldPattern.MatchString(category) || seen[category] {
			return fmt.Errorf("invalid failure category")
		}
		seen[category] = true
	}
	return nil
}

type RuntimeSlot interface {
	Definition() Definition
	DiscoveryMode() DiscoveryMode
	Project(context.Context, RuntimeScope, Definition) (Definition, bool, error)
	Metadata() RuntimeMetadata
	Execute(context.Context, LoadedTool, json.RawMessage) (*Result, error)
}

type MetaSearchInput struct {
	Query      string `json:"query" validate:"required,max=512" description:"Natural-language search intent. Put business terms here."`
	Capability string `json:"capability,omitempty" description:"Optional exact capability code such as system or operations. Omit when unknown; do not use natural language."`
	Source     Source `json:"source,omitempty" validate:"omitempty,oneof=meta action builtin plugin http mcp" description:"Optional exact tool source. Use action for governed application Actions; omit when unknown."`
	Limit      int    `json:"limit,omitempty" validate:"omitempty,min=1,max=10" description:"Maximum candidates to return, from 1 to 10."`
}
type MetaLoadInput struct {
	ToolIDs []string `json:"tool_ids" validate:"required,min=1,max=3"`
}

type KnowledgeSearchInput struct {
	Query            string   `json:"query" validate:"required,max=4096"`
	KnowledgeBaseIDs []string `json:"knowledge_base_ids,omitempty" validate:"omitempty,max=16"`
	DocumentIDs      []string `json:"document_ids,omitempty" validate:"omitempty,max=100"`
	Limit            int      `json:"limit,omitempty"`
}
type MemorySearchInput struct {
	Query    string   `json:"query" validate:"required,max=4096"`
	DateFrom string   `json:"date_from,omitempty"`
	DateTo   string   `json:"date_to,omitempty"`
	Types    []string `json:"types,omitempty" validate:"omitempty,max=4"`
	Limit    int      `json:"limit,omitempty" validate:"omitempty,min=1,max=20"`
}

type scopeContextKey struct{}

func WithScopeID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, scopeContextKey{}, id)
}
func ScopeIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(scopeContextKey{}).(string)
	return v
}
