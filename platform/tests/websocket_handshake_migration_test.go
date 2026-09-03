package tests

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	"github.com/yzxingtu2026/ainp/platform/migrations"
	"github.com/yzxingtu2026/ainp/platform/modules/system"
)

func TestWebSocketHandshakeMigrationContract(t *testing.T) {
	body, err := os.ReadFile("../migrations/0067_websocket_handshake_grants.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(body)
	for _, required := range []string{
		"auth_websocket_handshake_grants", "secret_hash", "principal_kind", "principal_id", "session_id",
		"client_surface", "audience", "target_id", "target_path", "auth_version", "expires_at", "consumed_at",
		"REFERENCES auth_sessions(id)", "CHECK (auth_version > 0)",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("migration missing %q", required)
		}
	}
}

func TestWebSocketHandshakeMigrationThenAutoMigratePostgres(t *testing.T) {
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	root, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "ainp_websocket_migration_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err = root.Exec(fmt.Sprintf(`CREATE SCHEMA %q`, schema)).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Exec(fmt.Sprintf(`DROP SCHEMA %q CASCADE`, schema)).Error })
	db, err := database.Open(dsn + " search_path=" + schema)
	if err != nil {
		t.Fatal(err)
	}

	// Build the pre-0067 installed schema, then exercise the published SQL
	// migration path before running the startup AutoMigrate path.
	if err = system.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	if err = db.Migrator().DropTable("auth_websocket_handshake_grants"); err != nil {
		t.Fatal(err)
	}
	if _, err = migrations.BaselineThrough(db, 66); err != nil {
		t.Fatal(err)
	}
	status, err := migrations.Up(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Pending) != 0 {
		t.Fatalf("pending migrations after upgrade = %d, want 0", len(status.Pending))
	}
	for i := 0; i < 2; i++ {
		if err = system.AutoMigrate(db); err != nil {
			t.Fatalf("AutoMigrate attempt %d after versioned migrations: %v", i+1, err)
		}
	}

	var indexCount int64
	if err = db.Raw(`SELECT count(*) FROM pg_indexes
WHERE schemaname = current_schema()
  AND tablename = 'auth_websocket_handshake_grants'
  AND indexname = 'uni_auth_websocket_handshake_grants_secret_hash'
  AND indexdef LIKE 'CREATE UNIQUE INDEX %'`).Scan(&indexCount).Error; err != nil {
		t.Fatal(err)
	}
	if indexCount != 1 {
		t.Fatalf("named unique index count = %d, want 1", indexCount)
	}

	var legacyConstraintCount int64
	if err = db.Raw(`SELECT count(*) FROM information_schema.table_constraints
WHERE constraint_schema = current_schema()
  AND table_name = 'auth_websocket_handshake_grants'
  AND constraint_name = 'auth_websocket_handshake_grants_secret_hash_key'`).Scan(&legacyConstraintCount).Error; err != nil {
		t.Fatal(err)
	}
	if legacyConstraintCount != 0 {
		t.Fatalf("legacy secret_hash constraint count = %d, want 0", legacyConstraintCount)
	}
}
