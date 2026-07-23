package tests

import (
	"slices"
	"testing"

	. "github.com/yzxingtu2026/ainp/platform/modules/ai"
	navigationtool "github.com/yzxingtu2026/ainp/platform/modules/ai/tools/navigation"
)

func TestSystemOperationsIncludesDirectNavigationSearchAndHelp(t *testing.T) {
	registry, err := NewBuiltinAgentContractRegistry()
	if err != nil {
		t.Fatal(err)
	}
	contract, ok := registry.Get("system.operations", "1.0.0")
	if !ok {
		t.Fatal("system operations contract missing")
	}
	for _, toolID := range []string{navigationtool.ToolID, navigationtool.HelpToolID} {
		if !slices.Contains(contract.RequiredToolSelectors, toolID) {
			t.Fatalf("required navigation slot missing: %s", toolID)
		}
	}
}
