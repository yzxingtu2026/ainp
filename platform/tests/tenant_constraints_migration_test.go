package tests

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	"github.com/yzxingtu2026/ainp/platform/framework/job"
	"github.com/yzxingtu2026/ainp/platform/framework/outbox"
	"github.com/yzxingtu2026/ainp/platform/modules/ai"
	"github.com/yzxingtu2026/ainp/platform/modules/system"
)

func TestTenantCompositeConstraintMigrationContract(t *testing.T) {
	body, err := os.ReadFile("../migrations/0028_tenant_composite_constraints.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(body)
	for _, expected := range []string{
		"ux_users_tenant_id",
		"ux_ai_agents_tenant_id",
		"fk_user_roles_user_tenant",
		"fk_auth_sessions_user_tenant",
		"fk_ai_agent_versions_agent_tenant",
		"fk_ai_conversations_version_tenant",
		"fk_ai_messages_conversation_tenant",
		"fk_ai_stream_deltas_stream_tenant",
		"fk_ai_tool_confirmations_scope_tenant",
		"fk_ai_runs_trigger_tenant",
		"fk_ai_steps_run_tenant",
		"fk_ai_run_events_step_tenant",
	} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("tenant composite migration is missing %s", expected)
		}
	}
	if strings.Contains(sql, "REFERENCES ai_agents (id)") || strings.Contains(sql, "REFERENCES ai_conversations (id)") {
		t.Fatal("tenant composite migration must not introduce single-column tenant-owned references")
	}
}

func TestTenantCompositeConstraintMigrationPostgres(t *testing.T) {
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	admin, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "ainp_tenant_constraints_" + uuid.NewString()[:8]
	if err = admin.Exec(fmt.Sprintf(`CREATE SCHEMA %q`, schema)).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Exec(fmt.Sprintf(`DROP SCHEMA %q CASCADE`, schema)).Error })
	db, err := database.Open(dsn + " search_path=" + schema)
	if err != nil {
		t.Fatal(err)
	}
	if err = system.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	if err = outbox.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	if err = job.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	if err = ai.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile("../migrations/0028_tenant_composite_constraints.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(string(body)).Error; err != nil {
		t.Fatal(err)
	}

	var count int64
	if err = db.Raw(`SELECT COUNT(*) FROM pg_constraint
WHERE conname IN ('fk_ai_agent_versions_agent_tenant','fk_ai_conversations_version_tenant','fk_ai_runs_trigger_tenant','fk_user_roles_user_tenant')`).Scan(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 4 {
		t.Fatalf("tenant composite constraints installed = %d, want 4", count)
	}

	if err = db.Exec(`INSERT INTO tenants (id,name,status) VALUES ('tenant-a','A','enabled'),('tenant-b','B','enabled')`).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(`INSERT INTO users (id,tenant_id,username,password_hash,status,auth_version) VALUES ('user-a','tenant-a','a','x','enabled',1)`).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(`INSERT INTO ai_agents (id,tenant_id,code,name,description,source,visibility,status,current_version,revision) VALUES ('agent-b','tenant-b','b','B','','tenant','tenant','draft',0,1)`).Error; err != nil {
		t.Fatal(err)
	}
	err = db.Exec(`INSERT INTO ai_agent_versions (id,tenant_id,agent_id,version,kind,content_hash,model_alias,tenant_instructions,generation_config,tool_policy,created_by)
VALUES ('version-cross','tenant-a','agent-b',1,'release','','default','','{}','{}','user-a')`).Error
	if err == nil {
		t.Fatal("composite foreign key accepted a cross-tenant agent version")
	}
	if !strings.Contains(err.Error(), "fk_ai_agent_versions_agent_tenant") {
		t.Fatalf("cross-tenant version failed for the wrong reason: %v", err)
	}
}
