package tests

import (
	"fmt"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	"gorm.io/gorm"
)

func TestIdentityContractMigrationCanonicalizesHistoryAndIsIdempotent(t *testing.T) {
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	db, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "ainp_identity_" + uuid.NewString()[:8]
	if err = db.Exec(fmt.Sprintf(`CREATE SCHEMA %q`, schema)).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Exec(fmt.Sprintf(`DROP SCHEMA %q CASCADE`, schema)).Error })
	testDB, err := database.Open(dsn + " search_path=" + schema)
	if err != nil {
		t.Fatal(err)
	}

	for _, statement := range []string{
		`CREATE TABLE role_permissions (id text PRIMARY KEY, role_id text NOT NULL, permission text NOT NULL)`,
		`CREATE TABLE audit_logs (id text PRIMARY KEY, user_id text NOT NULL, actor_id text)`,
		`CREATE INDEX idx_audit_logs_user_id ON audit_logs(user_id)`,
		`CREATE INDEX idx_audit_logs_actor_id ON audit_logs(actor_id)`,
		`CREATE TABLE idempotency_records (id text PRIMARY KEY, user_id text NOT NULL)`,
		`CREATE TABLE ai_tool_scopes (id text PRIMARY KEY, user_id text NOT NULL)`,
		`CREATE INDEX idx_ai_tool_scopes_actor ON ai_tool_scopes(user_id)`,
		`CREATE TABLE ai_tool_confirmations (id text PRIMARY KEY, user_id text NOT NULL)`,
		`CREATE INDEX idx_ai_tool_confirmations_actor_status ON ai_tool_confirmations(user_id)`,
		`CREATE TABLE ai_tool_invocations (id text PRIMARY KEY, user_id text NOT NULL)`,
		`CREATE INDEX idx_ai_tool_invocations_actor_started ON ai_tool_invocations(user_id)`,
		`CREATE TABLE mcp_oauth_grants (id text PRIMARY KEY, user_id text NOT NULL)`,
		`CREATE TABLE mcp_action_confirmations (id text PRIMARY KEY, user_id text NOT NULL)`,
		`CREATE TABLE ai_knowledge_bases (id text PRIMARY KEY, owner_user_id text NOT NULL)`,
		`CREATE TABLE user_setting_values (id text PRIMARY KEY, user_id text NOT NULL)`,
		`CREATE TABLE ai_assistant_personalization_snapshots (id text PRIMARY KEY, user_id text NOT NULL)`,
		`CREATE TABLE user_notifications (id text PRIMARY KEY, user_id text NOT NULL)`,
		`CREATE TABLE attachment_links (tenant_id text NOT NULL, attachment_id text NOT NULL, resource_type text NOT NULL, resource_id text NOT NULL, field text NOT NULL)`,
		`INSERT INTO role_permissions VALUES ('old-only','role-a','system.user_list'), ('old-conflict','role-b','system.user_list'), ('canonical','role-b','system.account_list')`,
		`INSERT INTO audit_logs (id, user_id) VALUES ('audit-a','account-a')`,
		`INSERT INTO idempotency_records VALUES ('idem-a','account-a')`,
		`INSERT INTO ai_tool_scopes VALUES ('scope-a','account-a')`,
		`INSERT INTO ai_tool_confirmations VALUES ('tool-confirm-a','account-a')`,
		`INSERT INTO ai_tool_invocations VALUES ('tool-invoke-a','account-a')`,
		`INSERT INTO mcp_oauth_grants VALUES ('grant-a','account-a')`,
		`INSERT INTO mcp_action_confirmations VALUES ('mcp-confirm-a','account-a')`,
		`INSERT INTO ai_knowledge_bases VALUES ('kb-a','account-a')`,
		`INSERT INTO user_setting_values VALUES ('setting-a','account-a')`,
		`INSERT INTO ai_assistant_personalization_snapshots VALUES ('personal-a','account-a')`,
		`INSERT INTO user_notifications VALUES ('notification-a','account-a')`,
		`INSERT INTO attachment_links VALUES ('tenant-a','attachment-a','system.user_profile','account-a','avatar'), ('tenant-a','attachment-a','system.account_profile','account-a','avatar')`,
	} {
		if err = testDB.Exec(statement).Error; err != nil {
			t.Fatalf("prepare identity migration: %v\n%s", err, statement)
		}
	}

	applyTestMigration(t, testDB, "0055_identity_contract.sql")
	applyTestMigration(t, testDB, "0055_identity_contract.sql")

	for _, columns := range [][3]string{
		{"audit_logs", "user_id", "actor_id"},
		{"idempotency_records", "user_id", "principal_id"},
		{"ai_tool_scopes", "user_id", "principal_id"},
		{"ai_tool_confirmations", "user_id", "principal_id"},
		{"ai_tool_invocations", "user_id", "principal_id"},
		{"mcp_oauth_grants", "user_id", "account_id"},
		{"mcp_action_confirmations", "user_id", "account_id"},
		{"ai_knowledge_bases", "owner_user_id", "account_id"},
		{"user_setting_values", "user_id", "account_id"},
		{"ai_assistant_personalization_snapshots", "user_id", "account_id"},
		{"user_notifications", "user_id", "account_id"},
	} {
		if testDB.Migrator().HasColumn(columns[0], columns[1]) || !testDB.Migrator().HasColumn(columns[0], columns[2]) {
			t.Errorf("identity column migration incomplete for %s: old=%s new=%s", columns[0], columns[1], columns[2])
		}
	}
	assertIdentityValue(t, testDB, "audit_logs", "actor_id", "audit-a")
	assertIdentityValue(t, testDB, "idempotency_records", "principal_id", "idem-a")
	assertIdentityValue(t, testDB, "mcp_oauth_grants", "account_id", "grant-a")
	assertIdentityValue(t, testDB, "ai_knowledge_bases", "account_id", "kb-a")
	var auditActorNullable string
	if err = testDB.Raw(`SELECT is_nullable FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'audit_logs' AND column_name = 'actor_id'`).Scan(&auditActorNullable).Error; err != nil {
		t.Fatal(err)
	}
	if auditActorNullable != "NO" {
		t.Fatalf("audit_logs.actor_id nullable = %q, want NO", auditActorNullable)
	}

	var permissions []string
	if err = testDB.Raw(`SELECT permission FROM role_permissions ORDER BY role_id, permission`).Scan(&permissions).Error; err != nil {
		t.Fatal(err)
	}
	if len(permissions) != 2 || permissions[0] != "system.account_list" || permissions[1] != "system.account_list" {
		t.Fatalf("unexpected canonical permissions: %#v", permissions)
	}
	var profileLinks int64
	if err = testDB.Table("attachment_links").Where("resource_type = ?", "system.account_profile").Count(&profileLinks).Error; err != nil || profileLinks != 1 {
		t.Fatalf("canonical profile links = %d, err=%v", profileLinks, err)
	}
}

func assertIdentityValue(t *testing.T, db *gorm.DB, table, column, id string) {
	t.Helper()
	var value string
	if err := db.Raw(fmt.Sprintf(`SELECT %s FROM %s WHERE id = ?`, column, table), id).Scan(&value).Error; err != nil {
		t.Fatal(err)
	}
	if value != "account-a" {
		t.Fatalf("%s.%s history = %q, want account-a", table, column, value)
	}
}
