package tests

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	"github.com/yzxingtu2026/ainp/platform/migrations"
)

func TestMCPPrincipalContextMigration(t *testing.T) {
	migration, err := migrations.Find(64)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"mcp_oauth_grants", "mcp_action_confirmations", "principal_kind", "client_surface", "audience", "status = 'revoked'"} {
		if !strings.Contains(migration.SQL, want) {
			t.Fatalf("migration 0064 missing %q", want)
		}
	}
}

func TestMCPPrincipalContextMigrationPostgres(t *testing.T) {
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	admin, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "ainp_mcp_principal_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err = admin.Exec(fmt.Sprintf(`CREATE SCHEMA %q`, schema)).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Exec(fmt.Sprintf(`DROP SCHEMA %q CASCADE`, schema)).Error })
	db, err := database.Open(dsn + " search_path=" + schema)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE mcp_oauth_grants (id varchar(64) PRIMARY KEY, tenant_id varchar(64) NOT NULL, account_id varchar(64) NOT NULL, status varchar(16) NOT NULL)`,
		`CREATE TABLE mcp_action_confirmations (id varchar(64) PRIMARY KEY, tenant_id varchar(64) NOT NULL, account_id varchar(64) NOT NULL, status varchar(16) NOT NULL, arguments_ciphertext text NOT NULL DEFAULT '')`,
		`INSERT INTO mcp_oauth_grants VALUES ('grant-legacy','tenant-a','account-a','active')`,
		`INSERT INTO mcp_action_confirmations VALUES ('pending-legacy','tenant-a','account-a','pending','encrypted')`,
		`INSERT INTO mcp_action_confirmations VALUES ('consumed-legacy','tenant-a','account-a','consumed','')`,
	} {
		if err = db.Exec(statement).Error; err != nil {
			t.Fatalf("prepare pre-0064 schema: %v\n%s", err, statement)
		}
	}
	applyTestMigration(t, db, "0064_mcp_principal_context.sql")
	if err = db.Exec(`INSERT INTO mcp_action_confirmations(id,tenant_id,account_id,principal_kind,client_surface,audience,status,arguments_ciphertext) VALUES ('approved-current','tenant-a','account-a','ainp.account','ainp.tenant-console','ainp:tenant-console','approved','current')`).Error; err != nil {
		t.Fatal(err)
	}
	applyTestMigration(t, db, "0064_mcp_principal_context.sql")

	var count int64
	if err = db.Raw(`SELECT count(*) FROM mcp_oauth_grants WHERE id='grant-legacy' AND principal_kind='ainp.account' AND client_surface='ainp.tenant-console' AND audience='ainp:tenant-console'`).Scan(&count).Error; err != nil || count != 1 {
		t.Fatalf("legacy MCP Grant was not backfilled: count=%d err=%v", count, err)
	}
	if err = db.Raw(`SELECT count(*) FROM mcp_action_confirmations WHERE id='pending-legacy' AND status='revoked' AND arguments_ciphertext=''`).Scan(&count).Error; err != nil || count != 1 {
		t.Fatalf("legacy actionable confirmation was not revoked: count=%d err=%v", count, err)
	}
	if err = db.Raw(`SELECT count(*) FROM mcp_action_confirmations WHERE id='consumed-legacy' AND status='consumed' AND principal_kind='ainp.account'`).Scan(&count).Error; err != nil || count != 1 {
		t.Fatalf("terminal legacy confirmation was not preserved: count=%d err=%v", count, err)
	}
	if err = db.Raw(`SELECT count(*) FROM mcp_action_confirmations WHERE id='approved-current' AND status='approved' AND arguments_ciphertext='current'`).Scan(&count).Error; err != nil || count != 1 {
		t.Fatalf("idempotent migration revoked a current confirmation: count=%d err=%v", count, err)
	}
}
