package tests

import (
	"strings"
	"testing"

	"github.com/yzxingtu2026/ainp/platform/migrations"
)

func TestAgentRuntimePrincipalContextMigration(t *testing.T) {
	migration, err := migrations.Find(61)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"ai_agent_runs ADD COLUMN IF NOT EXISTS principal_kind",
		"ai_agent_runs ADD COLUMN IF NOT EXISTS client_surface",
		"ai_agent_runs ADD COLUMN IF NOT EXISTS audience",
		"ai_tool_scopes ADD COLUMN IF NOT EXISTS principal_kind",
		"CREATE UNIQUE INDEX ux_ai_run_idem ON ai_agent_runs (tenant_id, user_id, idempotency_key, principal_kind)",
	} {
		if !strings.Contains(migration.SQL, want) {
			t.Fatalf("migration 0061 missing %q", want)
		}
	}
}
