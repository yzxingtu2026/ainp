package tests

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodelruntime"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	"github.com/yzxingtu2026/ainp/platform/framework/menu"
	"github.com/yzxingtu2026/ainp/platform/framework/setting"
	"github.com/yzxingtu2026/ainp/platform/modules/ai"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestNavigationIndexStatusIgnoresRetiredDocuments(t *testing.T) {
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	admin, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "ainp_navigation_status_" + uuid.NewString()[:8]
	if err = admin.Exec(fmt.Sprintf(`CREATE SCHEMA %q`, schema)).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Exec(fmt.Sprintf(`DROP SCHEMA %q CASCADE`, schema)).Error })
	db, err := database.Open(dsn + " search_path=" + schema)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(`CREATE TABLE navigation_search_documents (
		id text PRIMARY KEY,
		tenant_id text NOT NULL,
		index_status text NOT NULL,
		updated_at timestamptz
	)`).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(`INSERT INTO navigation_search_documents (id, tenant_id, index_status) VALUES
		('active', 'tenant-1', 'ready'),
		('retired', 'tenant-1', 'stale')`).Error; err != nil {
		t.Fatal(err)
	}

	settings := setting.NewRegistry()
	if err = settings.RegisterCategory(setting.Category{Key: "ai", Title: "AI", Icon: "mdi:brain", Surface: auth.RealmTenant}); err != nil {
		t.Fatal(err)
	}
	if err = settings.RegisterDefinition(setting.Definition{Key: menu.SettingKeySemanticSearch, CategoryKey: "ai", Title: "Navigation search", Scope: setting.ScopeTenant, Surface: auth.RealmTenant, SchemaVersion: 1, Fields: []setting.Field{{Key: menu.SettingFieldModelAlias, Type: setting.FieldText, Label: "Embedding Model Alias", Default: ""}}}); err != nil {
		t.Fatal(err)
	}
	provider := setting.NewProvider(settings, navigationSettingStore{alias: "embedding-alias"})
	service := ai.NewNavigationSearchService(db, menu.NewRegistry(), menu.NewHelpRegistry(), provider, navigationEmbeddingRuntime{}, nil)
	ctx := auth.WithContext(context.Background(), &auth.Context{Authenticated: true, Realm: auth.RealmTenant, TenantID: "tenant-1"})
	status, err := service.IndexStatus(ctx, "tenant-1")
	if err != nil || status.State != "ready" || status.Total != 1 || status.Ready != 1 || status.Stale != 1 {
		t.Fatalf("retired navigation document degraded active index status: %#v %v", status, err)
	}
}

type navigationSettingStore struct{ alias string }

func (s navigationSettingStore) Get(context.Context, string, string) (*setting.StoredValue, error) {
	return &setting.StoredValue{SchemaVersion: 1, Revision: 1, Values: map[string]any{menu.SettingFieldModelAlias: s.alias}}, nil
}
func (navigationSettingStore) Put(context.Context, string, string, int64, int, map[string]any, string) (*setting.StoredValue, error) {
	return nil, errors.New("not implemented")
}
func (navigationSettingStore) Delete(context.Context, string, string, int64) error {
	return errors.New("not implemented")
}

type navigationEmbeddingRuntime struct{}

func (navigationEmbeddingRuntime) Embed(context.Context, aimodelruntime.EmbeddingRequest) (*aimodelruntime.EmbeddingResponse, error) {
	return &aimodelruntime.EmbeddingResponse{
		Model:                aimodelruntime.ModelSummary{DeploymentID: "deployment-1"},
		Dimensions:           3,
		PreprocessingVersion: aimodelruntime.EmbeddingPreprocessingVersionCosine,
		Vectors:              []aimodelruntime.EmbeddingVector{{ID: "query", Vector: []float32{0.1, 0.2, 0.3}}},
	}, nil
}

func TestNavigationSemanticSearchBindsEveryVectorQueryArgument(t *testing.T) {
	db, err := gorm.Open(postgres.New(postgres.Config{DSN: "host=127.0.0.1 user=test dbname=test sslmode=disable", PreferSimpleProtocol: true}), &gorm.Config{DryRun: true, DisableAutomaticPing: true, SkipDefaultTransaction: true})
	if err != nil {
		t.Fatal(err)
	}
	settings := setting.NewRegistry()
	if err = settings.RegisterCategory(setting.Category{Key: "ai", Title: "AI", Icon: "mdi:brain", Surface: auth.RealmTenant}); err != nil {
		t.Fatal(err)
	}
	if err = settings.RegisterDefinition(setting.Definition{Key: menu.SettingKeySemanticSearch, CategoryKey: "ai", Title: "Navigation search", Scope: setting.ScopeTenant, Surface: auth.RealmTenant, SchemaVersion: 1, Fields: []setting.Field{{Key: menu.SettingFieldModelAlias, Type: setting.FieldText, Label: "Embedding Model Alias", Default: ""}}}); err != nil {
		t.Fatal(err)
	}
	provider := setting.NewProvider(settings, navigationSettingStore{alias: "embedding-alias"})

	var query string
	var args []any
	capture := func(tx *gorm.DB) {
		if tx.Statement.SQL.Len() == 0 {
			return
		}
		query = tx.Statement.SQL.String()
		args = append([]any(nil), tx.Statement.Vars...)
	}
	for _, register := range []func(string, func(*gorm.DB)) error{
		db.Callback().Query().Before("gorm:query").Register,
		db.Callback().Raw().Before("gorm:raw").Register,
		db.Callback().Row().Before("gorm:row").Register,
	} {
		if err = register("tests:capture_navigation_query", capture); err != nil {
			t.Fatal(err)
		}
	}
	service := ai.NewNavigationSearchService(db, menu.NewRegistry(), menu.NewHelpRegistry(), provider, navigationEmbeddingRuntime{}, nil)
	ctx := auth.WithContext(context.Background(), &auth.Context{Authenticated: true, Realm: auth.RealmTenant, TenantID: "tenant-1"})
	service.SearchNavigation(ctx, menu.SemanticSearchRequest{TenantID: "tenant-1", Language: "zh-CN", Query: "附件页面地址", AllowedMenuIDs: []string{"system.attachments", "system.accounts"}})
	if query == "" {
		t.Fatal("navigation vector query was not executed")
	}
	if got, want := len(args), 10; got != want {
		t.Fatalf("bound arguments = %d, want %d", got, want)
	}
	if len(args) < 2 || args[len(args)-2] != "system.attachments" || args[len(args)-1] != "system.accounts" {
		t.Fatalf("unexpected allowed menu arguments: %#v", args)
	}
}
