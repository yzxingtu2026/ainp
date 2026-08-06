package tests

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/config"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	"github.com/yzxingtu2026/ainp/platform/framework/job"
	"github.com/yzxingtu2026/ainp/platform/framework/menu"
	"github.com/yzxingtu2026/ainp/platform/host"
	"github.com/yzxingtu2026/ainp/platform/migrations"
)

type testHostModule struct {
	id       string
	events   *[]string
	prepare  bool
	source   *migrations.Source
	register func(*app.App) error
}

func (m testHostModule) ID() string { return m.id }
func (m testHostModule) Register(application *app.App) error {
	if m.events != nil {
		*m.events = append(*m.events, "register:"+m.id)
	}
	if m.register != nil {
		return m.register(application)
	}
	return nil
}
func (m testHostModule) Prepare(*app.App) error {
	if m.prepare && m.events != nil {
		*m.events = append(*m.events, "prepare:"+m.id)
	}
	return nil
}
func (m testHostModule) MigrationSource() migrations.Source {
	if m.source == nil {
		return migrations.Source{}
	}
	return *m.source
}

func TestHostCompositionRejectsInvalidAndDuplicateModuleIDs(t *testing.T) {
	for _, test := range []struct {
		name    string
		modules []host.Module
	}{
		{name: "invalid", modules: []host.Module{testHostModule{id: "Bad ID"}}},
		{name: "duplicate", modules: []host.Module{testHostModule{id: "orders"}, testHostModule{id: "orders"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := host.New(host.WithModules(test.modules...)); err == nil {
				t.Fatal("expected fail-closed module validation")
			}
		})
	}
}

func TestHostCompositionRejectsMigrationOwnerDifferentFromModule(t *testing.T) {
	source := migrations.Source{Owner: "other", FS: fstest.MapFS{"0001_init.sql": {Data: []byte("SELECT 1;")}}}
	_, err := host.New(host.WithModules(testHostModule{id: "orders", source: &source}))
	if err == nil {
		t.Fatal("expected migration owner mismatch")
	}
	if got := fmt.Sprint(err); got != `host: module "orders" migration owner must match module id` {
		t.Fatalf("unexpected error: %s", got)
	}
}

func TestHostCompositionExcludesDemoByDefaultAndRequiresExplicitOptIn(t *testing.T) {
	production, err := host.New()
	if err != nil {
		t.Fatal(err)
	}
	if production.DemoEnabled() {
		t.Fatal("downstream host must exclude Demo by default")
	}

	showcase, err := host.New(host.WithDemo())
	if err != nil {
		t.Fatal(err)
	}
	if !showcase.DemoEnabled() {
		t.Fatal("WithDemo must explicitly include Demo")
	}
}

func TestHostDemoSelectionControlsRegistriesAndSeedData(t *testing.T) {
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	root, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}

	previousDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(".."); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previousDir) })

	for _, test := range []struct {
		name        string
		options     []host.Option
		demoEnabled bool
	}{
		{name: "production default"},
		{name: "explicit showcase", options: []host.Option{host.WithDemo()}, demoEnabled: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			schema := "ainp_host_demo_" + strings.ReplaceAll(uuid.NewString()[:8], "-", "")
			if err := root.Exec(fmt.Sprintf(`CREATE SCHEMA %q`, schema)).Error; err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = root.Exec(fmt.Sprintf(`DROP SCHEMA %q CASCADE`, schema)).Error })

			cfg := hostTestConfig(dsn+" search_path="+schema, t.TempDir())
			platform, err := host.OpenInstalled(context.Background(), cfg, test.options...)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				platform.App.Outbox = nil
				_ = platform.Close()
			})

			_, capabilityExists := platform.App.Capabilities.Get("demo")
			_, actionExists := platform.App.Actions.Get("demo.hello")
			menuExists := registryHasMatch(platform.App.Menus.List(), func(item menu.Menu) bool { return strings.HasPrefix(item.ID, "demo.") })
			helpExists := registryHasMatch(platform.App.PageHelp.List(), func(item menu.HelpDocument) bool { return strings.HasPrefix(item.MenuID, "demo.") })
			jobExists := registryHasMatch(platform.App.Jobs.Registry.List(), func(item job.Definition) bool { return strings.HasPrefix(item.Type, "demo.") })
			for name, got := range map[string]bool{
				"capability": capabilityExists,
				"action":     actionExists,
				"menu":       menuExists,
				"job":        jobExists,
			} {
				if got != test.demoEnabled {
					t.Fatalf("Demo %s registration = %v, want %v", name, got, test.demoEnabled)
				}
			}
			if !test.demoEnabled && helpExists {
				t.Fatal("production default registered Demo page help")
			}

			var tenants int64
			if err := platform.DB.Table("tenants").Count(&tenants).Error; err != nil {
				t.Fatal(err)
			}
			if test.demoEnabled && tenants == 0 {
				t.Fatal("explicit Demo must create the idempotent showcase tenant in an empty database")
			}
			if !test.demoEnabled && tenants != 0 {
				t.Fatalf("production default wrote %d Demo tenants", tenants)
			}
		})
	}
}

func hostTestConfig(dsn, storageRoot string) *config.Config {
	disabled := false
	return &config.Config{
		Database: config.DatabaseConfig{DSN: dsn},
		Auth: config.AuthConfig{
			JWT:        config.JWTConfig{Secret: "host-demo-test-secret", Issuer: "ainp-test", TTL: "15m"},
			RefreshTTL: "720h",
		},
		AI:      config.AIConfig{CredentialEncryptionKey: "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="},
		I18n:    config.I18nConfig{Dir: "i18n", DefaultLanguage: "zh-CN"},
		Storage: config.StorageConfig{Provider: "local", Local: config.StorageLocalConfig{RootDir: storageRoot}},
		Outbox:  config.OutboxConfig{Enabled: &disabled},
		Jobs:    config.JobsConfig{Enabled: &disabled},
	}
}

func registryHasMatch[T any](items []T, matches func(T) bool) bool {
	for _, item := range items {
		if matches(item) {
			return true
		}
	}
	return false
}
