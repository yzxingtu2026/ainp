package host

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/agentslot"
	"github.com/yzxingtu2026/ainp/platform/framework/aiagent"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/prompt"
	"github.com/yzxingtu2026/ainp/platform/modules/ai"
	"gorm.io/gorm"
)

type contributionModule struct {
	id            string
	contributions []aiagent.ContractContribution
	assets        RuntimeAssets
}

func (m contributionModule) ID() string                   { return m.id }
func (m contributionModule) Register(*app.App) error      { return nil }
func (m contributionModule) RuntimeAssets() RuntimeAssets { return m.assets }
func (m contributionModule) AgentContractContributions() []aiagent.ContractContribution {
	return m.contributions
}

func TestAgentContractContributionsAreDeterministicAndFailClosed(t *testing.T) {
	modules := []Module{
		contributionModule{id: "zeta", contributions: []aiagent.ContractContribution{{TargetKey: "system.operations", TargetVersion: "1.0.0", AllowedToolCapabilities: []string{"zeta", "shared"}}}},
		contributionModule{id: "alpha", contributions: []aiagent.ContractContribution{{TargetKey: "system.operations", TargetVersion: "1.0.0", AllowedToolCapabilities: []string{"alpha", "shared"}}}},
	}
	composition, err := New(WithModules(modules...))
	if err != nil {
		t.Fatal(err)
	}
	if err := prompt.LoadDir("../prompts"); err != nil {
		t.Fatal(err)
	}
	contracts, err := ai.NewBuiltinAgentContractRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if err := composition.applyAgentContractContributionsTo(contracts); err != nil {
		t.Fatal(err)
	}
	contract, _ := contracts.Get("system.operations", "1.0.0")
	for _, capability := range []string{"ai", "alpha", "operations", "shared", "system", "zeta"} {
		if !slices.Contains(contract.AllowedToolCapabilities, capability) {
			t.Fatalf("missing deterministic capability %s: %#v", capability, contract.AllowedToolCapabilities)
		}
	}

	badPrompt := fstest.MapFS{"other/instructions.md": {Data: []byte("---\nversion: 1.0.0\n---\nUnsafe.")}}
	if err := prompt.LoadSources(prompt.Source{Owner: "other", FS: badPrompt}); err != nil {
		t.Fatal(err)
	}
	tests := []aiagent.ContractContribution{
		{Owner: "alpha", TargetKey: "unknown.contract", TargetVersion: "1.0.0", AllowedToolCapabilities: []string{"alpha"}},
		{Owner: "alpha", TargetKey: "system.operations", TargetVersion: "1.0.0", BaseInstructionsPromptKey: "other/instructions", BaseInstructionsVersion: "1.0.0"},
		{Owner: "other", TargetKey: "system.operations", TargetVersion: "1.0.0", BaseInstructionsPromptKey: "other/instructions", BaseInstructionsVersion: "2.0.0"},
	}
	for _, contribution := range tests {
		registry := aiagent.NewRegistry()
		base := contract
		base.AllowedToolCapabilities = []string{"system"}
		if err := registry.Register(base); err != nil {
			t.Fatal(err)
		}
		if err := registry.ApplyContributions([]aiagent.ContractContribution{contribution}); err == nil {
			t.Fatalf("unsafe contribution must fail: %#v", contribution)
		}
	}
	registry := aiagent.NewRegistry()
	base := contract
	base.AllowedToolCapabilities = []string{"system"}
	if err := registry.Register(base); err != nil {
		t.Fatal(err)
	}
	err = registry.ApplyContributions([]aiagent.ContractContribution{
		{Owner: "alpha", TargetKey: "system.operations", TargetVersion: "1.0.0", AllowedToolCapabilities: []string{"alpha"}},
		{Owner: "zeta", TargetKey: "unknown.contract", TargetVersion: "1.0.0", AllowedToolCapabilities: []string{"zeta"}},
	})
	if err == nil {
		t.Fatal("partially invalid contribution set must fail")
	}
	unchanged, _ := registry.Get("system.operations", "1.0.0")
	if slices.Contains(unchanged.AllowedToolCapabilities, "alpha") {
		t.Fatalf("failed contribution set mutated the registry: %#v", unchanged.AllowedToolCapabilities)
	}
}

func TestAgentToolCapabilityCoverageDoesNotExcludeUnexposedDemo(t *testing.T) {
	if err := prompt.LoadDir("../prompts"); err != nil {
		t.Fatal(err)
	}
	contracts, err := ai.NewBuiltinAgentContractRegistry()
	if err != nil {
		t.Fatal(err)
	}
	application := &app.App{Actions: action.NewRegistry(nil, nil), AgentContracts: contracts}
	application.AgentSlots = agentslot.NewRegistry(contracts)
	if err := application.AgentSlots.Register(agentslot.Slot{Key: "workspace.default", Version: "1.0.0", Module: "workspace", Title: "Default", DefaultContractKey: "system.operations", DefaultContractVersion: "1.0.0", RequiredCapabilities: []string{"chat", "tools"}, TenantOverride: true, UserOverride: true, FallbackPolicy: agentslot.FallbackFailClosed, UpgradePolicy: agentslot.UpgradeCompatible}); err != nil {
		t.Fatal(err)
	}
	application.AgentTenantInitializer = func(context.Context, *gorm.DB, string, string) error { return nil }
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
