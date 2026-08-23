package tests

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	"github.com/yzxingtu2026/ainp/platform/modules/system"
)

func TestPrincipalSessionMigrationContract(t *testing.T) {
	body, err := os.ReadFile("../migrations/0058_principal_sessions.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"principal_kind", "principal_id", "client_surface", "audience", "token_family_id", "ALTER COLUMN account_id DROP NOT NULL", "ck_auth_sessions_account_principal", "auth_refresh_token_uses"} {
		if !strings.Contains(string(body), expected) {
			t.Fatalf("principal session migration is missing %q", expected)
		}
	}
}

func TestAsyncPrincipalContextMigrationContract(t *testing.T) {
	body, err := os.ReadFile("../migrations/0059_async_principal_context.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"outbox_events", "job_runs", "principal_kind", "client_surface", "audience", "idx_outbox_principal", "idx_job_runs_principal"} {
		if !strings.Contains(string(body), expected) {
			t.Fatalf("async principal context migration is missing %q", expected)
		}
	}
}

func TestPrincipalSessionMigrationPostgres(t *testing.T) {
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	admin, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "ainp_principal_session_migration_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err = admin.Exec(fmt.Sprintf(`CREATE SCHEMA %q`, schema)).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Exec(fmt.Sprintf(`DROP SCHEMA %q CASCADE`, schema)).Error })
	db, err := database.Open(dsn + " search_path=" + schema)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE tenant_accounts (id varchar(64) PRIMARY KEY, tenant_id varchar(64) NOT NULL, UNIQUE(tenant_id,id))`,
		`CREATE TABLE auth_sessions (id varchar(64) PRIMARY KEY, tenant_id varchar(64) NOT NULL, account_id varchar(64) NOT NULL, realm varchar(32) NOT NULL, client varchar(64) NOT NULL, device_summary varchar(255) NOT NULL DEFAULT '', refresh_token_hash varchar(64) NOT NULL UNIQUE, previous_refresh_token_hash varchar(64) NOT NULL DEFAULT '', auth_version bigint NOT NULL, expires_at timestamptz NOT NULL, last_seen_at timestamptz NOT NULL, revoked_at timestamptz, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), CONSTRAINT fk_auth_sessions_account_tenant FOREIGN KEY(tenant_id,account_id) REFERENCES tenant_accounts(tenant_id,id))`,
		`INSERT INTO tenant_accounts(id,tenant_id) VALUES ('account-a','tenant-a')`,
		`INSERT INTO auth_sessions(id,tenant_id,account_id,realm,client,refresh_token_hash,auth_version,expires_at,last_seen_at) VALUES ('session-a','tenant-a','account-a','tenant','web','hash-a',7,now()+interval '1 day',now())`,
	} {
		if err = db.Exec(statement).Error; err != nil {
			t.Fatalf("prepare schema: %v\n%s", err, statement)
		}
	}
	body, err := os.ReadFile("../migrations/0058_principal_sessions.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(string(body)).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(string(body)).Error; err != nil {
		t.Fatalf("migration must be idempotent: %v", err)
	}
	var count int64
	if err = db.Raw(`SELECT count(*) FROM auth_sessions WHERE id='session-a' AND principal_kind='ainp.account' AND principal_id='account-a' AND client_surface='ainp.tenant-console' AND audience='ainp:tenant-console' AND token_family_id='session-a'`).Scan(&count).Error; err != nil || count != 1 {
		t.Fatalf("legacy session not preserved: count=%d err=%v", count, err)
	}
	if err = db.Exec(`INSERT INTO auth_sessions(id,tenant_id,principal_kind,principal_id,realm,client,client_surface,audience,token_family_id,refresh_token_hash,auth_version,expires_at,last_seen_at) VALUES ('session-c','tenant-a','example.customer','customer-1','tenant','mobile','example.mobile','example:mobile','family-c','hash-c',1,now()+interval '1 day',now())`).Error; err != nil {
		t.Fatalf("custom principal session rejected: %v", err)
	}
	if err = db.Exec(`INSERT INTO auth_sessions(id,tenant_id,principal_kind,principal_id,account_id,realm,client,client_surface,audience,token_family_id,refresh_token_hash,auth_version,expires_at,last_seen_at) VALUES ('bad','tenant-a','example.customer','customer-1','account-a','tenant','mobile','example.mobile','example:mobile','bad','hash-bad',1,now()+interval '1 day',now())`).Error; err == nil {
		t.Fatal("custom principal must not reuse account_id")
	}
}

func TestSystemAutoMigrateBackfillsLegacyAccountSessions(t *testing.T) {
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	admin, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "ainp_principal_session_auto_migrate_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err = admin.Exec(fmt.Sprintf(`CREATE SCHEMA %q`, schema)).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Exec(fmt.Sprintf(`DROP SCHEMA %q CASCADE`, schema)).Error })
	db, err := database.Open(dsn + " search_path=" + schema)
	if err != nil {
		t.Fatal(err)
	}
	if err = system.AutoMigrate(db); err != nil {
		t.Fatalf("create current schema: %v", err)
	}
	for _, statement := range []string{
		`DROP TABLE auth_refresh_token_uses`,
		`ALTER TABLE auth_sessions DROP COLUMN principal_kind CASCADE, DROP COLUMN principal_id CASCADE, DROP COLUMN client_surface CASCADE, DROP COLUMN audience CASCADE, DROP COLUMN token_family_id CASCADE, ALTER COLUMN account_id SET NOT NULL`,
		`INSERT INTO tenants(id,name,status) VALUES ('tenant-a','A','enabled')`,
		`INSERT INTO tenant_accounts(id,tenant_id,username,password_hash,status,auth_version) VALUES ('account-a','tenant-a','admin','hash','enabled',7)`,
		`INSERT INTO auth_sessions(id,tenant_id,account_id,realm,client,refresh_token_hash,auth_version,expires_at,last_seen_at) VALUES ('session-a','tenant-a','account-a','tenant','web','hash-a',7,now()+interval '1 day',now())`,
	} {
		if err = db.Exec(statement).Error; err != nil {
			t.Fatalf("prepare legacy session schema: %v\n%s", err, statement)
		}
	}
	if err = system.AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate must backfill legacy Account sessions before adding NOT NULL columns: %v", err)
	}
	var count int64
	if err = db.Raw(`SELECT count(*) FROM auth_sessions WHERE id='session-a' AND principal_kind='ainp.account' AND principal_id='account-a' AND client_surface='ainp.tenant-console' AND audience='ainp:tenant-console' AND token_family_id='session-a'`).Scan(&count).Error; err != nil || count != 1 {
		t.Fatalf("legacy session not preserved by AutoMigrate: count=%d err=%v", count, err)
	}
}
