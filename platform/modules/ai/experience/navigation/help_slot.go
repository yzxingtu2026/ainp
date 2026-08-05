package navigation

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/aitool"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/menu"
)

const HelpToolID = "builtin:navigation.help"

type HelpInput struct {
	MenuID  string `json:"menu_id" validate:"required,max=128" description:"Stable menu_id returned by navigation.search."`
	UseCase string `json:"use_case,omitempty" validate:"omitempty,max=160" description:"Optional use case returned by the trusted help metadata."`
	Locale  string `json:"locale,omitempty" validate:"omitempty,max=16" description:"Preferred help locale."`
}

type HelpSlot struct {
	menus *menu.Registry
	help  *menu.HelpRegistry
}

func NewHelp(menus *menu.Registry, help *menu.HelpRegistry) *HelpSlot {
	return &HelpSlot{menus: menus, help: help}
}

func (*HelpSlot) Definition() aitool.Definition {
	return aitool.Definition{
		ID: HelpToolID, Source: aitool.SourceBuiltin, Name: "navigation.help", TitleForHuman: "页面操作帮助",
		DescriptionForLLM:   "Read reviewed, versioned operation help for an authorized menu_id returned by navigation.search. Use it for how-to questions. Never invent file paths, click controls, fill forms, submit changes, or execute actions.",
		DescriptionForHuman: "读取当前用户有权访问页面的操作说明", Capability: "ai", InputSchema: action.SchemaFor[HelpInput](), Risk: aitool.RiskLow, SideEffect: aitool.SideEffectRead,
		BindingMode: aitool.BindingForbidden, LoadingMode: aitool.LoadingDirect, RuntimeRef: "navigation.help", SchemaVersion: 1, Status: "enabled",
	}
}
func (*HelpSlot) DiscoveryMode() aitool.DiscoveryMode { return aitool.DiscoveryDirect }
func (*HelpSlot) Metadata() aitool.RuntimeMetadata {
	return aitool.RuntimeMetadata{StepType: "tool_search", InvocationClass: "navigation_help", BudgetCategory: "navigation", MaxCallsPerRun: 4, FailureCategories: []string{"invalid_request", "authorization_changed", "help_not_found"}}
}
func (*HelpSlot) Project(ctx context.Context, scope aitool.RuntimeScope, definition aitool.Definition) (aitool.Definition, bool, error) {
	ac := auth.MustFromContext(ctx)
	visible := ac.Authenticated && ac.TenantID != "" && ac.PrincipalID != "" && ac.TenantID == scope.TenantID && ac.PrincipalID == scope.PrincipalID && string(ac.Realm) == scope.Realm && ac.AuthVersion == scope.AuthVersion
	return definition, visible, nil
}
func (s *HelpSlot) Execute(ctx context.Context, _ aitool.LoadedTool, arguments json.RawMessage) (*aitool.Result, error) {
	var input HelpInput
	if json.Unmarshal(arguments, &input) != nil || strings.TrimSpace(input.MenuID) == "" || strings.ContainsAny(input.MenuID, "/\\") {
		return &aitool.Result{Status: "failed", Summary: "invalid navigation help request", ErrorCategory: "invalid_request"}, nil
	}
	ac := auth.MustFromContext(ctx)
	if !ac.Authenticated || ac.TenantID == "" || ac.PrincipalID == "" {
		return &aitool.Result{Status: "failed", Summary: "authorization changed", ErrorCategory: "authorization_changed"}, nil
	}
	visible := false
	for _, item := range s.menus.TreeForUser(ac.Realm, ac.HasPermission) {
		if item.ID == input.MenuID && item.Type == menu.TypeMenu {
			visible = true
			break
		}
	}
	if !visible {
		return &aitool.Result{Status: "failed", Summary: "page is no longer authorized", ErrorCategory: "authorization_changed"}, nil
	}
	locale := strings.TrimSpace(input.Locale)
	if locale == "" {
		locale = ac.Language
	}
	if locale == "" {
		locale = "zh-CN"
	}
	document, ok := s.help.Get(input.MenuID, locale)
	if !ok {
		return &aitool.Result{Status: "failed", Summary: "reviewed page help is unavailable", ErrorCategory: "help_not_found"}, nil
	}
	if input.UseCase != "" {
		matched := false
		for _, value := range document.UseCases {
			if value == input.UseCase {
				matched = true
				break
			}
		}
		if !matched {
			return &aitool.Result{Status: "failed", Summary: "requested use case is unavailable", ErrorCategory: "help_not_found"}, nil
		}
	}
	return &aitool.Result{Status: "completed", Summary: "page help loaded", Data: map[string]any{"menu_id": document.MenuID, "version": document.Version, "locale": document.Locale, "use_cases": document.UseCases, "content": document.Content, "citation": map[string]any{"source": document.Source, "version": document.Version}}}, nil
}
