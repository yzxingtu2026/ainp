package aitool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
)

type ActionProvider struct{ Actions *action.Registry }

func (p *ActionProvider) Source() Source { return SourceAction }
func (p *ActionProvider) List(context.Context) ([]Definition, error) {
	if p.Actions == nil {
		return nil, fmt.Errorf("action registry is required")
	}
	if err := p.Actions.ValidateDynamicReferences(); err != nil {
		return nil, err
	}
	out := []Definition{}
	for _, a := range p.Actions.List() {
		if a.Meta.AI == nil || !a.Meta.AI.ExposeTool {
			continue
		}
		out = append(out, actionDefinition(a))
	}
	return out, nil
}
func (p *ActionProvider) Get(_ context.Context, id string) (*Definition, error) {
	actionID := strings.TrimPrefix(id, "action:")
	a, ok := p.Actions.Get(actionID)
	if !ok || a.Meta.AI == nil || !a.Meta.AI.ExposeTool {
		return nil, fmt.Errorf("tool not found")
	}
	d := actionDefinition(a)
	return &d, nil
}
func actionDefinition(a action.Action) Definition {
	m := a.Meta.AI
	permission := []string{}
	if a.Meta.AccessMode() == action.AccessPermission {
		permission = []string{a.Meta.PermissionCode(a.ID)}
	}
	refs := make([]DynamicReference, 0, len(m.DynamicReferences))
	for _, ref := range m.DynamicReferences {
		refs = append(refs, DynamicReference{Field: ref.Field, ResolverID: ref.ResolverID, DiscoveryToolID: "action:" + ref.DiscoveryAction, RequireEvidence: ref.RequireEvidence, MaxInvalidValues: ref.MaxInvalidValues, MaxInvalidBytes: ref.MaxInvalidBytes})
	}
	return Definition{ID: "action:" + a.ID, Source: SourceAction, Name: a.Meta.Name, TitleForHuman: strings.TrimSpace(m.ToolTitle), DescriptionForLLM: m.Summary, DescriptionForHuman: m.HumanDescription, ResultSummary: m.ResultSummary, Capability: a.Meta.Capability, Tags: append([]string(nil), m.Tags...), InputSchema: action.ToolInputSchema(m.InputSchema), OutputSchema: m.OutputSchema, Operation: m.Operation, Batch: cloneAIBatchMeta(m.Batch), Risk: Risk(m.Risk), SideEffect: SideEffect(m.SideEffect), RequiresConfirmation: m.RequiresConfirmation, BindingMode: BindingForbidden, LoadingMode: LoadingProgressive, PermissionRefs: permission, Realm: auth.NormalizeRealm(a.Meta.Surface), PrincipalKinds: append([]auth.PrincipalKind(nil), a.Meta.PrincipalKinds...), ClientSurfaces: append([]string(nil), a.Meta.ClientSurfaces...), RuntimeRef: a.ID, SchemaVersion: 1, Status: "enabled", DynamicReferences: refs}
}

func cloneAIBatchMeta(meta *action.AIBatchMeta) *action.AIBatchMeta {
	if meta == nil {
		return nil
	}
	clone := *meta
	return &clone
}

type ActionExecutor struct{ Actions *action.Registry }

func (e *ActionExecutor) ResolveReference(ctx context.Context, id string) (action.ReferenceSnapshot, error) {
	if e.Actions == nil {
		return action.ReferenceSnapshot{}, fmt.Errorf("action registry is required")
	}
	return e.Actions.ResolveReference(ctx, id)
}

func (e *ActionExecutor) Execute(ctx context.Context, tool LoadedTool, args json.RawMessage) (*Result, error) {
	if e.Actions == nil {
		return nil, fmt.Errorf("action registry is required")
	}
	data, err := e.Actions.Invoke(ctx, tool.Definition.RuntimeRef, args)
	if err != nil {
		return nil, err
	}
	return &Result{Status: "completed", Summary: tool.Definition.ResultSummary, Data: data}, nil
}
