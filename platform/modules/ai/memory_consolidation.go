package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/aimemory"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodelruntime"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	tenantdata "github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"gorm.io/gorm"
)

const MemoryConsolidationJobType = "ai.memory.consolidate"

type MemoryConsolidationService struct {
	db          tenantdata.GuardedDatabase
	invocations MemoryCandidateExtractor
}

func NewMemoryConsolidationService(db *gorm.DB, invocations MemoryCandidateExtractor) *MemoryConsolidationService {
	return &MemoryConsolidationService{db: tenantdata.GuardedDB(db), invocations: invocations}
}

type consolidationPlan struct {
	Operations []consolidationOperation `json:"operations"`
}
type consolidationOperation struct {
	Operation    string `json:"operation"`
	FromItemID   string `json:"from_item_id"`
	FromRevision uint64 `json:"from_revision"`
	ToItemID     string `json:"to_item_id"`
	ToRevision   uint64 `json:"to_revision"`
	Summary      string `json:"summary"`
}

func (s *MemoryConsolidationService) Consolidate(ctx context.Context, userID string) error {
	db := s.db.WithContext(ctx)
	var items []MemoryItem
	if err := db.Where("user_id=? AND status IN ('active','pending_review')", userID).Order("importance desc,recall_count desc,recorded_at").Limit(1000).Find(&items).Error; err != nil {
		return err
	}
	if len(items) < 2 {
		return nil
	}
	var policy aimemory.Policy
	if strictJSON(items[0].PolicyJSON, &policy) != nil || policy.Validate() != nil {
		return jobPermanent(fmt.Errorf("invalid consolidation policy"))
	}
	limit := policy.MaxConsolidationItems
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	owner := uuid.NewString()
	now := time.Now().UTC()
	expires := now.Add(2 * time.Minute)
	batch := MemoryConsolidationBatch{}
	if err := db.Transaction(func(tx *gorm.DB) error {
		result := tx.Where("user_id=?", userID).Limit(1).Find(&batch)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			batch = MemoryConsolidationBatch{ID: uuid.NewString(), UserID: userID, Status: "running", LeaseOwner: owner, LeaseExpiresAt: &expires, Attempts: 1, PlanJSON: []byte(`{}`), CreatedAt: now, UpdatedAt: now}
			return tx.Create(&batch).Error
		}
		res := tx.Model(&MemoryConsolidationBatch{}).Where("id=? AND (status!='running' OR lease_expires_at<?)", batch.ID, now).Updates(map[string]any{"status": "running", "lease_owner": owner, "lease_expires_at": expires, "attempts": gorm.Expr("attempts+1"), "error_category": ""})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return errs.New(errs.CodeConflict, "memory consolidation already running")
		}
		return nil
	}); err != nil {
		return err
	}
	input := make([]map[string]any, 0, len(items))
	allowed := map[string]uint64{}
	for _, item := range items {
		input = append(input, map[string]any{"id": item.ID, "revision": item.Revision, "kind": item.Type, "status": item.Status, "summary": item.Summary, "attribute_key": item.AttributeKey, "attribute_value": item.AttributeValue, "confidence": item.Confidence, "importance": item.Importance, "recall_count": item.RecallCount, "last_recalled_at": item.LastRecalledAt, "valid_from": item.ValidFrom, "valid_to": item.ValidTo})
		allowed[item.ID] = item.Revision
	}
	body, _ := json.Marshal(map[string]any{"items": input, "policy_version": policy.PolicyVersion})
	max := int64(policy.MaxRecallTokens)
	temp := float64(0)
	value, err := s.invocations.Generate(ctx, aimodelruntime.GenerateRequest{ModelAlias: policy.ConsolidationModelAlias, Instructions: "Propose only a consolidation plan. Never delete, activate, supersede, revoke, or mutate facts. Use only supplied item IDs and exact revisions. Return no-op when uncertain.", Messages: []aimodelruntime.Message{{Role: aimodelruntime.RoleUser, Content: []aimodelruntime.ContentPart{{Type: aimodelruntime.PartInputText, Text: string(body)}}}}, StructuredOutput: &aimodelruntime.StructuredOutput{Name: "memory_consolidation_plan", Strict: true, Schema: consolidationPlanSchema()}, MaxOutputTokens: &max, Temperature: &temp, TimeoutMS: 60000, Metadata: map[string]string{"purpose": "memory_consolidation", "policy_version": policy.PolicyVersion}})
	if err != nil {
		_ = s.fail(ctx, batch.ID, owner, "model_error")
		return err
	}
	response, ok := asGenerateResponse(value)
	if !ok {
		_ = s.fail(ctx, batch.ID, owner, "invalid_response")
		return jobPermanent(fmt.Errorf("invalid consolidation response"))
	}
	text := ""
	for _, output := range response.Output {
		if output.Type == aimodelruntime.PartOutputText {
			text += output.Text
		}
	}
	var plan consolidationPlan
	if strictJSON([]byte(text), &plan) != nil {
		_ = s.fail(ctx, batch.ID, owner, "invalid_schema")
		return jobPermanent(fmt.Errorf("invalid consolidation plan"))
	}
	if err = validateConsolidationPlan(plan, allowed); err != nil {
		_ = s.fail(ctx, batch.ID, owner, "invalid_plan")
		return jobPermanent(err)
	}
	planJSON, _ := json.Marshal(plan)
	finished := time.Now().UTC()
	return db.Transaction(func(tx *gorm.DB) error {
		for _, operation := range plan.Operations {
			if operation.Operation != "duplicate" && operation.Operation != "conflict" {
				continue
			}
			relation := MemoryRelation{ID: uuid.NewString(), UserID: userID, FromItemID: operation.FromItemID, ToItemID: operation.ToItemID, Kind: operation.Operation, CreatedAt: finished}
			var existing MemoryRelation
			if result := tx.Where("from_item_id=? AND to_item_id=? AND kind=?", relation.FromItemID, relation.ToItemID, relation.Kind).Limit(1).Find(&existing); result.Error != nil {
				return result.Error
			} else if result.RowsAffected == 0 {
				if err := tx.Create(&relation).Error; err != nil {
					return err
				}
			}
		}
		updates := map[string]any{"status": "completed", "lease_owner": "", "lease_expires_at": nil, "watermark_at": finished, "plan_json": planJSON, "error_category": ""}
		if response.InvocationID != "" {
			updates["invocation_id"] = response.InvocationID
		}
		res := tx.Model(&MemoryConsolidationBatch{}).Where("id=? AND lease_owner=?", batch.ID, owner).Updates(updates)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return errs.New(errs.CodeConflict, "memory consolidation lease changed")
		}
		return nil
	})
}
func (s *MemoryConsolidationService) fail(ctx context.Context, id, owner, category string) error {
	return s.db.WithContext(ctx).Model(&MemoryConsolidationBatch{}).Where("id=? AND lease_owner=?", id, owner).Updates(map[string]any{"status": "failed", "lease_owner": "", "lease_expires_at": nil, "error_category": category}).Error
}
func validateConsolidationPlan(plan consolidationPlan, allowed map[string]uint64) error {
	if len(plan.Operations) > 100 {
		return fmt.Errorf("too many consolidation operations")
	}
	for _, op := range plan.Operations {
		switch op.Operation {
		case "noop":
			continue
		case "duplicate", "conflict", "summary":
		default:
			return fmt.Errorf("invalid consolidation operation")
		}
		if op.FromItemID == "" || allowed[op.FromItemID] != op.FromRevision {
			return fmt.Errorf("from item revision drift")
		}
		if op.Operation != "summary" && (op.ToItemID == "" || allowed[op.ToItemID] != op.ToRevision || op.ToItemID == op.FromItemID) {
			return fmt.Errorf("to item revision drift")
		}
	}
	return nil
}
func consolidationPlanSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","additionalProperties":false,"required":["operations"],"properties":{"operations":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["operation","from_item_id","from_revision","to_item_id","to_revision","summary"],"properties":{"operation":{"type":"string","enum":["noop","duplicate","conflict","summary"]},"from_item_id":{"type":"string"},"from_revision":{"type":"integer"},"to_item_id":{"type":"string"},"to_revision":{"type":"integer"},"summary":{"type":"string"}}}}}}`)
}
