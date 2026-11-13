package pagecapture

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodel"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodelruntime"
	"github.com/yzxingtu2026/ainp/platform/framework/aitool"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
)

const (
	ToolID                 = "builtin:page.capture"
	AttachmentResourceType = "ai.page_capture"
	AttachmentField        = "capture"
	FulfillmentKind        = "page.capture"
	DefaultMaxCallsPerRun  = 3
	DefaultTimeoutSeconds  = 30
)

type CaptureInput struct {
	Reason string `json:"reason,omitempty" validate:"omitempty,max=240" description:"Optional brief note on what aspect of the current page to inspect for guiding the user."`
}

type ModelAliasLookupFunc func(ctx context.Context, runID string) (string, error)

type Slot struct {
	models      aimodel.Resolver
	lookupAlias ModelAliasLookupFunc
}

func New(models aimodel.Resolver, lookupAlias ModelAliasLookupFunc) *Slot {
	return &Slot{models: models, lookupAlias: lookupAlias}
}

func (*Slot) Definition() aitool.Definition {
	return aitool.Definition{
		ID:                  ToolID,
		Source:              aitool.SourceBuiltin,
		Name:                "page.capture",
		TitleForHuman:       "查看当前页面",
		DescriptionForLLM:   "Capture the current web console main content area (excluding the AI assistant panel itself) as an image so you can see open dialogs, visible validation errors, wizard steps, and live UI state to guide the user on the current page. Call this when the user asks about or needs help operating the current page and existing context is insufficient.",
		DescriptionForHuman: "截取当前业务主内容区画面（不含 AI 助手面板），辅助操作引导",
		ResultSummary:       "已捕获当前业务页面画面",
		Capability:          "ai",
		Tags:                []string{"page", "capture", "screenshot", "guide", "页面", "当前页面", "引导"},
		InputSchema:         action.SchemaFor[CaptureInput](),
		Risk:                aitool.RiskLow,
		SideEffect:          aitool.SideEffectRead,
		BindingMode:         aitool.BindingForbidden,
		LoadingMode:         aitool.LoadingDirect,
		ClientSurfaces:      []string{auth.TenantConsoleSurface},
		RuntimeRef:          "page.capture",
		SchemaVersion:       1,
		Status:              "enabled",
	}
}

func (*Slot) DiscoveryMode() aitool.DiscoveryMode { return aitool.DiscoveryDirect }

func (*Slot) Metadata() aitool.RuntimeMetadata {
	return aitool.RuntimeMetadata{
		StepType:                 "tool_execute",
		InvocationClass:          "page_capture",
		BudgetCategory:           "page_capture",
		MaxCallsPerRun:           DefaultMaxCallsPerRun,
		ClientFulfillment:        true,
		FulfillmentTimeoutSecond: DefaultTimeoutSeconds,
		Observations: []aitool.RuntimeObservation{
			{Field: "capture_status", Source: "result_data"},
		},
		FailureCategories: []string{
			"invalid_request",
			"page_capture_budget_exhausted",
			"authorization_changed",
			"capability_not_supported",
			"capture_timeout",
			"capture_failed",
			"capture_cancelled",
			"attachment_unavailable",
		},
	}
}

func (s *Slot) Project(ctx context.Context, scope aitool.RuntimeScope, definition aitool.Definition) (aitool.Definition, bool, error) {
	ac := action.NormalizeActor(auth.MustFromContext(ctx))
	if !ac.Authenticated || ac.TenantID == "" || ac.PrincipalID == "" {
		return definition, false, nil
	}
	if ac.TenantID != scope.TenantID || ac.PrincipalID != scope.PrincipalID || string(auth.NormalizeRealm(ac.Realm)) != scope.Realm || ac.AuthVersion != scope.AuthVersion {
		return definition, false, nil
	}
	surface := strings.TrimSpace(scope.ClientSurface)
	if surface == "" {
		surface = ac.ClientSurface
	}
	if surface != auth.TenantConsoleSurface || ac.ClientSurface != auth.TenantConsoleSurface {
		return definition, false, nil
	}
	if !s.supportsVision(ctx, scope) {
		return definition, false, nil
	}
	return definition, true, nil
}

func (s *Slot) supportsVision(ctx context.Context, scope aitool.RuntimeScope) bool {
	if s.models == nil {
		return false
	}
	alias := aimodelruntime.ModelAliasFromContext(ctx)
	if alias == "" && scope.AgentRunID != "" && s.lookupAlias != nil {
		resolvedAlias, err := s.lookupAlias(ctx, scope.AgentRunID)
		if err != nil {
			return false
		}
		alias = strings.TrimSpace(resolvedAlias)
	}
	if alias == "" {
		return false
	}
	_, err := s.models.Resolve(ctx, alias, VisionModelRequirements())
	return err == nil
}

func VisionModelRequirements() aimodel.CapabilityRequirements {
	return aimodel.CapabilityRequirements{
		ModelType:        aimodel.ModelTypeChat,
		InputModalities:  []aimodel.Modality{aimodel.ModalityText, aimodel.ModalityImage},
		OutputModalities: []aimodel.Modality{aimodel.ModalityText},
		Features:         []aimodel.Feature{aimodel.FeatureVision},
	}
}

func (s *Slot) Execute(ctx context.Context, _ aitool.LoadedTool, arguments json.RawMessage) (*aitool.Result, error) {
	var input CaptureInput
	if len(arguments) > 0 && string(arguments) != "{}" {
		if err := json.Unmarshal(arguments, &input); err != nil {
			return &aitool.Result{Status: aitool.ResultStatusFailed, Summary: "invalid page capture request", ErrorCategory: "invalid_request"}, nil
		}
	}
	reason := strings.TrimSpace(input.Reason)
	if len([]rune(reason)) > 240 {
		return &aitool.Result{Status: aitool.ResultStatusFailed, Summary: "invalid page capture request", ErrorCategory: "invalid_request"}, nil
	}
	ac := action.NormalizeActor(auth.MustFromContext(ctx))
	if !ac.Authenticated || ac.TenantID == "" || ac.PrincipalID == "" || ac.ClientSurface != auth.TenantConsoleSurface {
		return &aitool.Result{Status: aitool.ResultStatusFailed, Summary: "authorization changed", ErrorCategory: "authorization_changed"}, nil
	}
	scopeID := aitool.ScopeIDFromContext(ctx)
	if scopeID == "" {
		return &aitool.Result{Status: aitool.ResultStatusFailed, Summary: "page capture requires an active agent run", ErrorCategory: "invalid_request"}, nil
	}
	if s.models != nil {
		alias := aimodelruntime.ModelAliasFromContext(ctx)
		if alias != "" {
			if _, err := s.models.Resolve(ctx, alias, VisionModelRequirements()); err != nil {
				return &aitool.Result{Status: aitool.ResultStatusFailed, Summary: "current model does not support vision", ErrorCategory: "capability_not_supported"}, nil
			}
		}
	}
	params := map[string]any{}
	if reason != "" {
		params["reason"] = reason
	}
	return &aitool.Result{
		Status:  aitool.ResultStatusClientFulfillmentRequired,
		Summary: "client page capture is required",
		Data: aitool.ClientFulfillmentRequest{
			Kind:       FulfillmentKind,
			Parameters: params,
		},
	}, nil
}
