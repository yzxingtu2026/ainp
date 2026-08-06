package host

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/prompt"
	"github.com/yzxingtu2026/ainp/platform/modules/ai"
)

func TestAgentToolCapabilityCoverageDoesNotExcludeUnexposedDemo(t *testing.T) {
	if err := prompt.LoadDir("../prompts"); err != nil {
		t.Fatal(err)
	}
	contracts, err := ai.NewBuiltinAgentContractRegistry()
	if err != nil {
		t.Fatal(err)
	}
	application := &app.App{Actions: action.NewRegistry(nil, nil), AgentContracts: contracts}
	handler := func(context.Context, json.RawMessage) (any, error) { return nil, nil }
	if err := application.Actions.Register(action.Action{
		ID:      "system.covered",
		Handler: handler,
		Meta: action.Metadata{
			Capability: "system",
			AI: &action.AIMeta{
				ExposeTool:       true,
				ToolTitle:        "系统查询",
				Summary:          "查询系统信息",
				HumanDescription: "查询系统信息",
				ResultSummary:    "已返回系统信息",
				Risk:             "low",
				SideEffect:       "read",
				InputSchema:      action.InputSchemaFor[struct{}](),
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := application.Actions.Register(action.Action{
		ID:      "demo.unexposed",
		Handler: handler,
		Meta:    action.Metadata{Capability: "demo"},
	}); err != nil {
		t.Fatal(err)
	}

	if err := validateAgentToolCapabilityCoverage(application); err != nil {
		t.Fatalf("boot rejected unexposed demo capability: %v", err)
	}

	if err := application.Actions.Register(action.Action{
		ID:      "foundation.missing",
		Handler: handler,
		Meta: action.Metadata{
			Capability: "foundation",
			AI: &action.AIMeta{
				ExposeTool:       true,
				ToolTitle:        "企业资料",
				Summary:          "查询企业资料",
				HumanDescription: "查询企业资料",
				ResultSummary:    "已返回企业资料",
				Risk:             "low",
				SideEffect:       "read",
				InputSchema:      action.InputSchemaFor[struct{}](),
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := validateAgentToolCapabilityCoverage(application); err == nil || !strings.Contains(err.Error(), "missing=[foundation]") {
		t.Fatalf("boot must keep missing capability fail-closed: %v", err)
	}
}
