package aitool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
)

type ActionProvider struct{ Actions *action.Registry }

func (p *ActionProvider) Source() Source { return SourceAction }
func (p *ActionProvider) List(context.Context) ([]Definition, error) {
	if p.Actions == nil {
		return nil, fmt.Errorf("action registry is required")
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
	return Definition{ID: "action:" + a.ID, Source: SourceAction, Name: a.Meta.Name, TitleForHuman: strings.TrimSpace(m.ToolTitle), DescriptionForLLM: m.Summary, DescriptionForHuman: m.HumanDescription, ResultSummary: m.ResultSummary, Capability: a.Meta.Capability, Tags: append([]string(nil), m.Tags...), InputSchema: action.ToolInputSchema(m.InputSchema), OutputSchema: m.OutputSchema, Risk: Risk(m.Risk), SideEffect: SideEffect(m.SideEffect), RequiresConfirmation: m.RequiresConfirmation, BindingMode: BindingForbidden, LoadingMode: LoadingProgressive, PermissionRefs: permission, RuntimeRef: a.ID, SchemaVersion: 1, Status: "enabled"}
}

type ActionExecutor struct{ Actions *action.Registry }

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
