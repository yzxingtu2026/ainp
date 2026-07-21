package navigation

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/aitool"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/menu"
)

const ToolID = "builtin:navigation.search"

type SearchInput struct {
	Query string `json:"query" validate:"required,max=200" description:"Natural-language description of the page or feature the user wants to find."`
	Limit int    `json:"limit,omitempty" validate:"omitempty,min=1,max=5" description:"Maximum navigation results, from 1 to 5."`
}

type Slot struct{ search *menu.SearchService }

func New(registry *menu.Registry) *Slot { return &Slot{search: menu.NewSearchService(registry)} }

func (*Slot) Definition() aitool.Definition {
	return aitool.Definition{
		ID: ToolID, Source: aitool.SourceBuiltin, Name: "navigation.search", TitleForHuman: "功能导航搜索",
		DescriptionForLLM:   "Find authorized application pages by user intent. Return only trusted registry paths. Use this directly when the user asks where a feature is or how to enter a page; never invent or modify a returned path.",
		DescriptionForHuman: "搜索当前用户可访问的系统功能页面", Capability: "ai",
		InputSchema: action.SchemaFor[SearchInput](), Risk: aitool.RiskLow, SideEffect: aitool.SideEffectRead,
		BindingMode: aitool.BindingForbidden, LoadingMode: aitool.LoadingDirect,
		RuntimeRef: "navigation.search", SchemaVersion: 1, Status: "enabled",
	}
}

func (*Slot) DiscoveryMode() aitool.DiscoveryMode { return aitool.DiscoveryDirect }

func (*Slot) Metadata() aitool.RuntimeMetadata {
	return aitool.RuntimeMetadata{
		StepType: "tool_search", InvocationClass: "navigation_retrieval",
		BudgetCategory: "navigation", MaxCallsPerRun: 4,
		Observations:      []aitool.RuntimeObservation{{Field: "returned_count", Source: "result_data"}},
		FailureCategories: []string{"invalid_request", "navigation_budget_exhausted", "authorization_changed", "navigation_search_failed"},
	}
}

func (*Slot) Project(ctx context.Context, scope aitool.RuntimeScope, definition aitool.Definition) (aitool.Definition, bool, error) {
	ac := auth.MustFromContext(ctx)
	visible := ac.Authenticated && ac.TenantID != "" && ac.UserID != "" &&
		ac.TenantID == scope.TenantID && ac.UserID == scope.UserID && string(ac.Realm) == scope.Realm && ac.AuthVersion == scope.AuthVersion
	return definition, visible, nil
}

func (s *Slot) Execute(ctx context.Context, _ aitool.LoadedTool, arguments json.RawMessage) (*aitool.Result, error) {
	var input SearchInput
	if err := json.Unmarshal(arguments, &input); err != nil {
		return &aitool.Result{Status: "failed", Summary: "invalid navigation search request", ErrorCategory: "invalid_request"}, nil
	}
	ac := auth.MustFromContext(ctx)
	if !ac.Authenticated || ac.TenantID == "" || ac.UserID == "" {
		return &aitool.Result{Status: "failed", Summary: "authorization changed", ErrorCategory: "authorization_changed"}, nil
	}
	results, err := s.search.Search(menu.SearchOptions{
		Realm: ac.Realm, Language: ac.Language, HasPermission: ac.HasPermission,
		Query: input.Query, Limit: input.Limit, MaxLimit: menu.MaxToolSearchLimit,
	})
	if err != nil {
		return &aitool.Result{Status: "failed", Summary: fmt.Sprintf("navigation search failed: %v", err), ErrorCategory: "invalid_request"}, nil
	}
	return &aitool.Result{
		Status: "completed", Summary: "navigation search completed",
		Data: map[string]any{"items": results, "returned_count": len(results)},
	}, nil
}
