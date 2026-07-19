package tests

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/aiagent"
	"github.com/yzxingtu2026/ainp/platform/framework/aimemory"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodel"
	"github.com/yzxingtu2026/ainp/platform/framework/aitool"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	"github.com/yzxingtu2026/ainp/platform/modules/ai"
	"github.com/yzxingtu2026/ainp/platform/modules/system"
)

type memoryPolicyResolver struct {
	requirements []aimodel.CapabilityRequirements
}

func TestAgentMemoryPolicyMigrationPostgres(t *testing.T) {
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	admin, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "ainp_memory_policy_" + uuid.NewString()[:8]
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
	if err = ai.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile("../migrations/0030_agent_memory_policy.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(string(body)).Error; err != nil {
		t.Fatal(err)
	}
	var count int64
	if err = db.Raw(`SELECT COUNT(*) FROM pg_constraint WHERE conname='ck_ai_agent_memory_policy_source'`).Scan(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("memory policy source constraints = %d, want 1", count)
	}
}

func (r *memoryPolicyResolver) Resolve(_ context.Context, alias string, req aimodel.CapabilityRequirements) (*aimodel.ResolvedModel, error) {
	r.requirements = append(r.requirements, req)
	return &aimodel.ResolvedModel{DeploymentID: "d-" + alias, Limits: aimodel.Limits{ContextWindow: 8192}}, nil
}

func TestMemoryPolicyOverrideIsFrozenAndRequiresStructuredOutput(t *testing.T) {
	resolver := &memoryPolicyResolver{}
	svc := ai.NewAgentService(nil, aiagent.NewRegistry(), resolver, aitool.NewRegistry())
	ctx := auth.WithContext(context.Background(), &auth.Context{Authenticated: true, Realm: auth.RealmTenant, TenantID: "tenant-a", UserID: "user-a"})
	p := aimemory.SystemDefaultPolicy()
	p.Enabled = true
	p.ExtractionModelAlias = "memory"
	p.ConsolidationModelAlias = ""
	effective, source, err := svc.ResolveMemoryPolicy(ctx, &p)
	if err != nil {
		t.Fatal(err)
	}
	if source != "agent_version" || effective.ConsolidationModelAlias != "memory" {
		t.Fatalf("unexpected effective policy: source=%s policy=%#v", source, effective)
	}
	if len(resolver.requirements) != 1 {
		t.Fatalf("resolved models = %d, want 1", len(resolver.requirements))
	}
	for _, req := range resolver.requirements {
		if len(req.Features) != 1 || req.Features[0] != aimodel.FeatureStructuredOutput {
			t.Fatalf("requirements = %#v", req)
		}
	}
}

func TestAgentMemoryPolicyMigrationContract(t *testing.T) {
	body, err := os.ReadFile("../migrations/0030_agent_memory_policy.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"draft_memory_policy", "memory_policy jsonb NOT NULL", "memory_policy_source", "agent_version", "tenant_default", "system_default"} {
		if !strings.Contains(string(body), expected) {
			t.Fatalf("migration missing %q", expected)
		}
	}
}
