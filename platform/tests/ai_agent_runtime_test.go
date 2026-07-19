package tests

import (
	"os"
	"strings"
	"testing"

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
	for _, expected := range []string{"SlotIDs()", "SlotMetadata(toolID)", "metadata.Observations"} {
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
	for _, expected := range []string{"DirectSlotIDs()", "s.registry.Project", "ExecutorFor", "SlotMetadata"} {
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
