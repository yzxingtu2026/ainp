package tests

import (
	"fmt"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/audit"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	"github.com/yzxingtu2026/ainp/platform/migrations"
	"github.com/yzxingtu2026/ainp/platform/modules/ai"
	"github.com/yzxingtu2026/ainp/platform/modules/system"
)

func TestMigrationRunnerAppliesCurrentSchemaOnce(t *testing.T) {
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	root, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "ainp_migrations_" + uuid.NewString()[:8]
	if err := root.Exec(fmt.Sprintf(`CREATE SCHEMA %q`, schema)).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Exec(fmt.Sprintf(`DROP SCHEMA %q CASCADE`, schema)).Error })
	db, err := database.Open(dsn + " search_path=" + schema)
	if err != nil {
		t.Fatal(err)
	}
	if err := audit.NewGormRecorder(db).AutoMigrate(); err != nil {
		t.Fatal(err)
	}
	if err := system.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	if err := ai.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}

	first, err := migrations.Baseline(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Pending) != 0 || len(first.Applied) == 0 {
		t.Fatalf("unexpected first migration status: %#v", first)
	}
	repeatedBaseline, err := migrations.Baseline(db)
	if err != nil || len(repeatedBaseline.Pending) != 0 || len(repeatedBaseline.Applied) != len(first.Applied) {
		t.Fatalf("baseline retry was not idempotent: %#v %v", repeatedBaseline, err)
	}
	second, err := migrations.Up(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Pending) != 0 || len(second.Applied) != len(first.Applied) {
		t.Fatalf("migration rerun was not idempotent: %#v", second)
	}
	if err := db.Exec("UPDATE schema_migrations SET checksum = 'tampered' WHERE version = (SELECT MIN(version) FROM schema_migrations)").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := migrations.Inspect(db); err == nil {
		t.Fatal("tampered migration history must fail closed")
	}
}
