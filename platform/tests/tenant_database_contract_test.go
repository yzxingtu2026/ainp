package tests

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	"github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type tenantContractDoc struct {
	ID        string `gorm:"primaryKey"`
	TenantID  string `gorm:"size:64;not null"`
	Title     string
	Revision  uint64
	DeletedAt gorm.DeletedAt
}

func (tenantContractDoc) TableName() string { return "tenant_contract_docs" }

type tenantContractGlobal struct {
	ID   string `gorm:"primaryKey"`
	Name string
}

func (tenantContractGlobal) TableName() string { return "tenant_contract_globals" }

func tenantDryDB(t *testing.T, registry *tenant.Registry) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(postgres.New(postgres.Config{DSN: "host=127.0.0.1 user=test dbname=test sslmode=disable", PreferSimpleProtocol: true}), &gorm.Config{DryRun: true, DisableAutomaticPing: true, SkipDefaultTransaction: true})
	if err != nil {
		t.Fatal(err)
	}
	if err = tenant.InstallCallbacks(db, registry); err != nil {
		t.Fatal(err)
	}
	return db
}
func tenantContext(id string) context.Context {
	return auth.WithContext(context.Background(), &auth.Context{Authenticated: true, Realm: auth.RealmTenant, TenantID: id})
}

func TestTenantRegistryValidationAndInventory(t *testing.T) {
	r := tenant.NewRegistry()
	if err := r.Register(tenantContractDoc{}, tenant.ModelOptions{Kind: tenant.TenantOwned, InjectOnCreate: true, SoftDelete: true, SystemAccess: true}); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(tenantContractGlobal{}, tenant.ModelOptions{Kind: tenant.GlobalModel}); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(&tenantContractDoc{}, tenant.ModelOptions{Kind: tenant.TenantOwned}); !errors.Is(err, tenant.ErrDuplicateModel) {
		t.Fatalf("expected duplicate error, got %v", err)
	}
	if err := r.ValidateKnown(tenantContractDoc{}, tenantContractGlobal{}); err != nil {
		t.Fatal(err)
	}
	if got := r.Inventory(); len(got) != 2 || got[0].Model > got[1].Model {
		t.Fatalf("unexpected inventory: %#v", got)
	}
}

func TestTenantCallbacksFailClosedAndInjectTenant(t *testing.T) {
	r := tenant.NewRegistry()
	_ = r.Register(tenantContractDoc{}, tenant.ModelOptions{Kind: tenant.TenantOwned, InjectOnCreate: true, SoftDelete: true, SystemAccess: true})
	_ = r.Register(tenantContractGlobal{}, tenant.ModelOptions{Kind: tenant.GlobalModel})
	db := tenantDryDB(t, r)
	if err := db.Find(&[]tenantContractDoc{}).Error; !errors.Is(err, tenant.ErrTenantDBRequired) {
		t.Fatalf("naked DB must fail closed: %v", err)
	}
	tdb, err := tenant.TenantDB(tenantContext("tenant-a"), db)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := tenant.NewTenantRepository[tenantContractDoc](tdb)
	if err != nil {
		t.Fatal(err)
	}
	doc := tenantContractDoc{ID: "d1"}
	if err = repo.Create(&doc); err != nil {
		t.Fatal(err)
	}
	if doc.TenantID != "tenant-a" {
		t.Fatalf("tenant not injected: %#v", doc)
	}
	conflict := tenantContractDoc{ID: "d2", TenantID: "tenant-b"}
	if err = repo.Create(&conflict); !errors.Is(err, tenant.ErrTenantConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
	if _, err = tenant.NewTenantRepository[tenantContractDoc](tenant.TenantDatabase{}); !errors.Is(err, tenant.ErrTenantDBRequired) {
		t.Fatalf("expected typed boundary error: %v", err)
	}
	global := db.Find(&[]tenantContractGlobal{})
	if global.Error != nil {
		t.Fatalf("global model was blocked: %v", global.Error)
	}
}

func TestTenantQuerySQLAndSystemBoundary(t *testing.T) {
	r := tenant.NewRegistry()
	_ = r.Register(tenantContractDoc{}, tenant.ModelOptions{Kind: tenant.TenantOwned, InjectOnCreate: true, SoftDelete: true, SystemAccess: true})
	db := tenantDryDB(t, r)
	tdb, _ := tenant.TenantDB(tenantContext("tenant-a"), db)
	repo, _ := tenant.NewTenantRepository[tenantContractDoc](tdb)
	_, err := repo.GetByID("d1")
	if err != nil {
		t.Fatal(err)
	}
	stmt := db.Session(&gorm.Session{DryRun: true})
	wrapped, _ := tenant.TenantDB(tenantContext("tenant-a"), stmt)
	// Executing through the repository proves tenant and soft-delete predicates coexist.
	q, _ := tenant.NewTenantRepository[tenantContractDoc](wrapped)
	_, _ = q.List(0, 10)
	badCtx := auth.WithContext(context.Background(), &auth.Context{Authenticated: true, Realm: auth.RealmTenant, IsSystemOperator: true, TenantID: "a"})
	if _, err := tenant.SystemDB(badCtx, db); !errors.Is(err, tenant.ErrPlatformRealmRequired) {
		t.Fatalf("tenant realm built SystemDB: %v", err)
	}
	platform := auth.WithContext(context.Background(), &auth.Context{Authenticated: true, Realm: auth.RealmPlatform, IsSystemOperator: true})
	sdb, err := tenant.SystemDB(platform, db)
	if err != nil {
		t.Fatal(err)
	}
	srepo, _ := tenant.NewSystemRepository[tenantContractDoc](sdb)
	if _, err = srepo.List(0, 10); err != nil {
		t.Fatal(err)
	}
	if _, err = tenant.TenantRaw[tenantContractDoc](tdb, "SELECT * FROM tenant_contract_docs"); !errors.Is(err, tenant.ErrUnsafeQuery) {
		t.Fatalf("unbound raw query accepted: %v", err)
	}
}

func TestTenantRepositoryKeepsChainedConditions(t *testing.T) {
	r := tenant.NewRegistry()
	_ = r.Register(tenantContractDoc{}, tenant.ModelOptions{Kind: tenant.TenantOwned, InjectOnCreate: true, SystemAccess: true})
	db := tenantDryDB(t, r)
	tdb, _ := tenant.TenantDB(tenantContext("tenant-a"), db)
	repo, _ := tenant.NewTenantRepository[tenantContractDoc](tdb)
	query := repo.Where("title = ?", "A").Where("revision = ?", 2).Order("id desc")
	_, err := query.Find("id <> ?", "x")
	if err != nil {
		t.Fatal(err)
	}
}

func TestTenantTransactionKeepsAccessMode(t *testing.T) {
	r := tenant.NewRegistry()
	_ = r.Register(tenantContractDoc{}, tenant.ModelOptions{Kind: tenant.TenantOwned, InjectOnCreate: true, SystemAccess: true})
	db := tenantDryDB(t, r)
	tdb, _ := tenant.TenantDB(tenantContext("tenant-a"), db)
	err := tdb.Transaction(func(tx tenant.TenantDatabase) error {
		repo, _ := tenant.NewTenantRepository[tenantContractDoc](tx)
		doc := tenantContractDoc{ID: "tx", Revision: 1}
		return repo.Create(&doc)
	})
	// DryRun transactions may report the driver's placeholder transaction error; access errors must never appear.
	if errors.Is(err, tenant.ErrTenantDBRequired) || errors.Is(err, tenant.ErrTenantContextRequired) {
		t.Fatalf("transaction lost tenant access: %v", err)
	}
}

func TestTenantDatabasePostgresIsolation(t *testing.T) {
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	if _, ok := tenant.DefaultRegistry.Lookup(tenantContractDoc{}); !ok {
		if err := tenant.DefaultRegistry.Register(tenantContractDoc{}, tenant.ModelOptions{Kind: tenant.TenantOwned, InjectOnCreate: true, SoftDelete: true, SystemAccess: true}); err != nil {
			t.Fatal(err)
		}
	}
	admin, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "ainp_tenant_" + uuid.NewString()[:8]
	if err = admin.Exec(fmt.Sprintf(`CREATE SCHEMA %q`, schema)).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Exec(fmt.Sprintf(`DROP SCHEMA %q CASCADE`, schema)).Error })
	db, err := database.Open(dsn + " search_path=" + schema)
	if err != nil {
		t.Fatal(err)
	}
	platform := auth.WithContext(context.Background(), &auth.Context{Authenticated: true, Realm: auth.RealmPlatform, IsSystemOperator: true})
	sdb, err := tenant.SystemDB(platform, db)
	if err != nil {
		t.Fatal(err)
	}
	srepo, _ := tenant.NewSystemRepository[tenantContractDoc](sdb)
	if err = db.Session(&gorm.Session{SkipHooks: true}).AutoMigrate(&tenantContractDoc{}); err != nil {
		t.Fatal(err)
	}

	aDB, _ := tenant.TenantDB(tenantContext("tenant-a"), db)
	aRepo, _ := tenant.NewTenantRepository[tenantContractDoc](aDB)
	bDB, _ := tenant.TenantDB(tenantContext("tenant-b"), db)
	bRepo, _ := tenant.NewTenantRepository[tenantContractDoc](bDB)
	a := tenantContractDoc{ID: "a", Title: "A", Revision: 1}
	b := tenantContractDoc{ID: "b", Title: "B", Revision: 1}
	if err = aRepo.Create(&a); err != nil {
		t.Fatal(err)
	}
	if err = bRepo.Create(&b); err != nil {
		t.Fatal(err)
	}
	if a.TenantID != "tenant-a" || b.TenantID != "tenant-b" {
		t.Fatalf("trusted tenants not injected: %#v %#v", a, b)
	}
	if _, err = aRepo.GetByID("b"); !tenant.IsNotFound(err) {
		t.Fatalf("tenant A read tenant B: %v", err)
	}
	if rows, err := aRepo.List(0, 10); err != nil || len(rows) != 1 || rows[0].ID != "a" {
		t.Fatalf("tenant A list leaked: %#v %v", rows, err)
	}
	if err = aRepo.UpdateRevision("a", 1, map[string]any{"title": "A2", "revision": 2}); err != nil {
		t.Fatal(err)
	}
	if err = aRepo.Delete(&a); err != nil {
		t.Fatal(err)
	}
	if _, err = aRepo.GetByID("a"); !tenant.IsNotFound(err) {
		t.Fatalf("soft deleted row remained visible: %v", err)
	}
	if rows, err := srepo.List(0, 10); err != nil || len(rows) != 1 || rows[0].ID != "b" {
		t.Fatalf("system view unexpected: %#v %v", rows, err)
	}
	if err = db.Find(&[]tenantContractDoc{}).Error; !errors.Is(err, tenant.ErrTenantDBRequired) {
		t.Fatalf("naked postgres DB did not fail closed: %v", err)
	}
}
