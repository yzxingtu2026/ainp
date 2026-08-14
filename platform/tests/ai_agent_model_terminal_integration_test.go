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
	"github.com/yzxingtu2026/ainp/platform/framework/aiagentruntime"
	"github.com/yzxingtu2026/ainp/platform/framework/aiconversation"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodelruntime"
	"github.com/yzxingtu2026/ainp/platform/framework/aitool"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	"github.com/yzxingtu2026/ainp/platform/framework/prompt"
	"github.com/yzxingtu2026/ainp/platform/framework/runtimecontext"
	tenantdata "github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"github.com/yzxingtu2026/ainp/platform/modules/ai"
)

func TestAgentModelTimeoutPersistsCanonicalTerminalFactsPostgres(t *testing.T) {
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	admin, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "ainp_agent_model_terminal_" + strings.ReplaceAll(uuid.NewString(), "-", "")
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
	if err = prompt.LoadDir("../prompts"); err != nil {
		t.Fatal(err)
	}

	fixtureDB, err := tenantdata.MigrationDB(db)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	userID := "user-a"
	agent := ai.Agent{ID: "agent-a", TenantID: "tenant-a", OwnerUserID: &userID, Code: "fixture", Name: "Fixture", Source: "tenant", Visibility: "private", Status: "enabled", CurrentVersion: 1, DraftGenerationConfig: json.RawMessage(`{}`), DraftToolPolicy: json.RawMessage(`{}`), Revision: 1}
	version := ai.AgentVersion{ID: "version-a", TenantID: agent.TenantID, AgentID: agent.ID, Version: 1, Kind: "release", ModelAlias: "fixture-chat", GenerationConfig: json.RawMessage(`{}`), ToolPolicy: json.RawMessage(`{}`), MemoryPolicy: json.RawMessage(`{}`), CreatedBy: userID}
	conversation := ai.Conversation{ID: "conversation-a", TenantID: agent.TenantID, OwnerUserID: userID, Title: "Fixture", AgentID: agent.ID, AgentVersionID: version.ID, Purpose: ai.ConversationPurposeWorkspace, Visibility: "private", Status: "active", Revision: 1, CreatedAt: now}
	content, _ := json.Marshal([]aimodelruntime.ContentPart{{Type: aimodelruntime.PartInputText, Text: "hello"}})
	message := ai.ConversationMessage{ID: "message-a", TenantID: agent.TenantID, ConversationID: conversation.ID, Role: "user", Status: "committed", Sequence: 1, AuthorUserID: &userID, Content: content, IdempotencyKey: "message-a", CreatedAt: now, CommittedAt: &now}
	budget, _ := json.Marshal(aiagentruntime.SystemDefaultBudget())
	run := ai.AgentRun{ID: "run-a", TenantID: agent.TenantID, UserID: userID, ConversationID: conversation.ID, ConversationRevision: conversation.Revision, TriggerMessageID: message.ID, AgentVersionID: version.ID, SessionID: "session-a", AuthVersion: 1, OriginRequestID: "request-a", TraceID: "trace-a", Status: "queued", ExecutionMode: "normal", BudgetJSON: budget, IdempotencyKey: "run-a", Revision: 1, CreatedAt: now}
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO ai_agents (id,tenant_id,owner_user_id,code,name,source,visibility,status,current_version,has_draft,draft_generation_config,draft_tool_policy,revision,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, []any{agent.ID, agent.TenantID, userID, agent.Code, agent.Name, agent.Source, agent.Visibility, agent.Status, agent.CurrentVersion, false, `{}`, `{}`, agent.Revision, now, now}},
		{`INSERT INTO ai_agent_versions (id,tenant_id,agent_id,version,kind,content_hash,model_alias,generation_config,tool_policy,memory_policy,memory_policy_source,created_by,created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`, []any{version.ID, version.TenantID, version.AgentID, version.Version, version.Kind, "fixture-hash", version.ModelAlias, `{}`, `{}`, `{}`, "system_default", userID, now}},
		{`INSERT INTO ai_conversations (id,tenant_id,owner_user_id,title,agent_id,agent_version_id,purpose,visibility,status,revision,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`, []any{conversation.ID, conversation.TenantID, conversation.OwnerUserID, conversation.Title, conversation.AgentID, conversation.AgentVersionID, conversation.Purpose, conversation.Visibility, conversation.Status, conversation.Revision, now, now}},
		{`INSERT INTO ai_messages (id,tenant_id,conversation_id,role,status,sequence,author_user_id,content,idempotency_key,created_at,committed_at) VALUES (?,?,?,?,?,?,?,?,?,?,?)`, []any{message.ID, message.TenantID, message.ConversationID, message.Role, message.Status, message.Sequence, userID, string(content), message.IdempotencyKey, now, now}},
		{`INSERT INTO ai_agent_runs (id,tenant_id,user_id,conversation_id,conversation_revision,trigger_message_id,agent_version_id,session_id,auth_version,origin_request_id,trace_id,status,execution_mode,budget_json,idempotency_key,revision,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, []any{run.ID, run.TenantID, run.UserID, run.ConversationID, run.ConversationRevision, run.TriggerMessageID, run.AgentVersionID, run.SessionID, run.AuthVersion, run.OriginRequestID, run.TraceID, run.Status, run.ExecutionMode, string(budget), run.IdempotencyKey, run.Revision, now, now}},
	}
	for _, statement := range statements {
		if err = fixtureDB.Exec(statement.query, statement.args...).Error; err != nil {
			t.Fatal(err)
		}
	}

	source := &runtimeStepStream{steps: []runtimeStreamStep{{waitContext: true}}}
	adapters := aimodelruntime.NewAdapterRegistry()
	if err = adapters.Register("fixture", streamFixtureAdapter{source: source}); err != nil {
		t.Fatal(err)
	}
	signer, err := aitool.NewHandleSigner("issue-271-fixture")
	if err != nil {
		t.Fatal(err)
	}
	tools := ai.NewToolService(db, aitool.NewRegistry(), signer, nil)
	invocations := ai.NewInvocationService(db, streamFixtureResolver{}, adapters, nil)
	invocations.SetToolProjector(tools)
	conversations := ai.NewConversationService(db, nil)
	authorization := &fakeAuthorizationProvider{snapshot: &auth.AuthorizationSnapshot{TenantID: run.TenantID, PrincipalID: run.UserID, SessionID: run.SessionID, AuthVersion: run.AuthVersion}}
	runtime := ai.NewAgentRuntimeService(db, conversations, invocations, tools, agentTerminalCompactor{}, nil, authorization, nil, agentTerminalContextProvider{now: now})

	baseCtx := auth.WithContext(context.Background(), &auth.Context{Authenticated: true, TenantID: run.TenantID, PrincipalID: run.UserID, SessionID: run.SessionID, AuthVersion: run.AuthVersion, Realm: auth.RealmTenant})
	executeCtx, cancel := context.WithTimeout(baseCtx, time.Second)
	defer cancel()
	if err = runtime.Execute(executeCtx, run.ID, "worker-a"); err != nil {
		t.Fatal(err)
	}
	if !source.closed.Load() {
		t.Fatal("provider stream was not closed")
	}

	tdb, err := tenantdata.TenantDB(baseCtx, db)
	if err != nil {
		t.Fatal(err)
	}
	invocationRows, err := tenantdata.TenantRaw[ai.ModelInvocation](tdb, "SELECT * FROM ai_model_invocations WHERE tenant_id=? AND agent_run_id=? LIMIT 1", run.ID)
	if err != nil || len(invocationRows) != 1 {
		t.Fatalf("invocation rows=%d err=%v", len(invocationRows), err)
	}
	invocation := invocationRows[0]
	if invocation.Status != "failed" || invocation.ErrorCategory != string(aimodelruntime.ErrorTimeout) || invocation.FinishReason != string(aimodelruntime.FinishCancelled) {
		t.Fatalf("invocation terminal mismatch: %#v", invocation)
	}
	runRows, err := tenantdata.TenantRaw[ai.AgentRun](tdb, "SELECT * FROM ai_agent_runs WHERE tenant_id=? AND id=? LIMIT 1", run.ID)
	if err != nil || len(runRows) != 1 || runRows[0].Status != "failed" || runRows[0].ErrorCategory != string(aimodelruntime.ErrorTimeout) {
		t.Fatalf("run terminal mismatch: rows=%#v err=%v", runRows, err)
	}
	stepRows, err := tenantdata.TenantRaw[ai.AgentStep](tdb, "SELECT * FROM ai_agent_steps WHERE tenant_id=? AND run_id=? AND type='model' LIMIT 1", run.ID)
	if err != nil || len(stepRows) != 1 {
		t.Fatalf("model step rows=%d err=%v", len(stepRows), err)
	}
	step := stepRows[0]
	if step.Status != "failed" || step.ErrorCategory != string(aimodelruntime.ErrorTimeout) || step.ModelInvocationID == nil || *step.ModelInvocationID != invocation.ID {
		t.Fatalf("model step terminal mismatch: %#v", step)
	}
	events, err := tenantdata.TenantRaw[ai.AgentRunEvent](tdb, "SELECT * FROM ai_agent_run_events WHERE tenant_id=? AND run_id=? AND type IN ('step.failed','run.failed') ORDER BY sequence", run.ID)
	if err != nil || len(events) != 2 {
		t.Fatalf("terminal events=%d err=%v", len(events), err)
	}
	for _, event := range events {
		var payload map[string]any
		if json.Unmarshal(event.Payload, &payload) != nil || payload["error_category"] != string(aimodelruntime.ErrorTimeout) {
			t.Fatalf("terminal event mismatch: %#v payload=%#v", event, payload)
		}
	}
}

type agentTerminalCompactor struct{}

func (agentTerminalCompactor) Compact(context.Context, aiconversation.CompactionRequest) (*aiconversation.CompactionResult, error) {
	return &aiconversation.CompactionResult{Status: "unchanged"}, nil
}

func (agentTerminalCompactor) ContextTokenLimit(context.Context, string) (int, error) {
	return 7000, nil
}

func (agentTerminalCompactor) ModelTurnBudget(context.Context, string) (ai.ModelContextBudget, error) {
	return ai.ModelContextBudget{ContextWindow: 8192, ReservedOutputTokens: 512, SafetyMarginTokens: 680, AvailableTokens: 7000}, nil
}

func (agentTerminalCompactor) EstimateRequestTokens(context.Context, aimodelruntime.GenerateRequest) (int, error) {
	return 128, nil
}

type agentTerminalContextProvider struct{ now time.Time }

func (p agentTerminalContextProvider) Build(_ context.Context, request runtimecontext.BuildRequest) (*runtimecontext.TrustedRuntimeContext, error) {
	snapshot, err := runtimecontext.NewTimeSnapshot(p.now, "Asia/Shanghai", "fixture")
	if err != nil {
		return nil, err
	}
	return &runtimecontext.TrustedRuntimeContext{
		TenantID: request.TenantID, PrincipalID: request.PrincipalID, ConversationID: request.ConversationID, ConversationCreatedAt: request.ConversationCreatedAt,
		AgentID: request.AgentID, AgentVersionID: request.AgentVersionID, RunID: request.RunID, Time: snapshot,
		ContextVersion: runtimecontext.ContextVersion, PolicyVersion: runtimecontext.PolicyVersion,
	}, nil
}
