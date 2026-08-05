package memory

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/aiconversation"
	"github.com/yzxingtu2026/ainp/platform/framework/aimemory"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodelruntime"
	"github.com/yzxingtu2026/ainp/platform/framework/diagnostic"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/logging"
	"github.com/yzxingtu2026/ainp/platform/framework/prompt"
	tenantdata "github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"github.com/yzxingtu2026/ainp/platform/modules/ai/conversation"
	"gorm.io/gorm"
)

type AgentRun = conversation.AgentRun
type ConversationMessage = conversation.ConversationMessage
type agentVersionProjection struct {
	ID, AgentID  string
	MemoryPolicy json.RawMessage
}

const (
	MemoryExtractionJobType       = "ai.memory.evaluate_run"
	MemoryExtractionPromptVersion = "2"
	MemoryExtractionSchemaVersion = "2"

	memoryExtractionInvalidCandidateTimezone = "invalid_candidate_timezone"
	memoryExtractionInvalidRuntimeTimezone   = "invalid_runtime_timezone"
)

type memoryExtractionValidationError struct {
	category string
}

func (e memoryExtractionValidationError) Error() string { return e.category }

type MemoryCandidateExtractor interface {
	Generate(context.Context, aimodelruntime.GenerateRequest) (any, error)
}
type MemoryExtractionService struct {
	db          tenantdata.GuardedDatabase
	invocations MemoryCandidateExtractor
}

func NewMemoryExtractionService(db *gorm.DB, invocations MemoryCandidateExtractor) *MemoryExtractionService {
	return &MemoryExtractionService{db: tenantdata.GuardedDB(db), invocations: invocations}
}

type memoryExtractionOutput struct {
	Candidates []memoryCandidate `json:"candidates"`
}
type memoryCandidate struct {
	Kind               aimemory.ItemType      `json:"kind"`
	Summary            string                 `json:"summary"`
	Content            string                 `json:"content"`
	OccurredAt         *string                `json:"occurred_at"`
	OccurredEndAt      *string                `json:"occurred_end_at"`
	OccurredTimezone   string                 `json:"occurred_timezone"`
	DatePrecision      aimemory.DatePrecision `json:"date_precision"`
	ValidFrom          *string                `json:"valid_from"`
	ValidTo            *string                `json:"valid_to"`
	Confidence         float64                `json:"confidence"`
	Sensitivity        aimemory.Sensitivity   `json:"sensitivity"`
	Importance         int                    `json:"importance"`
	ExplicitUserIntent bool                   `json:"explicit_user_intent"`
	SourceMessageIDs   []string               `json:"source_message_ids"`
	Reason             string                 `json:"reason"`
	AttributeKey       string                 `json:"attribute_key"`
	AttributeValue     string                 `json:"attribute_value"`
}

func (s *MemoryExtractionService) EvaluateRun(ctx context.Context, runID string) error {
	started := time.Now()
	if strings.TrimSpace(runID) == "" {
		return jobPermanent(errs.New(errs.CodeValidation, "run id required"))
	}
	db := s.db.WithContext(ctx)
	var run AgentRun
	if err := db.Where("id=?", runID).First(&run).Error; err != nil {
		return err
	}
	if run.Status != "completed" {
		return jobPermanent(fmt.Errorf("run is not completed"))
	}
	var existing MemoryEvaluation
	lookup := db.Where("run_id=?", run.ID).Limit(1).Find(&existing)
	if lookup.Error != nil {
		return lookup.Error
	}
	if lookup.RowsAffected == 1 && existing.Status == "completed" {
		return nil
	}
	if existing.ID != "" && existing.Status == "running" && existing.UpdatedAt.After(time.Now().UTC().Add(-5*time.Minute)) {
		return nil
	}
	tdb, err := s.db.Tenant(ctx)
	if err != nil {
		return err
	}
	versions, err := tenantdata.TenantRaw[agentVersionProjection](tdb, `SELECT id,agent_id,memory_policy FROM ai_agent_versions WHERE tenant_id=? AND id=? LIMIT 1`, run.AgentVersionID)
	if err != nil || len(versions) != 1 {
		if err != nil {
			return err
		}
		return gorm.ErrRecordNotFound
	}
	version := versions[0]
	var policy aimemory.Policy
	if err := strictJSON(version.MemoryPolicy, &policy); err != nil || policy.Validate() != nil {
		return jobPermanent(fmt.Errorf("invalid frozen memory policy"))
	}
	if !policy.Enabled {
		return s.recordSkipped(ctx, run, "policy_disabled")
	}
	var trigger ConversationMessage
	if err := db.Where("id=? AND conversation_id=? AND status='committed' AND role='user'", run.TriggerMessageID, run.ConversationID).First(&trigger).Error; err != nil {
		return jobPermanent(fmt.Errorf("eligible trigger message missing"))
	}
	var replies []ConversationMessage
	if err := db.Where("conversation_id=? AND run_id=? AND status='committed' AND role='assistant'", run.ConversationID, run.ID).Order("sequence").Find(&replies).Error; err != nil {
		return err
	}
	userText, ok := eligibleText(trigger, aimodelruntime.PartInputText)
	if !ok || len(replies) == 0 {
		return s.recordSkipped(ctx, run, "ineligible_delta")
	}
	allowed := map[string]bool{trigger.ID: true}
	assistantTexts := []map[string]any{}
	sequenceTo := trigger.Sequence
	for _, reply := range replies {
		if text, eligible := eligibleText(reply, aimodelruntime.PartOutputText); eligible {
			assistantTexts = append(assistantTexts, map[string]any{"id": reply.ID, "sequence": reply.Sequence, "text": text})
			allowed[reply.ID] = true
			if reply.Sequence > sequenceTo {
				sequenceTo = reply.Sequence
			}
		}
	}
	if len(assistantTexts) == 0 {
		return s.recordSkipped(ctx, run, "ineligible_delta")
	}
	arguments := map[string]any{"run_id": run.ID, "conversation_id": run.ConversationID, "observed_at": run.FinishedAt, "user_message": map[string]any{"id": trigger.ID, "sequence": trigger.Sequence, "text": userText}, "assistant_messages": assistantTexts, "allowed_source_message_ids": mapKeys(allowed), "max_candidates": policy.MaxCandidatesPerTurn, "policy_version": policy.PolicyVersion, "prompt_version": policy.PromptVersion, "schema_version": policy.SchemaVersion}
	body, _ := json.Marshal(arguments)
	hash := sha256Hex(body)
	evaluation := existing
	if evaluation.ID == "" {
		evaluation = MemoryEvaluation{ID: uuid.NewString(), UserID: run.UserID, ConversationID: run.ConversationID, RunID: run.ID, AgentVersionID: run.AgentVersionID, SequenceFrom: trigger.Sequence, SequenceTo: sequenceTo, Status: "running", ArgumentsHash: hash, Attempts: 1}
		if err := db.Create(&evaluation).Error; err != nil {
			if findErr := db.Where("run_id=?", run.ID).First(&evaluation).Error; findErr != nil {
				return err
			}
			if evaluation.Status == "completed" {
				return nil
			}
		}
	} else {
		res := db.Model(&MemoryEvaluation{}).Where("id=? AND (status='failed' OR updated_at < ?)", evaluation.ID, time.Now().UTC().Add(-5*time.Minute)).Updates(map[string]any{"status": "running", "attempts": gorm.Expr("attempts+1"), "error_category": ""})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return nil
		}
	}
	max := int64(policy.MaxRecallTokens)
	temp := float64(0)
	value, err := s.invocations.Generate(ctx, aimodelruntime.GenerateRequest{
		ModelAlias:       policy.ExtractionModelAlias,
		Instructions:     prompt.MustGet("ai/memory-extraction"),
		Messages:         []aimodelruntime.Message{{Role: aimodelruntime.RoleUser, Content: []aimodelruntime.ContentPart{{Type: aimodelruntime.PartInputText, Text: string(body)}}}},
		StructuredOutput: &aimodelruntime.StructuredOutput{Name: "extract_memory_candidates", Description: "Extract memory candidates from the conversation", Schema: memoryCandidateSchema(), Strict: true},
		MaxOutputTokens:  &max,
		Temperature:      &temp,
		TimeoutMS:        60000,
		Metadata:         map[string]string{"purpose": "memory_candidate_extraction", "run_id": run.ID, "policy_version": policy.PolicyVersion, "prompt_version": policy.PromptVersion, "schema_version": policy.SchemaVersion},
	})
	if err != nil {
		_ = s.markFailed(ctx, evaluation.ID, "model_error")
		return err
	}
	response, ok := asGenerateResponse(value)
	if !ok {
		_ = s.markFailed(ctx, evaluation.ID, "invalid_response")
		return jobPermanent(fmt.Errorf("invalid extraction response"))
	}
	text := ""
	for _, item := range response.Output {
		if item.Type == aimodelruntime.PartOutputText {
			text += item.Text
		}
	}
	var output memoryExtractionOutput
	if err = strictJSON([]byte(text), &output); err != nil || len(output.Candidates) > policy.MaxCandidatesPerTurn {
		logging.Warn(ctx, "memory extraction schema mismatch", "run_id", run.ID, "error", err, "text_len", len(text), "candidate_count", len(output.Candidates), "max_allowed", policy.MaxCandidatesPerTurn)
		_ = s.markFailed(ctx, evaluation.ID, "invalid_schema")
		return jobPermanent(fmt.Errorf("invalid candidate schema"))
	}
	if err = s.persistCandidates(ctx, run, version, policy, evaluation, response.InvocationID, hash, allowed, output.Candidates); err != nil {
		category := "persistence_error"
		var validationErr memoryExtractionValidationError
		if errors.As(err, &validationErr) {
			category = validationErr.category
		}
		_ = s.markFailed(ctx, evaluation.ID, category)
		return err
	}
	types, sensitivities := map[string]int{}, map[string]int{}
	for _, candidate := range output.Candidates {
		types[string(candidate.Kind)]++
		sensitivities[string(candidate.Sensitivity)]++
	}
	logging.Info(ctx, "memory evaluation completed", "run_id", run.ID, "candidate_count", len(output.Candidates), "empty", len(output.Candidates) == 0, "type_counts", types, "sensitivity_counts", sensitivities, "duration_ms", time.Since(started).Milliseconds(), "input_tokens", response.Usage.InputTokens, "output_tokens", response.Usage.OutputTokens)
	return nil
}

type MemoryExtractionBacklog struct {
	Pending int64 `json:"pending"`
	Failed  int64 `json:"failed"`
}

func (s *MemoryExtractionService) Backlog(ctx context.Context) (MemoryExtractionBacklog, error) {
	var out MemoryExtractionBacklog
	db := s.db.WithContext(ctx)
	if err := db.Model(&MemoryEvaluation{}).Where("status='running'").Count(&out.Pending).Error; err != nil {
		return out, err
	}
	if err := db.Model(&MemoryEvaluation{}).Where("status='failed'").Count(&out.Failed).Error; err != nil {
		return out, err
	}
	return out, nil
}

func eligibleText(message ConversationMessage, expected aimodelruntime.PartType) (string, bool) {
	var parts []aimodelruntime.ContentPart
	if json.Unmarshal(message.Content, &parts) != nil || len(parts) == 0 {
		return "", false
	}
	texts := []string{}
	for _, p := range parts {
		if p.Type != expected || p.DerivedSource != nil {
			return "", false
		}
		if strings.TrimSpace(p.Text) != "" {
			texts = append(texts, p.Text)
		}
	}
	return strings.Join(texts, "\n"), len(texts) > 0
}
func strictJSON(body []byte, out any) error {
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return err
	}
	var trailing any
	if err := d.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("trailing json")
	}
	return nil
}
func sha256Hex(v []byte) string { sum := sha256.Sum256(v); return hex.EncodeToString(sum[:]) }
func stripMarkdownCodeBlock(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		// Remove opening fence (```json or ```)
		if idx := strings.Index(s, "\n"); idx != -1 {
			s = s[idx+1:]
		}
		// Remove closing fence
		if idx := strings.LastIndex(s, "```"); idx != -1 {
			s = s[:idx]
		}
	}
	return strings.TrimSpace(s)
}
func mapKeys(v map[string]bool) []string {
	out := make([]string, 0, len(v))
	for k := range v {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
func (s *MemoryExtractionService) recordSkipped(ctx context.Context, run AgentRun, reason string) error {
	now := time.Now().UTC()
	row := MemoryEvaluation{ID: uuid.NewString(), UserID: run.UserID, ConversationID: run.ConversationID, RunID: run.ID, AgentVersionID: run.AgentVersionID, Status: "completed", ErrorCategory: reason, Attempts: 1, CompletedAt: &now}
	return s.db.WithContext(ctx).Where("run_id=?", run.ID).FirstOrCreate(&row).Error
}
func (s *MemoryExtractionService) markFailed(ctx context.Context, id, category string) error {
	return s.db.WithContext(ctx).Model(&MemoryEvaluation{}).Where("id=?", id).Updates(map[string]any{"status": "failed", "error_category": category}).Error
}

func (s *MemoryExtractionService) persistCandidates(ctx context.Context, run AgentRun, version agentVersionProjection, policy aimemory.Policy, evaluation MemoryEvaluation, invocationID, argsHash string, allowed map[string]bool, candidates []memoryCandidate) error {
	now := time.Now().UTC()
	policyJSON, _ := json.Marshal(policy)
	db := s.db.WithContext(ctx)
	return db.Transaction(func(tx *gorm.DB) error {
		for i := range candidates {
			normalizeCandidate(&candidates[i])
			// Fix invalid source_message_ids: replace hallucinated IDs with the trigger message ID
			validIDs := make([]string, 0, len(candidates[i].SourceMessageIDs))
			for _, id := range candidates[i].SourceMessageIDs {
				if allowed[id] {
					validIDs = append(validIDs, id)
				}
			}
			if len(validIDs) == 0 {
				// Fallback to trigger message ID
				validIDs = []string{run.TriggerMessageID}
			}
			candidates[i].SourceMessageIDs = validIDs
			c := candidates[i]
			if err := validateCandidate(c, allowed); err != nil {
				return jobPermanent(err)
			}
			occurred, err := parseOptionalTime(c.OccurredAt)
			if err != nil {
				return jobPermanent(err)
			}
			end, err := parseOptionalTime(c.OccurredEndAt)
			if err != nil {
				return jobPermanent(err)
			}
			validFrom, err := parseOptionalTime(c.ValidFrom)
			if err != nil {
				return jobPermanent(err)
			}
			validTo, err := parseOptionalTime(c.ValidTo)
			if err != nil {
				return jobPermanent(err)
			}
			if occurred != nil && end != nil && end.Before(*occurred) {
				return jobPermanent(fmt.Errorf("invalid occurred range"))
			}
			localDate, occurredTimezone, err := candidateLocalDate(occurred, c.OccurredTimezone, run.Timezone)
			if err != nil {
				return jobPermanent(err)
			}
			if validFrom != nil && validTo != nil && !validTo.After(*validFrom) {
				return jobPermanent(fmt.Errorf("invalid validity range"))
			}
			id := uuid.NewString()
			agentID := version.AgentID
			evalID := evaluation.ID
			verID := version.ID
			contentHash := sha256Hex([]byte(c.Summary + "\x00" + c.Content))
			item := MemoryItem{ID: id, UserID: run.UserID, AgentID: &agentID, Type: c.Kind, Status: aimemory.StatusCandidate, Summary: c.Summary, Content: c.Content, Confidence: c.Confidence, Sensitivity: c.Sensitivity, Importance: c.Importance, Revision: 1, OccurredAt: occurred, OccurredEndAt: end, OccurredTimezone: occurredTimezone, LocalDate: localDate, DatePrecision: c.DatePrecision, ObservedAt: now, RecordedAt: now, ValidFrom: validFrom, ValidTo: validTo, EvaluationID: &evalID, AgentVersionID: &verID, PolicyJSON: policyJSON, PromptVersion: policy.PromptVersion, SchemaVersion: policy.SchemaVersion, ArgumentsHash: argsHash, ExplicitUserIntent: c.ExplicitUserIntent, AttributeKey: strings.TrimSpace(c.AttributeKey), AttributeValue: strings.TrimSpace(c.AttributeValue)}
			if err = tx.Create(&item).Error; err != nil {
				return err
			}
			iv := MemoryItemVersion{ID: uuid.NewString(), UserID: run.UserID, ItemID: id, Revision: 1, Summary: c.Summary, Content: c.Content, ContentHash: contentHash, CreatedAt: now}
			if err = tx.Create(&iv).Error; err != nil {
				return err
			}
			for _, messageID := range c.SourceMessageIDs {
				mid := messageID
				rid := run.ID
				aid := version.AgentID
				avid := version.ID
				uid := run.UserID
				var iid *string
				if invocationID != "" {
					value := invocationID
					iid = &value
				}
				sourceKind := aimemory.SourceAssistant
				if messageID == run.TriggerMessageID {
					sourceKind = aimemory.SourceUserStatement
				}
				e := MemoryEvidence{ID: uuid.NewString(), UserID: run.UserID, ItemID: id, SourceKind: sourceKind, ConversationID: &run.ConversationID, MessageID: &mid, RunID: &rid, AgentID: &aid, AgentVersionID: &avid, ActorUserID: &uid, ExtractionInvocationID: iid, Citation: c.Reason, ContentHash: contentHash, SourceRevision: 1, CreatedAt: now}
				if err = tx.Create(&e).Error; err != nil {
					return err
				}
			}
		}
		updates := map[string]any{"status": "completed", "candidate_count": len(candidates), "completed_at": now}
		if invocationID != "" {
			updates["invocation_id"] = invocationID
		}
		return tx.Model(&MemoryEvaluation{}).Where("id=?", evaluation.ID).Updates(updates).Error
	})
}
func normalizeCandidate(c *memoryCandidate) {
	if c.Kind == "" {
		c.Kind = aimemory.TypePreference
	}
	if c.DatePrecision == "" {
		c.DatePrecision = aimemory.DateUnknown
	}
	if c.Sensitivity == "" {
		c.Sensitivity = aimemory.SensitivityNormal
	}
	if c.Reason == "" {
		c.Reason = "preference"
	}
	if c.Confidence == 0 {
		c.Confidence = 0.8
	}
	if c.Importance == 0 {
		c.Importance = 50
	}
	// Auto-generate summary/content from attribute fields if missing
	if strings.TrimSpace(c.Summary) == "" && c.AttributeKey != "" {
		c.Summary = c.AttributeKey + ": " + c.AttributeValue
	}
	if strings.TrimSpace(c.Content) == "" {
		if c.AttributeKey != "" {
			c.Content = c.AttributeKey + ": " + c.AttributeValue
		} else {
			c.Content = c.Summary
		}
	}
}
func validateCandidate(c memoryCandidate, allowed map[string]bool) error {
	if !aimemory.ValidItemType(c.Kind) || !aimemory.ValidDatePrecision(c.DatePrecision) || strings.TrimSpace(c.Summary) == "" || strings.TrimSpace(c.Content) == "" || c.Confidence < 0 || c.Confidence > 1 || c.Importance < 0 || c.Importance > 100 || len(c.SourceMessageIDs) == 0 {
		return fmt.Errorf("invalid candidate")
	}
	if c.Sensitivity != aimemory.SensitivityNormal && c.Sensitivity != aimemory.SensitivitySensitive && c.Sensitivity != aimemory.SensitivityRestricted {
		return fmt.Errorf("invalid sensitivity")
	}
	if c.Reason != "preference" && c.Reason != "stable_fact" && c.Reason != "context" && c.Reason != "event" {
		return fmt.Errorf("invalid reason")
	}
	for _, id := range c.SourceMessageIDs {
		if !allowed[id] {
			return fmt.Errorf("source message out of bounds")
		}
	}
	return nil
}
func parseOptionalTime(v *string) (*time.Time, error) {
	if v == nil || strings.TrimSpace(*v) == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, *v)
	if err != nil {
		t, err = time.Parse("2006-01-02", *v)
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}
func candidateLocalDate(occurred *time.Time, candidateTimezone, runtimeTimezone string) (*time.Time, string, error) {
	if occurred == nil {
		return nil, "", nil
	}
	timezone := strings.TrimSpace(candidateTimezone)
	category := memoryExtractionInvalidCandidateTimezone
	diagnosticCategory := diagnostic.UpstreamInvalidResponse
	if timezone == "" {
		timezone = strings.TrimSpace(runtimeTimezone)
		category = memoryExtractionInvalidRuntimeTimezone
		diagnosticCategory = diagnostic.Validation
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		validationErr := memoryExtractionValidationError{category: category}
		return nil, "", diagnostic.New(diagnosticCategory, "ai.memory_extraction.candidate_validation", "memory extraction timezone validation failed", validationErr, nil)
	}
	local := occurred.In(location)
	date := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
	return &date, timezone, nil
}
func memoryCandidateSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","additionalProperties":false,"required":["candidates"],"properties":{"candidates":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["kind","summary","content","occurred_at","occurred_end_at","occurred_timezone","date_precision","valid_from","valid_to","confidence","sensitivity","importance","explicit_user_intent","source_message_ids","reason","attribute_key","attribute_value"],"properties":{"kind":{"type":"string","enum":["preference","fact","context","event"]},"summary":{"type":"string"},"content":{"type":"string"},"occurred_at":{"type":["string","null"]},"occurred_end_at":{"type":["string","null"]},"occurred_timezone":{"type":"string","description":"IANA timezone name such as Asia/Shanghai for occurred_at; use an empty string when the timezone cannot be determined. Do not emit abbreviations, UTC offsets, or localized names."},"date_precision":{"type":"string","enum":["exact_time","exact_day","range","approximate","unknown"]},"valid_from":{"type":["string","null"]},"valid_to":{"type":["string","null"]},"confidence":{"type":"number","minimum":0,"maximum":1},"sensitivity":{"type":"string","enum":["normal","sensitive","restricted"]},"importance":{"type":"integer","minimum":0,"maximum":100},"explicit_user_intent":{"type":"boolean"},"source_message_ids":{"type":"array","items":{"type":"string"}},"reason":{"type":"string","enum":["preference","stable_fact","context","event"]},"attribute_key":{"type":"string"},"attribute_value":{"type":"string"}}}}}}`)
}

func (s *MemoryExtractionService) FlushBeforeCompaction(ctx context.Context, conversationID string, from, to uint64) error {
	var runs []AgentRun
	err := s.db.WithContext(ctx).Where("conversation_id=? AND status='completed'", conversationID).Find(&runs).Error
	if err != nil {
		return err
	}
	for _, run := range runs {
		var trigger ConversationMessage
		if s.db.WithContext(ctx).Where("id=? AND sequence BETWEEN ? AND ?", run.TriggerMessageID, from, to).First(&trigger).Error == nil {
			if err = s.EvaluateRun(ctx, run.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

var _ aiconversation.DurableMemoryFlushHook = (*MemoryExtractionService)(nil)
