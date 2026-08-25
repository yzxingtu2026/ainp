package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodelruntime"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	tenantdata "github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"github.com/yzxingtu2026/ainp/platform/modules/ai"
	"github.com/yzxingtu2026/ainp/platform/modules/ai/tooling"
	"github.com/yzxingtu2026/ainp/platform/modules/system"

	"github.com/yzxingtu2026/ainp/platform/framework/aiagentruntime"
)

func TestConfirmationApproveRejectsExternalConversationChangesPostgres(t *testing.T) {
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	admin, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "ainp_confirmation_revision_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err = admin.Exec(fmt.Sprintf(`CREATE SCHEMA %q`, schema)).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Exec(fmt.Sprintf(`DROP SCHEMA %q CASCADE`, schema)).Error })
	db, err := database.Open(dsn + " search_path=" + schema)
	if err != nil {
		t.Fatal(err)
	}
	if err = ai.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	fixtureDB, err := tenantdata.MigrationDB(db)
	if err != nil {
		t.Fatal(err)
	}
	conversation := ai.Conversation{ID: "conversation-confirmation", TenantID: "tenant-a", OwnerUserID: "user-a", Title: "Confirmation", AgentID: "agent-a", AgentVersionID: "version-a", Purpose: ai.ConversationPurposeWorkspace, Visibility: "private", Status: "active", Revision: 4}
	if err = fixtureDB.Create(&conversation).Error; err != nil {
		t.Fatal(err)
	}
	runID, conversationID := "run-confirmation", conversation.ID
	run := ai.AgentRun{ID: runID, TenantID: "tenant-a", UserID: "user-a", ConversationID: conversation.ID, ConversationRevision: conversation.Revision, TriggerMessageID: "message-a", AgentVersionID: "version-a", SessionID: "session-a", AuthVersion: 1, Status: "waiting_confirmation", BudgetJSON: json.RawMessage(`{}`), IdempotencyKey: "run-confirmation", Revision: 1}
	runDB, _ := tenantdata.MigrationDB(db)
	if err = runDB.Create(&run).Error; err != nil {
		t.Fatal(err)
	}
	scope := tooling.ToolScope{ID: "scope-confirmation", TenantID: "tenant-a", PrincipalID: "user-a", Realm: string(auth.RealmTenant), SessionID: "session-a", AuthVersion: 1, AgentRunID: &runID, ConversationID: &conversationID, ExpiresAt: time.Now().UTC().Add(time.Minute)}
	scopeDB, _ := tenantdata.MigrationDB(db)
	if err = scopeDB.Create(&scope).Error; err != nil {
		t.Fatal(err)
	}
	ctx := auth.WithContext(context.Background(), &auth.Context{Authenticated: true, TenantID: "tenant-a", PrincipalID: "user-a", SessionID: "session-a", AuthVersion: 1, Realm: auth.RealmTenant})
	service := ai.NewAgentRuntimeService(db, nil, nil, nil, nil, nil, nil, nil)
	confirmation := tooling.ToolConfirmation{ScopeID: scope.ID}
	if err = service.OnConfirmationApprove(ctx, &confirmation); err != nil {
		t.Fatalf("unchanged conversation rejected: %v", err)
	}
	mutationDB, _ := tenantdata.MigrationDB(db)
	if err = mutationDB.Model(&ai.Conversation{}).Where("id=?", conversation.ID).Update("revision", conversation.Revision+1).Error; err != nil {
		t.Fatal(err)
	}
	if err = service.OnConfirmationApprove(ctx, &confirmation); !isAppErrorCode(err, errs.CodeRevisionConflict) {
		t.Fatalf("revision change error=%v, want revision conflict", err)
	}
	mutationDB, _ = tenantdata.MigrationDB(db)
	if err = mutationDB.Model(&ai.Conversation{}).Where("id=?", conversation.ID).Updates(map[string]any{"revision": conversation.Revision, "status": "archived"}).Error; err != nil {
		t.Fatal(err)
	}
	if err = service.OnConfirmationApprove(ctx, &confirmation); !isAppErrorCode(err, errs.CodeRevisionConflict) {
		t.Fatalf("archived conversation error=%v, want revision conflict", err)
	}
}

func TestAgentRunCancelPersistsTerminalCategoryPostgres(t *testing.T) {
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	admin, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "ainp_agent_cancel_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err = admin.Exec(fmt.Sprintf(`CREATE SCHEMA %q`, schema)).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Exec(fmt.Sprintf(`DROP SCHEMA %q CASCADE`, schema)).Error })
	db, err := database.Open(dsn + " search_path=" + schema)
	if err != nil {
		t.Fatal(err)
	}
	if err = ai.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	run := ai.AgentRun{ID: "run-cancel", TenantID: "tenant-a", UserID: "user-a", ConversationID: "conversation-a", ConversationRevision: 1, TriggerMessageID: "message-a", AgentVersionID: "version-a", SessionID: "session-a", AuthVersion: 1, Status: "running", BudgetJSON: json.RawMessage(`{}`), IdempotencyKey: "run-cancel", LeaseOwner: "worker-a", LeaseExpiresAt: ptrTime(now.Add(time.Minute)), Revision: 3, StartedAt: &now}
	steps := []ai.AgentStep{
		{ID: "step-model", TenantID: run.TenantID, RunID: run.ID, StepIndex: 1, Type: "model", Status: "running", InputSummary: json.RawMessage(`{}`), OutputSummary: json.RawMessage(`{}`), CreatedAt: now, StartedAt: &now},
		{ID: "step-tool", TenantID: run.TenantID, RunID: run.ID, StepIndex: 2, Type: "tool_execute", Status: "waiting", InputSummary: json.RawMessage(`{}`), OutputSummary: json.RawMessage(`{}`), CreatedAt: now, StartedAt: &now},
	}
	fixtureDB, err := tenantdata.MigrationDB(db)
	if err != nil {
		t.Fatal(err)
	}
	if err = fixtureDB.Create(&run).Error; err != nil {
		t.Fatal(err)
	}
	for index := range steps {
		step := steps[index]
		if err = fixtureDB.Exec(`INSERT INTO ai_agent_steps
(id,tenant_id,run_id,step_index,type,status,input_summary,output_summary,created_at,started_at)
VALUES (?,?,?,?,?,?,?,?,?,?)`, step.ID, step.TenantID, step.RunID, step.StepIndex, step.Type, step.Status, step.InputSummary, step.OutputSummary, step.CreatedAt, step.StartedAt).Error; err != nil {
			t.Fatal(err)
		}
	}

	ctx := auth.WithContext(context.Background(), &auth.Context{Authenticated: true, TenantID: run.TenantID, PrincipalID: run.UserID, SessionID: run.SessionID, AuthVersion: run.AuthVersion, Realm: auth.RealmTenant})
	service := ai.NewAgentRuntimeService(db, nil, nil, nil, nil, nil, nil, nil)
	if _, err = service.Cancel(ctx, ai.AgentRunIDRequest{ID: run.ID, Revision: run.Revision}); err != nil {
		t.Fatal(err)
	}
	tdb, err := tenantdata.TenantDB(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	runs, err := tenantdata.TenantRaw[ai.AgentRun](tdb, "SELECT * FROM ai_agent_runs WHERE tenant_id=? AND id=? LIMIT 1", run.ID)
	if err != nil || len(runs) != 1 {
		t.Fatalf("run query rows=%d err=%v", len(runs), err)
	}
	if runs[0].Status != "cancelled" || runs[0].ErrorCategory != string(aimodelruntime.ErrorCancelled) || runs[0].FinishedAt == nil ||
		runs[0].TerminalProtocolVersion != aiagentruntime.TerminalProtocolVersion || runs[0].TerminalReason != "user_cancelled" ||
		runs[0].FinalizationMode != string(aiagentruntime.FinalizationNoticeOnly) || runs[0].FinalizationAttempted || runs[0].Retryable ||
		runs[0].UserMessageKey != "ai.run.terminal.cancelled" {
		t.Fatalf("cancelled run mismatch: %#v", runs[0])
	}
	persistedSteps, err := tenantdata.TenantRaw[ai.AgentStep](tdb, "SELECT * FROM ai_agent_steps WHERE tenant_id=? AND run_id=? ORDER BY step_index", run.ID)
	if err != nil || len(persistedSteps) != 2 {
		t.Fatalf("step query rows=%d err=%v", len(persistedSteps), err)
	}
	for _, step := range persistedSteps {
		if step.Status != "cancelled" || step.ErrorCategory != string(aimodelruntime.ErrorCancelled) || step.FinishedAt == nil {
			t.Fatalf("cancelled step mismatch: %#v", step)
		}
	}
	events, err := tenantdata.TenantRaw[ai.AgentRunEvent](tdb, "SELECT * FROM ai_agent_run_events WHERE tenant_id=? AND run_id=? ORDER BY sequence", run.ID)
	if err != nil || len(events) != 3 {
		t.Fatalf("event query rows=%d err=%v", len(events), err)
	}
	wantTypes := []string{"step.cancelled", "step.cancelled", "run.cancelled"}
	for index, event := range events {
		var payload map[string]any
		if json.Unmarshal(event.Payload, &payload) != nil || payload["error_category"] != string(aimodelruntime.ErrorCancelled) || event.Type != wantTypes[index] {
			t.Fatalf("cancel event mismatch: %#v payload=%#v", event, payload)
		}
		if event.Type == "run.cancelled" && (payload["terminal_reason"] != "user_cancelled" || payload["finalization_mode"] != string(aiagentruntime.FinalizationNoticeOnly) || payload["user_message_key"] != "ai.run.terminal.cancelled") {
			t.Fatalf("cancel terminal payload mismatch: %#v", payload)
		}
	}
}

func ptrTime(value time.Time) *time.Time { return &value }

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

func TestAgentActionAuditContextMigrationPostgres(t *testing.T) {
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	admin, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "ainp_agent_audit_migration_" + uuid.NewString()[:8]
	if err = admin.Exec(fmt.Sprintf(`CREATE SCHEMA %q`, schema)).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Exec(fmt.Sprintf(`DROP SCHEMA %q CASCADE`, schema)).Error })
	db, err := database.Open(dsn + " search_path=" + schema)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(`CREATE TABLE ai_agent_runs (id varchar(64) PRIMARY KEY, tenant_id varchar(64) NOT NULL)`).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(`CREATE TABLE audit_logs (id varchar(64) PRIMARY KEY, tenant_id varchar(64) NOT NULL)`).Error; err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile("../migrations/0042_agent_action_audit_context.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(string(body)).Error; err != nil {
		t.Fatal(err)
	}
	// The migration must remain idempotent for startup retries.
	if err = db.Exec(string(body)).Error; err != nil {
		t.Fatal(err)
	}
	var count int64
	if err = db.Raw(`SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=current_schema() AND ((table_name='ai_agent_runs' AND column_name IN ('origin_request_id','trace_id')) OR (table_name='audit_logs' AND column_name IN ('trace_id','agent_run_id','tool_invocation_id')))`).Scan(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 5 {
		t.Fatalf("agent audit context columns=%d, want 5", count)
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

func TestAgentTurnBudgetPlanner(t *testing.T) {
	tests := []struct {
		name      string
		input     aiagentruntime.TurnBudgetInput
		mode      aiagentruntime.ExecutionMode
		allowed   bool
		maxOutput int64
		reason    string
	}{
		{
			name:  "normal turn keeps completion reserve",
			input: aiagentruntime.TurnBudgetInput{HardLimit: 100000, ConsumedInput: 20000, ConsumedOutput: 1000, EstimatedInput: 10000, DeploymentMaxOutput: 4096, Reserve: aiagentruntime.CompletionReserve{InputTokens: 10000, OutputTokens: 2048, SafetyTokens: 512}},
			mode:  aiagentruntime.ExecutionModeNormal, allowed: true, maxOutput: 4096, reason: "within_soft_budget",
		},
		{
			name:  "soft pressure switches to completion",
			input: aiagentruntime.TurnBudgetInput{HardLimit: 100000, ConsumedInput: 79000, ConsumedOutput: 1000, EstimatedInput: 9000, DeploymentMaxOutput: 4096, Reserve: aiagentruntime.CompletionReserve{InputTokens: 9000, OutputTokens: 2048, SafetyTokens: 512}},
			mode:  aiagentruntime.ExecutionModeCompletionOnly, allowed: true, maxOutput: 4096, reason: "run_budget_pressure",
		},
		{
			name:  "completion mode is sticky",
			input: aiagentruntime.TurnBudgetInput{HardLimit: 100000, ConsumedInput: 1000, EstimatedInput: 1000, DeploymentMaxOutput: 2048, Reserve: aiagentruntime.CompletionReserve{InputTokens: 1000, OutputTokens: 2048, SafetyTokens: 128}, CurrentMode: aiagentruntime.ExecutionModeCompletionOnly},
			mode:  aiagentruntime.ExecutionModeCompletionOnly, allowed: true, maxOutput: 2048,
		},
		{
			name:  "completion is attempted at most once",
			input: aiagentruntime.TurnBudgetInput{HardLimit: 100000, ConsumedInput: 1000, EstimatedInput: 1000, DeploymentMaxOutput: 2048, Reserve: aiagentruntime.CompletionReserve{SafetyTokens: 128}, CurrentMode: aiagentruntime.ExecutionModeCompletionOnly, CompletionAttempted: true},
			mode:  aiagentruntime.ExecutionModeCompletionOnly, allowed: false, reason: "completion_already_attempted",
		},
		{
			name:  "minimum completion cannot fit",
			input: aiagentruntime.TurnBudgetInput{HardLimit: 1000, ConsumedInput: 800, EstimatedInput: 100, DeploymentMaxOutput: 2048, Reserve: aiagentruntime.CompletionReserve{SafetyTokens: 80}},
			mode:  aiagentruntime.ExecutionModeCompletionOnly, allowed: false, reason: "completion_cannot_fit",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := aiagentruntime.PlanTurn(tt.input)
			if err != nil {
				t.Fatal(err)
			}
			if got.Mode != tt.mode || got.Allowed != tt.allowed || got.MaxOutputTokens != tt.maxOutput || got.Reason != tt.reason {
				t.Fatalf("plan=%+v", got)
			}
		})
	}
	reserve := aiagentruntime.DefaultCompletionReserve(100000, 1024, 9000)
	if reserve.InputTokens != 9000 || reserve.OutputTokens != 1024 || reserve.SafetyTokens != 512 {
		t.Fatalf("reserve=%+v", reserve)
	}
}

func TestAgentTurnResultPlannerPreservesDeliverableOutput(t *testing.T) {
	tests := []struct {
		name string
		in   aiagentruntime.TurnResultInput
		want aiagentruntime.TurnResultPlan
	}{
		{name: "usable final text survives actual usage overrun", in: aiagentruntime.TurnResultInput{HasUsableText: true, HardExceeded: true}, want: aiagentruntime.TurnResultPlan{CommitFinal: true, CompletionReason: "budget_limit"}},
		{name: "completion text ignores provider tool violation", in: aiagentruntime.TurnResultInput{HasUsableText: true, ToolCallCount: 1, CompletionOnly: true, BudgetCompletion: true}, want: aiagentruntime.TurnResultPlan{CommitFinal: true, IgnoreToolCalls: true, CompletionReason: "budget_limit"}},
		{name: "completion tool without text fails closed", in: aiagentruntime.TurnResultInput{ToolCallCount: 1, CompletionOnly: true, BudgetCompletion: true}, want: aiagentruntime.TurnResultPlan{IgnoreToolCalls: true, ErrorCategory: "budget_completion_invalid_output"}},
		{name: "normal tool turn continues", in: aiagentruntime.TurnResultInput{ToolCallCount: 1}, want: aiagentruntime.TurnResultPlan{ExecuteTools: true}},
		{name: "hard overrun never executes tools", in: aiagentruntime.TurnResultInput{ToolCallCount: 1, HardExceeded: true}, want: aiagentruntime.TurnResultPlan{IgnoreToolCalls: true, ErrorCategory: "budget_hard_limit_exceeded"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := aiagentruntime.PlanTurnResult(tt.in); got != tt.want {
				t.Fatalf("result plan=%+v, want %+v", got, tt.want)
			}
		})
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

func TestAgentRunBudgetCompletionMigrationAndRuntimeContract(t *testing.T) {
	body, err := os.ReadFile("../migrations/0056_agent_run_budget_completion.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(body)
	for _, expected := range []string{"execution_mode", "completion_reason", "completion_attempted", "completion_reserved_input_tokens", "completion_reserved_output_tokens", "completion_safety_margin_tokens", "completion_only"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("budget completion migration missing %q", expected)
		}
	}
	runtimeBody, err := os.ReadFile("../modules/ai/conversation/runtime_service.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(runtimeBody)
	for _, expected := range []string{"completionOnlyRequest", `ToolChoice{Mode: "none"}`, "syncModelUsage", "budget_limit", "run_budget_pressure", "commitFinalAnswer"} {
		if !strings.Contains(source, expected) {
			t.Fatalf("budget completion runtime contract missing %q", expected)
		}
	}
	if strings.Count(source, "s.enterCompletionOnly(ctx, run, reserve, plan.Reason)") != 1 {
		t.Fatal("completion-only mode must be persisted only before the completion model request")
	}
	if !strings.Contains(source, "toolResultsPending := resumedConfirmation || queuedToolResultsPending") {
		t.Fatal("confirmation tool results must remain pending for model consumption")
	}
	if !strings.Contains(source, "return toolResultsPending, toolResultsPending") {
		t.Fatal("pending tool results at the step limit must get a completion turn")
	}
	promptBody, err := os.ReadFile("../prompts/ai/budget-completion.md")
	if err != nil {
		t.Fatal(err)
	}
	promptText := string(promptBody)
	for _, expected := range []string{"disabled only for this final turn", "Do not claim or imply", "reached its budget limit"} {
		if !strings.Contains(promptText, expected) {
			t.Fatalf("budget completion prompt missing %q", expected)
		}
	}
	anthropicBody, err := os.ReadFile("../framework/aimodelruntime/anthropic.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(anthropicBody), `r.ToolChoice.Mode != "none"`) {
		t.Fatal("anthropic completion-only request must omit unsupported none tool choice")
	}
}

func TestAgentRunBudgetCompletionMigrationPostgres(t *testing.T) {
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	admin, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "ainp_budget_completion_" + uuid.NewString()[:8]
	if err = admin.Exec(fmt.Sprintf(`CREATE SCHEMA %q`, schema)).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Exec(fmt.Sprintf(`DROP SCHEMA %q CASCADE`, schema)).Error })
	db, err := database.Open(dsn + " search_path=" + schema)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(`CREATE TABLE ai_agent_runs (id varchar(64) PRIMARY KEY)`).Error; err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile("../migrations/0056_agent_run_budget_completion.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(string(body)).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(string(body)).Error; err != nil {
		t.Fatalf("budget completion migration must be idempotent: %v", err)
	}
	var count int64
	if err = db.Raw(`SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='ai_agent_runs' AND column_name IN ('execution_mode','completion_reason','completion_attempted','completion_reserved_input_tokens','completion_reserved_output_tokens','completion_safety_margin_tokens')`).Scan(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 6 {
		t.Fatalf("budget completion columns=%d, want 6", count)
	}
}

func TestConversationToolWorkingSetMigrationAndRuntimeContract(t *testing.T) {
	body, err := os.ReadFile("../migrations/0052_conversation_tool_working_set.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(body)
	for _, expected := range []string{"ai_conversation_tool_working_set", "conversation_id", "schema_hash", "last_used_at DESC NULLS LAST", "use_count DESC", "activated_at DESC", "tool_id ASC", "ON DELETE CASCADE", "ON DELETE SET NULL"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("working set migration missing %q", expected)
		}
	}
	runtimeBody, err := os.ReadFile("../modules/ai/conversation/runtime_service.go")
	if err != nil {
		t.Fatal(err)
	}
	runtimeSource := string(runtimeBody)
	for _, expected := range []string{"conversationToolRestoreLimit     = 4", "toolRehydrateLimitPerRun         = 3", "WorkingSetCandidates", "WorkingSetEntry", `Status: "rehydrated"`, "SchemaHash != activation.Definition.SchemaHash"} {
		if !strings.Contains(runtimeSource, expected) {
			t.Fatalf("agent runtime working set contract missing %q", expected)
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
	body, err := os.ReadFile("../modules/ai/conversation/runtime_register.go")
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

func TestAgentActionAuditContextIsCanonicalAndPersisted(t *testing.T) {
	migration, err := os.ReadFile("../migrations/0042_agent_action_audit_context.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"origin_request_id", "trace_id", "agent_run_id", "tool_invocation_id"} {
		if !strings.Contains(string(migration), expected) {
			t.Fatalf("agent audit migration missing %q", expected)
		}
	}
	runtimeSource, err := os.ReadFile("../modules/ai/conversation/runtime_service.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"OriginRequestID: ac.RequestID", "TraceID: traceID", "traceID = ac.RequestID", `Client: "agent"`, "AgentRunID: run.ID"} {
		if !strings.Contains(string(runtimeSource), expected) {
			t.Fatalf("agent execution context missing %q", expected)
		}
	}
	toolSource, err := os.ReadFile("../modules/ai/tooling/tool_service.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(toolSource), "invocationContext.ToolInvocationID = inv.ID") {
		t.Fatal("tool invocation ID is not injected into Action execution context")
	}
}

func TestAgentRuntimeUsesSlotMetadataWithoutBusinessToolBranches(t *testing.T) {
	body, err := os.ReadFile("../modules/ai/conversation/runtime_service.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(body)
	for _, expected := range []string{"contract.RequiredToolSelectors", "Registry().IsBindable(b.ToolID)", "SlotMetadata(toolID)", "metadata.Observations"} {
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
	body, err := os.ReadFile("../modules/ai/knowledge/tool.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(body)
	for _, expected := range []string{`FormatUntrustedReferences`, `cacheKey`, `CanReadKnowledgeBase`} {
		if !strings.Contains(src, expected) {
			t.Fatalf("knowledge tool missing %q", expected)
		}
	}
	slot, err := os.ReadFile("../modules/ai/knowledge/slot.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`"builtin:knowledge.search"`, `DiscoveryDirect`, `MaxCallsPerRun: 4`, `Observations`} {
		if !strings.Contains(string(slot), expected) {
			t.Fatalf("knowledge slot missing %q", expected)
		}
	}
	toolService, err := os.ReadFile("../modules/ai/tooling/tool_service.go")
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
	slot, err := os.ReadFile("../modules/ai/memory/slot.go")
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
	if strings.Count(string(register), "memorypkg.New(") != 1 {
		t.Fatal("memory search slot must be registered exactly once")
	}
	toolService, err := os.ReadFile("../modules/ai/tooling/tool_service.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(toolService), `"builtin:memory.search"`) {
		t.Fatal("ToolService must not branch on memory tool ID")
	}
}
