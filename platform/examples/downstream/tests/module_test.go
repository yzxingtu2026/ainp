package downstream_test

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"example.com/ainp-downstream/composition"
	"example.com/ainp-downstream/modules/orders"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/aiagent"
	"github.com/yzxingtu2026/ainp/platform/framework/aitool"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/config"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	"github.com/yzxingtu2026/ainp/platform/framework/i18n"
	"github.com/yzxingtu2026/ainp/platform/framework/job"
	"github.com/yzxingtu2026/ainp/platform/framework/prompt"
	"github.com/yzxingtu2026/ainp/platform/host"
)

func TestOneModuleDeclarationBuildsAPIAndWorkerCompositions(t *testing.T) {
	api, err := host.New(host.WithModules(composition.Modules...))
	if err != nil {
		t.Fatal(err)
	}
	worker, err := host.New(host.WithModules(composition.Modules...))
	if err != nil {
		t.Fatal(err)
	}
	if api.DemoEnabled() || worker.DemoEnabled() {
		t.Fatal("production downstream compositions must exclude Demo by default")
	}

	showcase, err := host.New(host.WithModules(composition.Modules...), host.WithDemo())
	if err != nil {
		t.Fatal(err)
	}
	if !showcase.DemoEnabled() {
		t.Fatal("downstream showcase must opt in to Demo explicitly")
	}
}

func TestDownstreamHostLoadsEmbeddedRuntimeAssetsFromArbitraryWorkingDirectory(t *testing.T) {
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	root, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "ainp_downstream_assets_" + uuid.NewString()[:8]
	if err := root.Exec(fmt.Sprintf(`CREATE SCHEMA %q`, schema)).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Exec(fmt.Sprintf(`DROP SCHEMA %q CASCADE`, schema)).Error })

	previousDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previousDir) })

	disabled := false
	cfg := &config.Config{
		Database: config.DatabaseConfig{DSN: dsn + " search_path=" + schema},
		Auth: config.AuthConfig{
			JWT:        config.JWTConfig{Secret: "downstream-assets-test", Issuer: "ainp-test", TTL: "15m"},
			RefreshTTL: "720h",
		},
		AI:      config.AIConfig{CredentialEncryptionKey: "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="},
		Storage: config.StorageConfig{Provider: "local", Local: config.StorageLocalConfig{RootDir: t.TempDir()}},
		Outbox:  config.OutboxConfig{Enabled: &disabled},
		Jobs:    config.JobsConfig{Enabled: &disabled},
	}
	platform, err := host.OpenInstalled(context.Background(), cfg, host.WithModules(composition.Modules...))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		platform.App.Outbox = nil
		_ = platform.Close()
	})

	if prompt.Get("agent/system-operations") == "" || prompt.Get(orders.AgentPromptKey) == "" || prompt.Get("example.orders/order-summary") == "" {
		t.Fatal("built-in and downstream Prompts must both load from embedded filesystems")
	}
	if got := i18n.T("zh-CN", "example.orders.order_list"); got != "订单" {
		t.Fatalf("unexpected downstream translation: %q", got)
	}
	owners := map[string]bool{}
	for _, asset := range platform.RuntimeAssets {
		owners[asset.Owner] = true
		if !strings.HasPrefix(asset.Digest, "sha256:") {
			t.Fatalf("unsafe or missing asset digest: %#v", asset)
		}
	}
	if !owners["ainp"] || !owners[orders.ModuleID] {
		t.Fatalf("runtime asset manifest is missing owners: %#v", owners)
	}
}

func TestExternalModuleRegistersAtomicAICreateContract(t *testing.T) {
	assets := (orders.Module{}).RuntimeAssets()
	if err := prompt.LoadSources(prompt.Source{Owner: orders.ModuleID, FS: assets.Prompts}); err != nil {
		t.Fatal(err)
	}
	application := app.New(&config.Config{}, nil, nil, nil)
	application.AgentContracts = aiagent.NewRegistry()
	application.Jobs = &job.Runtime{Registry: job.NewRegistry()}
	if err := (orders.Module{}).Register(application); err != nil {
		t.Fatal(err)
	}
	contract, ok := application.AgentContracts.Get(orders.ModuleID, "1.0.0")
	if !ok {
		t.Fatal("downstream Agent Contract was not registered")
	}
	if contract.BaseInstructionsVersion != "1.0.0" || !slices.Equal(contract.AllowedToolSources, []string{"action"}) || !slices.Equal(contract.AllowedToolCapabilities, []string{orders.CapabilityID}) {
		t.Fatalf("downstream Agent Contract does not expose the orders Action scope: %#v", contract)
	}
	registered, ok := application.Actions.Get("orders.batch_create")
	if !ok || registered.Meta.AI == nil {
		t.Fatal("downstream batch create action was not registered")
	}
	if registered.Meta.AI.Operation != action.AIOperationCreate || registered.Meta.AI.Batch == nil || registered.Meta.AI.Batch.Mode != action.AIBatchAtomic || registered.Meta.IdempotencyMode() != "required" {
		t.Fatalf("downstream batch contract is incomplete: %#v", registered.Meta)
	}
	definition, err := (&aitool.ActionProvider{Actions: application.Actions}).Get(context.Background(), "action:orders.batch_create")
	if err != nil {
		t.Fatal(err)
	}
	if definition.Operation != action.AIOperationCreate || definition.Batch == nil || definition.Batch.ItemsPath != "items" {
		t.Fatalf("AI create contract was not projected: %#v", definition)
	}
	if err := application.AgentContracts.ValidateActionToolCapabilityCoverage(application.Actions, nil); err != nil {
		t.Fatalf("downstream Action capabilities must be reachable from an Agent Contract: %v", err)
	}
}
