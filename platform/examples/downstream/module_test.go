package downstream

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/action"
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
	modules := []host.Module{Module{}}
	api, err := host.New(host.WithModules(modules...))
	if err != nil {
		t.Fatal(err)
	}
	worker, err := host.New(host.WithModules(modules...))
	if err != nil {
		t.Fatal(err)
	}
	if api.DemoEnabled() || worker.DemoEnabled() {
		t.Fatal("production downstream compositions must exclude Demo by default")
	}

	showcase, err := host.New(host.WithModules(modules...), host.WithDemo())
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
	platform, err := host.OpenInstalled(context.Background(), cfg, host.WithModules(Module{}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		platform.App.Outbox = nil
		_ = platform.Close()
	})

	if prompt.Get("agent/system-operations") == "" || prompt.Get("example.orders/order-summary") == "" {
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
	if !owners["ainp"] || !owners["example.orders"] {
		t.Fatalf("runtime asset manifest is missing owners: %#v", owners)
	}
}

func TestExternalModuleRegistersAtomicAICreateContract(t *testing.T) {
	application := app.New(&config.Config{}, nil, nil, nil)
	application.Jobs = &job.Runtime{Registry: job.NewRegistry()}
	if err := (Module{}).Register(application); err != nil {
		t.Fatal(err)
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
}
