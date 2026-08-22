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
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
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
	contract, ok := platform.App.AgentContracts.Get("system.operations", "1.0.0")
	if !ok || !slices.Contains(contract.AllowedToolCapabilities, orders.CapabilityID) {
		t.Fatalf("default Agent Contract is missing downstream capability: %#v", contract.AllowedToolCapabilities)
	}
	slot, ok := platform.App.AgentSlots.Get("workspace.default", "1.0.0")
	if !ok || slot.DefaultContractKey != "system.operations" || slot.DefaultContractVersion != "1.0.0" {
		t.Fatalf("workspace default Slot no longer resolves the builtin Contract: %#v", slot)
	}
	tenantID := "tenant-default-agent"
	userID := "user-default-agent"
	if err := platform.App.AgentTenantInitializer(context.Background(), platform.DB, tenantID, userID); err != nil {
		t.Fatal(err)
	}
	var builtinCount int64
	if err := platform.DB.Table("ai_agents").Where("tenant_id=? AND source=? AND contract_key=?", tenantID, "builtin", "system.operations").Count(&builtinCount).Error; err != nil {
		t.Fatal(err)
	}
	if builtinCount != 1 {
		t.Fatalf("tenant initializer created %d builtin system operations Agents", builtinCount)
	}
}

func TestExternalModuleContributesAtomicAICreateToDefaultContract(t *testing.T) {
	assets := (orders.Module{}).RuntimeAssets()
	if err := prompt.LoadSources(prompt.Source{Owner: "ainp", FS: os.DirFS("../../../prompts")}, prompt.Source{Owner: orders.ModuleID, FS: assets.Prompts}); err != nil {
		t.Fatal(err)
	}
	application := app.New(&config.Config{}, nil, nil, nil)
	contract := aiagent.Contract{Key: "system.operations", Version: "1.0.0", Name: "System operations", BaseInstructions: prompt.MustGet("agent/system-operations"), BaseInstructionsVersion: "2.2.0", RequiredModelCapabilities: []string{"chat", "tools"}, AllowedToolSources: []string{"action"}, AllowedToolCapabilities: []string{"system", "operations", "ai"}, ConfigSchema: []byte(`{"type":"object"}`), SingletonScope: aiagent.SingletonTenant, ManagedFields: []string{"code", "source", "contract_key", "contract_version", "base_instructions", "visibility"}, UpgradePolicy: "managed-fields"}
	application.AgentContracts = aiagent.NewRegistry()
	if err := application.AgentContracts.Register(contract); err != nil {
		t.Fatal(err)
	}
	application.Jobs = &job.Runtime{Registry: job.NewRegistry()}
	if err := (orders.Module{}).Register(application); err != nil {
		t.Fatal(err)
	}
	contributions := (orders.Module{}).AgentContractContributions()
	contributions[0].Owner = orders.ModuleID
	if err := application.AgentContracts.ApplyContributions(contributions); err != nil {
		t.Fatal(err)
	}
	contract, ok := application.AgentContracts.Get("system.operations", "1.0.0")
	if !ok || !slices.Contains(contract.AllowedToolCapabilities, orders.CapabilityID) || !strings.Contains(contract.BaseInstructions, "downstream module instructions: "+orders.ModuleID) {
		t.Fatalf("default Agent Contract does not include the orders contribution: %#v", contract)
	}
	if _, exists := application.AgentContracts.Get(orders.ModuleID, "1.0.0"); exists {
		t.Fatal("downstream contribution must not create a ghost Agent Contract")
	}
	registered, ok := application.Actions.Get("orders.batch_create")
	if !ok || registered.Meta.AI == nil {
		t.Fatal("downstream batch create action was not registered")
	}
	customerCtx := auth.WithContext(context.Background(), &auth.Context{Authenticated: true, Realm: auth.RealmTenant, TenantID: "example-tenant", PrincipalID: "customer-1", Principal: auth.PrincipalRef{Kind: "example.customer", ID: "customer-1"}, ClientSurface: "example.customer-client", Permissions: []string{"orders:read"}})
	if _, err := application.Actions.Invoke(customerCtx, "orders.customer_list", nil); err != nil {
		t.Fatalf("customer surface action: %v", err)
	}
	accountCtx := auth.WithContext(context.Background(), &auth.Context{Authenticated: true, Realm: auth.RealmTenant, TenantID: "example-tenant", PrincipalID: "account-1", Principal: auth.PrincipalRef{Kind: auth.PrincipalKindAccount, ID: "account-1"}, ClientSurface: auth.TenantConsoleSurface, Permissions: []string{"orders:read"}})
	if _, err := application.Actions.Invoke(accountCtx, "orders.customer_list", nil); err == nil {
		t.Fatal("account must not invoke customer-only action")
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
	if err := application.AgentContracts.ValidateActionToolCapabilityReachability(application.Actions, map[string]struct{}{"system.operations@1.0.0": {}}, nil); err != nil {
		t.Fatalf("downstream Action capabilities must be reachable from an Agent Contract: %v", err)
	}
}
