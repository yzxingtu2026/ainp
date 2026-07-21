package tests

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	"github.com/yzxingtu2026/ainp/platform/modules/ai"
	"github.com/yzxingtu2026/ainp/platform/modules/system"

	"github.com/yzxingtu2026/ainp/platform/framework/aiagentruntime"
)

func TestAgentRunStateMachineRejectsIllegalTransitions(t *testing.T) {
	valid := [][2]aiagentruntime.RunStatus{{aiagentruntime.RunQueued, aiagentruntime.RunRunning}, {aiagentruntime.RunRunning, aiagentruntime.RunWaitingConfirmation}, {aiagentruntime.RunWaitingConfirmation, aiagentruntime.RunQueued}, {aiagentruntime.RunFailed, aiagentruntime.RunQueued}}
	for _, pair := range valid {
		if err := aiagentruntime.ValidateRunTransition(pair[0], pair[1]); err != nil {
			t.Fatalf("valid transition rejected: %v", err)
		}
	}
	for _, pair := range [][2]aiagentruntime.RunStatus{{aiagentruntime.RunCompleted, aiagentruntime.RunRunning}, {aiagentruntime.RunCancelled, aiagentruntime.RunQueued}, {aiagentruntime.RunQueued, aiagentruntime.RunCompleted}, {aiagentruntime.RunWaitingConfirmation, aiagentruntime.RunCompleted}} {
		if err := aiagentruntime.ValidateRunTransition(pair[0], pair[1]); err == nil {
			t.Fatalf("invalid transition accepted: %s -> %s", pair[0], pair[1])
		}
	}
}

func TestTrustedRuntimeContextMigrationPostgres(t *testing.T) {
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	admin, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "ainp_runtime_migration_" + uuid.NewString()[:8]
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
	body, err := os.ReadFile("../migrations/0034_trusted_runtime_context.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(string(body)).Error; err != nil {
		t.Fatal(err)
	}
	var count int64
	if err = db.Raw(`SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=current_schema() AND ((table_name='ai_agent_runs' AND column_name IN ('evaluated_at_utc','timezone','time_snapshot_json')) OR (table_name='ai_context_snapshots' AND column_name='runtime_context_refs'))`).Scan(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 4 {
		t.Fatalf("runtime context columns=%d, want 4", count)
	}
}

func TestAgentStepStateMachineAndBudget(t *testing.T) {
	if err := aiagentruntime.ValidateStepTransition(aiagentruntime.StepPending, aiagentruntime.StepRunning); err != nil {
		t.Fatal(err)
	}
	if err := aiagentruntime.ValidateStepTransition(aiagentruntime.StepCompleted, aiagentruntime.StepRunning); err == nil {
		t.Fatal("terminal step transitioned")
	}
	b := (aiagentruntime.Budget{}).Normalize()
	if err := b.Validate(); err != nil {
		t.Fatal(err)
	}
	b.MaxSteps = 101
	if err := b.Validate(); err == nil {
		t.Fatal("unsafe budget accepted")
	}
	for _, kind := range []aiagentruntime.StepType{aiagentruntime.StepContextAssemble, aiagentruntime.StepContextCompact, aiagentruntime.StepModel, aiagentruntime.StepToolSearch, aiagentruntime.StepToolLoad, aiagentruntime.StepToolExecute, aiagentruntime.StepConfirmation, aiagentruntime.StepFinalize} {
		if !aiagentruntime.ValidStepType(kind) {
			t.Fatalf("step type rejected: %s", kind)
		}
	}
}

func TestAgentRuntimeMigrationHasRecoveryConstraints(t *testing.T) {
	body, err := os.ReadFile("../migrations/0021_ai_agent_runtime.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(body)
	for _, expected := range []string{"UNIQUE(tenant_id,user_id,idempotency_key)", "lease_expires_at", "UNIQUE(tenant_id,run_id,step_index)", "UNIQUE(tenant_id,run_id,sequence)", "waiting_confirmation", "result_json", "ON DELETE RESTRICT"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("migration missing %s", expected)
		}
	}
}

func TestTrustedRuntimeContextMigrationStoresOnlyTimeAndRefs(t *testing.T) {
	body, err := os.ReadFile("../migrations/0034_trusted_runtime_context.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(body)
	for _, expected := range []string{"evaluated_at_utc", "timezone", "time_snapshot_json", "runtime_context_refs"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("runtime context migration missing %s", expected)
		}
	}
	for _, forbidden := range []string{"preferred_address", "memory_content", "prompt_text"} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("runtime context migration must not persist %s", forbidden)
		}
	}
}

func TestAgentRunOutboxEnqueueInjectsAuthFromEnvelope(t *testing.T) {
	body, err := os.ReadFile("../modules/ai/agent_runtime_register.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(body)
	for _, expected := range []string{
		`HandlerName: "ai.enqueue_agent_run"`,
		`auth.WithContext(ctx, &auth.Context{`,
		`TenantID:      env.TenantID`,
		`Client:        "outbox"`,
		`a.Jobs.Enqueuer.Enqueue(ctx,`,
	} {
		if !strings.Contains(src, expected) {
			t.Fatalf("agent run outbox handler missing %q", expected)
		}
	}
}

func TestAgentRuntimeUsesSlotMetadataWithoutBusinessToolBranches(t *testing.T) {
	body, err := os.ReadFile("../modules/ai/agent_runtime_service.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(body)
	for _, expected := range []string{"contract.RequiredToolSelectors", "registry.IsBindable(b.ToolID)", "SlotMetadata(toolID)", "metadata.Observations"} {
		if !strings.Contains(src, expected) {
			t.Fatalf("agent slot integration missing %q", expected)
		}
	}
	for _, forbidden := range []string{"knowledgeSearchToolID", `"builtin:knowledge.search"`, "knowledgeMessages(", "s.knowledge.Retrieve(ctx", "Instructions: aiknowledge.FormatUntrustedReferences"} {
		if strings.Contains(src, forbidden) {
			t.Fatalf("automatic or trusted knowledge injection remains: %q", forbidden)
		}
	}
}

func TestKnowledgeToolIsRegisteredAndKeepsChunkTextOutOfFacts(t *testing.T) {
	body, err := os.ReadFile("../modules/ai/knowledge_tool.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(body)
	for _, expected := range []string{`FormatUntrustedReferences`, `cacheKey`, `CanReadKnowledgeBase`} {
		if !strings.Contains(src, expected) {
			t.Fatalf("knowledge tool missing %q", expected)
		}
	}
	slot, err := os.ReadFile("../modules/ai/tools/knowledge/slot.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`"builtin:knowledge.search"`, `DiscoveryDirect`, `MaxCallsPerRun: 4`, `Observations`} {
		if !strings.Contains(string(slot), expected) {
			t.Fatalf("knowledge slot missing %q", expected)
		}
	}
	toolService, err := os.ReadFile("../modules/ai/tool_service.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(toolService), `result_json`) || !strings.Contains(string(toolService), `Summary: result.Summary`) {
		t.Fatal("tool invocation must persist a redacted result summary")
	}
	for _, expected := range []string{"scope.AllowedToolsJSON", "s.registry.Project", "ExecutorFor", "SlotMetadata"} {
		if !strings.Contains(string(toolService), expected) {
			t.Fatalf("tool service must consume slot registry through %q", expected)
		}
	}
	for _, forbidden := range []string{"knowledgeSearchToolID", `"builtin:knowledge.search"`, "SetDefinitionProjector"} {
		if strings.Contains(string(toolService), forbidden) {
			t.Fatalf("tool service contains business-specific slot branch %q", forbidden)
		}
	}
}

func TestMemorySearchIsSingleDirectSlotWithoutRuntimeIDBranch(t *testing.T) {
	slot, err := os.ReadFile("../modules/ai/tools/memory/slot.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(slot)
	for _, expected := range []string{`"builtin:memory.search"`, `DiscoveryDirect`, `MaxCallsPerRun: 3`, `memory_retrieval`} {
		if !strings.Contains(src, expected) {
			t.Fatalf("memory slot missing %q", expected)
		}
	}
	register, err := os.ReadFile("../modules/ai/register.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(register), "memorytool.New(") != 1 {
		t.Fatal("memory search slot must be registered exactly once")
	}
	toolService, err := os.ReadFile("../modules/ai/tool_service.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(toolService), `"builtin:memory.search"`) {
		t.Fatal("ToolService must not branch on memory tool ID")
	}
}
