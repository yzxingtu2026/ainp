package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/aimemory"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodelruntime"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/database"
	"github.com/yzxingtu2026/ainp/platform/framework/diagnostic"
	"github.com/yzxingtu2026/ainp/platform/framework/job"
	"github.com/yzxingtu2026/ainp/platform/framework/outbox"
	"github.com/yzxingtu2026/ainp/platform/framework/prompt"
	"github.com/yzxingtu2026/ainp/platform/modules/ai"
	"github.com/yzxingtu2026/ainp/platform/modules/system"
	"os"
	"strings"
	"testing"
)

type fakeMemoryExtractor struct {
	outputs []string
	calls   int
}

func (f *fakeMemoryExtractor) Generate(_ context.Context, _ aimodelruntime.GenerateRequest) (any, error) {
	text := f.outputs[f.calls]
	f.calls++
	return aimodelruntime.GenerateResponse{Output: []aimodelruntime.OutputItem{{Type: aimodelruntime.PartOutputText, Text: text}}, FinishReason: aimodelruntime.FinishCompleted}, nil
}

func TestMemoryExtractionMigrationAndReliableChainContract(t *testing.T) {
	body, err := os.ReadFile("../migrations/0031_memory_candidate_extraction.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(body)
	for _, expected := range []string{"ai_memory_evaluations", "UNIQUE (tenant_id,run_id)", "arguments_hash", "explicit_user_intent", "fk_ai_memory_items_evaluation_tenant", "FOREIGN KEY (tenant_id,run_id)"} {
		if !strings.Contains(sql, expected) {
			t.Fatalf("migration missing %q", expected)
		}
	}
	register, err := os.ReadFile("../modules/ai/memory/extraction_register.go")
	if err != nil {
		t.Fatal(err)
	}
	code := string(register)
	for _, expected := range []string{"aiagentruntime.EventRunTerminal", "MemoryExtractionJobType", "MaxAttempts: 5", "job.Permanent", "ActionMemoryExtractionRecover", "memory-recovery:"} {
		if !strings.Contains(code, expected) {
			t.Fatalf("reliable chain missing %q", expected)
		}
	}
	extraction, err := os.ReadFile("../modules/ai/memory/extraction.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"MemoryExtractionPromptVersion = \"2\"", "MemoryExtractionSchemaVersion = \"2\"", "IANA timezone", "invalid_candidate_timezone", "diagnostic.UpstreamInvalidResponse"} {
		if !strings.Contains(string(extraction), expected) {
			t.Fatalf("timezone contract missing %q", expected)
		}
	}
	promptBody, err := os.ReadFile("../prompts/ai/memory-extraction.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"version: 1.1.0", "IANA timezone", "输出空字符串", "UTC+8", "occurred_at 为空"} {
		if !strings.Contains(string(promptBody), expected) {
			t.Fatalf("memory extraction prompt missing %q", expected)
		}
	}
}

func TestMemoryExtractionMigrationPostgres(t *testing.T) {
	dsn := os.Getenv("AINP_TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("AINP_TEST_DATABASE_DSN is not set")
	}
	if err := prompt.LoadDir("../prompts"); err != nil {
		t.Fatal(err)
	}
	admin, err := database.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "ainp_memory_extract_" + uuid.NewString()[:8]
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
	if err = db.Migrator().DropTable(&ai.MemoryConsolidationBatch{}, &ai.MemoryRelation{}, &ai.MemoryGovernanceDecision{}, &ai.MemoryEvaluation{}); err != nil {
		t.Fatal(err)
	}
	restorePre0054AccountSchema(t, db)
	for _, name := range []string{"0028_tenant_composite_constraints.sql", "0029_ai_memory_foundation.sql", "0030_agent_memory_policy.sql", "0031_memory_candidate_extraction.sql", "0032_memory_governance_projections.sql"} {
		applyTestMigration(t, db, name)
	}
	applyTestMigration(t, db, "0054_tenant_accounts.sql")
	var count int64
	if err = db.Raw(`SELECT COUNT(*) FROM pg_constraint WHERE conname IN ('fk_ai_memory_items_evaluation_tenant','fk_ai_memory_items_version_tenant')`).Scan(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("candidate constraints = %d, want 2", count)
	}
	policy := aimemory.SystemDefaultPolicy()
	policy.Enabled = true
	policy.AutoSaveLowSensitive = true
	policy.ExtractionModelAlias = "memory"
	policyJSON, _ := json.Marshal(policy)
	statements := []struct {
		sql  string
		args []any
	}{{`INSERT INTO tenants(id,name,status) VALUES('tenant-a','A','enabled')`, nil}, {`INSERT INTO tenant_accounts(id,tenant_id,username,password_hash,status,auth_version) VALUES('user-a','tenant-a','a','x','enabled',1)`, nil}, {`INSERT INTO ai_agents(id,tenant_id,owner_user_id,code,name,description,source,visibility,status,current_version,revision) VALUES('agent-a','tenant-a','user-a','a','A','','personal','private','enabled',1,1)`, nil}, {`INSERT INTO ai_agent_versions(id,tenant_id,agent_id,version,kind,content_hash,model_alias,tenant_instructions,generation_config,tool_policy,memory_policy,memory_policy_source,created_by) VALUES('version-a','tenant-a','agent-a',1,'release','h','chat','','{}','{}',?,'agent_version','user-a')`, []any{policyJSON}}, {`INSERT INTO ai_conversations(id,tenant_id,owner_user_id,title,agent_id,agent_version_id,purpose,visibility,status,revision) VALUES('conversation-a','tenant-a','user-a','C','agent-a','version-a','workspace','private','active',1)`, nil}, {`INSERT INTO ai_messages(id,tenant_id,conversation_id,role,status,sequence,content,idempotency_key,committed_at) VALUES('m1','tenant-a','conversation-a','user','committed',1,'[{"type":"input_text","text":"以后请称呼我为老板"}]','m1',now())`, nil}, {`INSERT INTO ai_agent_runs(id,tenant_id,user_id,conversation_id,conversation_revision,trigger_message_id,agent_version_id,auth_version,timezone,status,budget_json,idempotency_key,revision,finished_at) VALUES('run-a','tenant-a','user-a','conversation-a',1,'m1','version-a',1,'Asia/Shanghai','completed','{}','run-a',1,now())`, nil}, {`INSERT INTO ai_messages(id,tenant_id,conversation_id,role,status,sequence,run_id,content,idempotency_key,committed_at) VALUES('m2','tenant-a','conversation-a','assistant','committed',2,'run-a','[{"type":"output_text","text":"好的，老板"}]','m2',now())`, nil}}
	for _, statement := range statements {
		if err = db.Exec(statement.sql, statement.args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	fake := &fakeMemoryExtractor{outputs: []string{
		`{"candidates":[{"kind":"preference","summary":"称呼偏好","content":"用户希望被称呼为老板","occurred_at":"2026-07-18T16:30:00Z","occurred_end_at":null,"occurred_timezone":"","date_precision":"exact_time","valid_from":null,"valid_to":null,"confidence":0.98,"sensitivity":"normal","importance":90,"explicit_user_intent":true,"source_message_ids":["m1","m2"],"reason":"preference","attribute_key":"preferred_address","attribute_value":"老板"}]}`,
	}}
	ctx := auth.WithContext(context.Background(), &auth.Context{Authenticated: true, Realm: auth.RealmTenant, TenantID: "tenant-a", PrincipalID: "user-a"})
	service := ai.NewMemoryExtractionService(db, fake)
	if err = service.EvaluateRun(ctx, "run-a"); err != nil {
		t.Fatal(err)
	}
	if err = service.EvaluateRun(ctx, "run-a"); err != nil {
		t.Fatal(err)
	}
	if err = db.Raw(`SELECT COUNT(*) FROM ai_memory_items WHERE tenant_id='tenant-a' AND status='candidate'`).Scan(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("idempotent candidates = %d, want 1", count)
	}
	if fake.calls != 1 {
		t.Fatalf("model calls = %d, want 1", fake.calls)
	}
	var occurredTimezone string
	if err = db.Raw(`SELECT occurred_timezone FROM ai_memory_items WHERE tenant_id='tenant-a' AND evaluation_id=(SELECT id FROM ai_memory_evaluations WHERE run_id='run-a')`).Scan(&occurredTimezone).Error; err != nil {
		t.Fatal(err)
	}
	if occurredTimezone != "Asia/Shanghai" {
		t.Fatalf("resolved occurred timezone = %q, want Asia/Shanghai", occurredTimezone)
	}

	invalidZones := []string{"UTC+8", "CST", "北京时间", "Mars/Olympus_Mons"}
	for i, timezone := range invalidZones {
		runID := fmt.Sprintf("run-%c", 'b'+i)
		userMessageID := fmt.Sprintf("m%d", 3+i*2)
		assistantMessageID := fmt.Sprintf("m%d", 4+i*2)
		sequence := 3 + i*2
		if err = db.Exec(`INSERT INTO ai_messages(id,tenant_id,conversation_id,role,status,sequence,content,idempotency_key,committed_at) VALUES(?,'tenant-a','conversation-a','user','committed',?,'[{"type":"input_text","text":"记录一个事件"}]',?,now())`, userMessageID, sequence, userMessageID).Error; err != nil {
			t.Fatal(err)
		}
		if err = db.Exec(`INSERT INTO ai_agent_runs(id,tenant_id,user_id,conversation_id,conversation_revision,trigger_message_id,agent_version_id,auth_version,timezone,status,budget_json,idempotency_key,revision,finished_at) VALUES(?,'tenant-a','user-a','conversation-a',1,?,'version-a',1,'Asia/Shanghai','completed','{}',?,1,now())`, runID, userMessageID, runID).Error; err != nil {
			t.Fatal(err)
		}
		if err = db.Exec(`INSERT INTO ai_messages(id,tenant_id,conversation_id,role,status,sequence,run_id,content,idempotency_key,committed_at) VALUES(?,'tenant-a','conversation-a','assistant','committed',?,?, '[{"type":"output_text","text":"已记录"}]',?,now())`, assistantMessageID, sequence+1, runID, assistantMessageID).Error; err != nil {
			t.Fatal(err)
		}
		fake.outputs = append(fake.outputs, fmt.Sprintf(`{"candidates":[{"kind":"event","summary":"敏感候选正文不得泄露","content":"不可记录的候选内容","occurred_at":"2026-07-19T09:00:00+08:00","occurred_end_at":null,"occurred_timezone":%q,"date_precision":"exact_time","valid_from":null,"valid_to":null,"confidence":0.9,"sensitivity":"normal","importance":80,"explicit_user_intent":true,"source_message_ids":[%q,%q],"reason":"event","attribute_key":"","attribute_value":""}]}`, timezone, userMessageID, assistantMessageID))
		if err = service.EvaluateRun(ctx, runID); err == nil {
			t.Fatalf("expected invalid candidate timezone failure for %q", timezone)
		} else {
			meta := diagnostic.Describe(err, "fallback")
			if meta.Category != diagnostic.UpstreamInvalidResponse || meta.Stage != "ai.memory_extraction.candidate_validation" || meta.SafeSummary != "memory extraction timezone validation failed" {
				t.Fatalf("unexpected diagnostic metadata for %q: %#v", timezone, meta)
			}
			if strings.Contains(err.Error(), timezone) || strings.Contains(err.Error(), "敏感候选正文不得泄露") {
				t.Fatalf("candidate data leaked through error: %v", err)
			}
		}
		var failureCategory string
		if err = db.Raw(`SELECT error_category FROM ai_memory_evaluations WHERE tenant_id='tenant-a' AND run_id=?`, runID).Scan(&failureCategory).Error; err != nil {
			t.Fatal(err)
		}
		if failureCategory != "invalid_candidate_timezone" {
			t.Fatalf("failure category for %q = %q, want invalid_candidate_timezone", timezone, failureCategory)
		}
		if err = db.Raw(`SELECT COUNT(*) FROM ai_memory_items WHERE tenant_id='tenant-a' AND evaluation_id=(SELECT id FROM ai_memory_evaluations WHERE run_id=?)`, runID).Scan(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("invalid timezone %q created %d memory items, want 0", timezone, count)
		}
	}
	fake.outputs = append(fake.outputs, `{"candidates":[{"kind":"event","summary":"恢复后的合法候选","content":"恢复后的合法候选","occurred_at":"2026-07-19T09:00:00+08:00","occurred_end_at":null,"occurred_timezone":"Asia/Shanghai","date_precision":"exact_time","valid_from":null,"valid_to":null,"confidence":0.9,"sensitivity":"normal","importance":80,"explicit_user_intent":true,"source_message_ids":["m3","m4"],"reason":"event","attribute_key":"","attribute_value":""}]}`)
	if err = service.EvaluateRun(ctx, "run-b"); err != nil {
		t.Fatalf("recover failed evaluation: %v", err)
	}
	if err = service.EvaluateRun(ctx, "run-b"); err != nil {
		t.Fatalf("idempotent recovered evaluation: %v", err)
	}
	if err = db.Raw(`SELECT COUNT(*) FROM ai_memory_items WHERE tenant_id='tenant-a' AND evaluation_id=(SELECT id FROM ai_memory_evaluations WHERE run_id='run-b')`).Scan(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 || fake.calls != 6 {
		t.Fatalf("recovered candidates/model calls = %d/%d, want 1/6", count, fake.calls)
	}
	if err = db.Exec(`INSERT INTO ai_messages(id,tenant_id,conversation_id,role,status,sequence,content,idempotency_key,committed_at) VALUES('m11','tenant-a','conversation-a','user','committed',11,'[{"type":"input_text","text":"记录未知时间事件"}]','m11',now())`).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(`INSERT INTO ai_agent_runs(id,tenant_id,user_id,conversation_id,conversation_revision,trigger_message_id,agent_version_id,auth_version,timezone,status,budget_json,idempotency_key,revision,finished_at) VALUES('run-f','tenant-a','user-a','conversation-a',1,'m11','version-a',1,'Asia/Shanghai','completed','{}','run-f',1,now())`).Error; err != nil {
		t.Fatal(err)
	}
	if err = db.Exec(`INSERT INTO ai_messages(id,tenant_id,conversation_id,role,status,sequence,run_id,content,idempotency_key,committed_at) VALUES('m12','tenant-a','conversation-a','assistant','committed',12,'run-f','[{"type":"output_text","text":"已记录"}]','m12',now())`).Error; err != nil {
		t.Fatal(err)
	}
	fake.outputs = append(fake.outputs, `{"candidates":[{"kind":"event","summary":"未知时间事件","content":"未知时间事件","occurred_at":null,"occurred_end_at":null,"occurred_timezone":"UTC+8","date_precision":"unknown","valid_from":null,"valid_to":null,"confidence":0.9,"sensitivity":"normal","importance":80,"explicit_user_intent":true,"source_message_ids":["m11","m12"],"reason":"event","attribute_key":"","attribute_value":""}]}`)
	if err = service.EvaluateRun(ctx, "run-f"); err != nil {
		t.Fatalf("unknown occurred time should not validate candidate timezone: %v", err)
	}
	var unknownLocalDate *string
	if err = db.Raw(`SELECT local_date::text, occurred_timezone FROM ai_memory_items WHERE tenant_id='tenant-a' AND evaluation_id=(SELECT id FROM ai_memory_evaluations WHERE run_id='run-f')`).Row().Scan(&unknownLocalDate, &occurredTimezone); err != nil {
		t.Fatal(err)
	}
	if unknownLocalDate != nil || occurredTimezone != "" {
		t.Fatalf("unknown occurred time persisted local_date/timezone = %v/%q", unknownLocalDate, occurredTimezone)
	}
	governance := ai.NewMemoryGovernanceService(db, nil)
	if err = governance.GovernRun(ctx, "run-a"); err != nil {
		t.Fatal(err)
	}
	projection := ai.NewMemoryProjectionService(db)
	if err = projection.Rebuild(ctx, "user-a"); err != nil {
		t.Fatal(err)
	}
	var status string
	if err = db.Raw(`SELECT status FROM ai_memory_items WHERE tenant_id='tenant-a' AND evaluation_id=(SELECT id FROM ai_memory_evaluations WHERE run_id='run-a')`).Scan(&status).Error; err != nil {
		t.Fatal(err)
	}
	if status != "active" {
		t.Fatalf("governed status = %s", status)
	}
	var profileJSON string
	if err = db.Raw(`SELECT profile_json FROM ai_memory_current_profiles WHERE tenant_id='tenant-a' AND user_id='user-a'`).Scan(&profileJSON).Error; err != nil {
		t.Fatal(err)
	}
	var profile map[string]any
	if json.Unmarshal([]byte(profileJSON), &profile) != nil || profile["preferred_address"] != "老板" {
		t.Fatalf("unexpected profile: %s", profileJSON)
	}
	var localDate string
	if err = db.Raw(`SELECT local_date::text FROM ai_memory_day_summaries WHERE tenant_id='tenant-a' AND user_id='user-a'`).Scan(&localDate).Error; err != nil {
		t.Fatal(err)
	}
	if localDate != "2026-07-19" {
		t.Fatalf("day archive date = %s", localDate)
	}
}
