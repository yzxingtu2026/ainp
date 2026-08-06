package host

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	"gorm.io/gorm"
)

func TestEnsureIdentityContractSchemaRejectsLegacyPersonalizationColumn(t *testing.T) {
	db := openSchemaPreflightTestDB(t)
	if err := db.Exec(`CREATE TABLE ai_assistant_personalization_snapshots (
		id text PRIMARY KEY,
		user_id varchar(64) NOT NULL
	)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO ai_assistant_personalization_snapshots VALUES ('snapshot-a', 'account-a')`).Error; err != nil {
		t.Fatal(err)
	}

	err := ensureIdentityContractSchema(db)
	if err == nil || !strings.Contains(err.Error(), "ai_assistant_personalization_snapshots.user_id") ||
		!strings.Contains(err.Error(), "run versioned migrations before AutoMigrate") {
		t.Fatalf("unexpected schema preflight error: %v", err)
	}
	if db.Migrator().HasColumn("ai_assistant_personalization_snapshots", "account_id") {
		t.Fatal("schema preflight must not add account_id before the versioned migration")
	}
}

func TestEnsureIdentityContractSchemaAcceptsCanonicalPersonalizationColumn(t *testing.T) {
	db := openSchemaPreflightTestDB(t)
	if err := db.Exec(`CREATE TABLE ai_assistant_personalization_snapshots (
		id text PRIMARY KEY,
		account_id varchar(64) NOT NULL
	)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := ensureIdentityContractSchema(db); err != nil {
		t.Fatalf("canonical schema rejected: %v", err)
	}
}

func openSchemaPreflightTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	db, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "ainp_boot_preflight_" + strings.ReplaceAll(uuid.NewString()[:8], "-", "")
	if err = db.Exec(fmt.Sprintf(`CREATE SCHEMA %q`, schema)).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Exec(fmt.Sprintf(`DROP SCHEMA %q CASCADE`, schema)).Error })
	testDB, err := database.Open(dsn + " search_path=" + schema)
	if err != nil {
		t.Fatal(err)
	}
	return testDB
}
