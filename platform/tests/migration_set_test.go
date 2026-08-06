package tests

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	"github.com/yzxingtu2026/ainp/platform/migrations"
)

func TestMigrationSetValidatesOwnersAndSequences(t *testing.T) {
	valid := fstest.MapFS{
		"0001_init.sql": {Data: []byte("SELECT 1;")},
		"0002_more.sql": {Data: []byte("SELECT 2;")},
	}
	if _, err := migrations.NewSet(migrations.Source{Owner: "orders", FS: valid}); err != nil {
		t.Fatalf("valid source rejected: %v", err)
	}
	if _, err := migrations.NewSet(
		migrations.Source{Owner: "orders", FS: valid},
		migrations.Source{Owner: "orders", FS: valid},
	); err == nil || !strings.Contains(err.Error(), "duplicate migration owner") {
		t.Fatalf("expected duplicate owner error, got %v", err)
	}
	gap := fstest.MapFS{
		"0001_init.sql": {Data: []byte("SELECT 1;")},
		"0003_gap.sql":  {Data: []byte("SELECT 3;")},
	}
	if _, err := migrations.NewSet(migrations.Source{Owner: "orders", FS: gap}); err == nil || !strings.Contains(err.Error(), "sequence gap") {
		t.Fatalf("expected sequence gap error, got %v", err)
	}
}

func TestMigrationSetBaselineUpAndChecksumFailClosed(t *testing.T) {
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	db, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "ainp_downstream_migrations_" + strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	if err := db.Exec(fmt.Sprintf(`CREATE SCHEMA %q`, schema)).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Exec(fmt.Sprintf(`DROP SCHEMA %q CASCADE`, schema)).Error })
	testDB, err := database.Open(dsn + " search_path=" + schema)
	if err != nil {
		t.Fatal(err)
	}
	if err := testDB.Exec(`CREATE TABLE tenant_accounts (id varchar(64) PRIMARY KEY)`).Error; err != nil {
		t.Fatal(err)
	}

	v1 := fstest.MapFS{"0001_init.sql": {Data: []byte(`CREATE TABLE downstream_v1 (id integer PRIMARY KEY);`)}}
	set, err := migrations.NewSet(migrations.Source{Owner: "orders", FS: v1})
	if err != nil {
		t.Fatal(err)
	}
	status, err := set.Baseline(testDB)
	if err != nil {
		t.Fatal(err)
	}
	if len(status.AppliedDownstream) != 1 || len(status.PendingDownstream) != 0 {
		t.Fatalf("unexpected baseline status: %#v", status)
	}
	if testDB.Migrator().HasTable("downstream_v1") {
		t.Fatal("baseline must not execute downstream SQL")
	}

	v2 := fstest.MapFS{
		"0001_init.sql": {Data: []byte(`CREATE TABLE downstream_v1 (id integer PRIMARY KEY);`)},
		"0002_next.sql": {Data: []byte(`CREATE TABLE downstream_v2 (id integer PRIMARY KEY);`)},
	}
	set, err = migrations.NewSet(migrations.Source{Owner: "orders", FS: v2})
	if err != nil {
		t.Fatal(err)
	}
	status, err = set.Up(testDB)
	if err != nil {
		t.Fatal(err)
	}
	if !testDB.Migrator().HasTable("downstream_v2") || len(status.AppliedDownstream) != 2 {
		t.Fatalf("downstream up did not apply v2: %#v", status)
	}

	mutated := fstest.MapFS{
		"0001_init.sql": {Data: []byte(`SELECT 'changed';`)},
		"0002_next.sql": {Data: []byte(`CREATE TABLE downstream_v2 (id integer PRIMARY KEY);`)},
	}
	set, err = migrations.NewSet(migrations.Source{Owner: "orders", FS: mutated})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := set.Inspect(testDB); err == nil || !strings.Contains(err.Error(), "orders:0001") || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("expected owner-qualified checksum failure, got %v", err)
	}
}
