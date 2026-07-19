package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/aimemory"
	"github.com/yzxingtu2026/ainp/platform/framework/aimodelruntime"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/outbox"
	tenantdata "github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"gorm.io/gorm"
)

const MemoryItemChangedEvent = "ai.memory.item.changed"
const MemoryProjectionJobType = "ai.memory.rebuild_projections"

type MemoryGovernanceService struct {
	db     tenantdata.GuardedDatabase
	writer outbox.Writer
}

func NewMemoryGovernanceService(db *gorm.DB, writer outbox.Writer) *MemoryGovernanceService {
	return &MemoryGovernanceService{db: tenantdata.GuardedDB(db), writer: writer}
}

type MemoryDecisionRequest struct {
	ItemID           string
	Revision         uint64
	Action           string
	SupersedesItemID *string
	IdempotencyKey   string
}

func (s *MemoryGovernanceService) Decide(ctx context.Context, r MemoryDecisionRequest) error {
	if r.ItemID == "" || r.Revision == 0 || r.IdempotencyKey == "" {
		return errs.New(errs.CodeValidation, "invalid memory decision")
	}
	if r.Action != "activate" && r.Action != "reject" && r.Action != "revoke" {
		return errs.New(errs.CodeValidation, "invalid memory action")
	}
	db := s.db.WithContext(ctx)
	return db.Transaction(func(tx *gorm.DB) error {
		var prior MemoryGovernanceDecision
		if result := tx.Where("idempotency_key=?", r.IdempotencyKey).Limit(1).Find(&prior); result.Error != nil {
			return result.Error
		} else if result.RowsAffected == 1 {
			return nil
		}
		var item MemoryItem
		if err := tx.Where("id=?", r.ItemID).First(&item).Error; err != nil {
			return err
		}
		from := []aimemory.ItemStatus{aimemory.StatusPendingReview}
		to := aimemory.StatusActive
		action := r.Action
		if r.Action == "reject" {
			to = aimemory.StatusRejected
		}
		if r.Action == "revoke" {
			from = []aimemory.ItemStatus{aimemory.StatusActive}
			to = aimemory.StatusRevoked
		}
		now := time.Now().UTC()
		updates := map[string]any{"status": to, "revision": gorm.Expr("revision+1")}
		var relatedID *string
		if r.SupersedesItemID != nil && r.Action == "activate" {
			var old MemoryItem
			if err := tx.Where("id=? AND user_id=? AND status='active'", *r.SupersedesItemID, item.UserID).First(&old).Error; err != nil {
				return errs.New(errs.CodeConflict, "superseded memory unavailable")
			}
			if res := tx.Model(&MemoryItem{}).Where("id=? AND revision=?", old.ID, old.Revision).Updates(map[string]any{"status": aimemory.StatusSuperseded, "valid_to": now, "revision": gorm.Expr("revision+1")}); res.Error != nil {
				return res.Error
			} else if res.RowsAffected != 1 {
				return errs.New(errs.CodeConflict, "memory revision conflict")
			}
			updates["supersedes_item_id"] = old.ID
			action = "supersede"
			relatedID = &old.ID
			if err := tx.Create(&MemoryRelation{ID: uuid.NewString(), UserID: item.UserID, FromItemID: item.ID, ToItemID: old.ID, Kind: "supersedes", CreatedAt: now}).Error; err != nil {
				return err
			}
		}
		res := tx.Model(&MemoryItem{}).Where("id=? AND revision=? AND status IN ?", item.ID, r.Revision, from).Updates(updates)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return errs.New(errs.CodeConflict, "memory revision conflict")
		}
		policyVersion := ""
		var policy aimemory.Policy
		if strictJSON(item.PolicyJSON, &policy) == nil {
			policyVersion = policy.PolicyVersion
		}
		decision := MemoryGovernanceDecision{ID: uuid.NewString(), UserID: item.UserID, ItemID: item.ID, ItemRevision: r.Revision, Action: action, Reason: "user_decision", RelatedItemID: relatedID, PolicyVersion: policyVersion, IdempotencyKey: r.IdempotencyKey, CreatedAt: now}
		if err := tx.Create(&decision).Error; err != nil {
			return err
		}
		if s.writer != nil {
			_, err := s.writer.Append(ctx, tx, outbox.AppendInput{EventType: MemoryItemChangedEvent, AggregateType: "ai_memory_item", AggregateID: item.ID, AggregateVersion: int64(r.Revision + 1), Payload: map[string]any{"item_id": item.ID, "user_id": item.UserID, "status": to, "revision": r.Revision + 1, "action": action}})
			return err
		}
		return nil
	})
}

func (s *MemoryGovernanceService) GovernRun(ctx context.Context, runID string) error {
	db := s.db.WithContext(ctx)
	var evaluation MemoryEvaluation
	if err := db.Where("run_id=? AND status='completed'", runID).First(&evaluation).Error; err != nil {
		return err
	}
	var items []MemoryItem
	if err := db.Where("evaluation_id=? AND status='candidate'", evaluation.ID).Order("created_at,id").Find(&items).Error; err != nil {
		return err
	}
	for i := range items {
		if err := s.governOne(ctx, &items[i]); err != nil {
			return err
		}
	}
	return nil
}
func (s *MemoryGovernanceService) governOne(ctx context.Context, item *MemoryItem) error {
	var policy aimemory.Policy
	if strictJSON(item.PolicyJSON, &policy) != nil {
		return errs.New(errs.CodeValidation, "invalid candidate policy")
	}
	db := s.db.WithContext(ctx)
	return db.Transaction(func(tx *gorm.DB) error {
		var existing MemoryGovernanceDecision
		if result := tx.Where("item_id=?", item.ID).Limit(1).Find(&existing); result.Error != nil {
			return result.Error
		} else if result.RowsAffected == 1 {
			return nil
		}
		var active []MemoryItem
		query := tx.Where("user_id=? AND status='active' AND type=?", item.UserID, item.Type)
		if item.AgentID != nil {
			query = query.Where("agent_id=?", *item.AgentID)
		}
		if err := query.Order("recorded_at desc").Find(&active).Error; err != nil {
			return err
		}
		action, reason, related := governanceOutcome(*item, policy, active)
		now := time.Now().UTC()
		updates := map[string]any{"revision": gorm.Expr("revision+1")}
		switch action {
		case "activate":
			updates["status"] = aimemory.StatusActive
		case "pending_review":
			updates["status"] = aimemory.StatusPendingReview
		case "reject", "duplicate":
			updates["status"] = aimemory.StatusRejected
		case "supersede":
			updates["status"] = aimemory.StatusActive
			updates["supersedes_item_id"] = related.ID
			if res := tx.Model(&MemoryItem{}).Where("id=? AND revision=? AND status='active'", related.ID, related.Revision).Updates(map[string]any{"status": aimemory.StatusSuperseded, "valid_to": now, "revision": gorm.Expr("revision+1")}); res.Error != nil {
				return res.Error
			} else if res.RowsAffected != 1 {
				return errs.New(errs.CodeConflict, "memory revision conflict")
			}
		}
		res := tx.Model(&MemoryItem{}).Where("id=? AND revision=? AND status='candidate'", item.ID, item.Revision).Updates(updates)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return errs.New(errs.CodeConflict, "memory revision conflict")
		}
		var relatedID *string
		if related != nil {
			id := related.ID
			relatedID = &id
			kind := "conflict"
			if action == "duplicate" {
				kind = "duplicate"
			}
			if action == "supersede" {
				kind = "supersedes"
			}
			relation := MemoryRelation{ID: uuid.NewString(), UserID: item.UserID, FromItemID: item.ID, ToItemID: related.ID, Kind: kind, CreatedAt: now}
			if err := tx.Create(&relation).Error; err != nil {
				return err
			}
			if action == "duplicate" {
				var evidence []MemoryEvidence
				if err := tx.Where("item_id=?", item.ID).Find(&evidence).Error; err != nil {
					return err
				}
				for _, source := range evidence {
					source.ID = uuid.NewString()
					source.ItemID = related.ID
					source.CreatedAt = now
					if err := tx.Create(&source).Error; err != nil {
						return err
					}
				}
			}
		}
		decision := MemoryGovernanceDecision{ID: uuid.NewString(), UserID: item.UserID, ItemID: item.ID, ItemRevision: item.Revision, Action: action, Reason: reason, RelatedItemID: relatedID, PolicyVersion: policy.PolicyVersion, IdempotencyKey: "govern:" + item.ID + ":" + fmt.Sprint(item.Revision), CreatedAt: now}
		if err := tx.Create(&decision).Error; err != nil {
			return err
		}
		if s.writer != nil && (action == "activate" || action == "supersede") {
			_, err := s.writer.Append(ctx, tx, outbox.AppendInput{EventType: MemoryItemChangedEvent, AggregateType: "ai_memory_item", AggregateID: item.ID, AggregateVersion: int64(item.Revision + 1), Payload: map[string]any{"item_id": item.ID, "user_id": item.UserID, "status": updates["status"], "revision": item.Revision + 1, "action": action}})
			return err
		}
		return nil
	})
}
func equivalentMemory(a, b MemoryItem) bool {
	return a.Type == b.Type && strings.EqualFold(strings.TrimSpace(a.Content), strings.TrimSpace(b.Content)) && a.AttributeKey == b.AttributeKey && a.AttributeValue == b.AttributeValue
}
func governanceOutcome(item MemoryItem, policy aimemory.Policy, active []MemoryItem) (string, string, *MemoryItem) {
	for i := range active {
		if equivalentMemory(active[i], item) {
			return "duplicate", "equivalent_active", &active[i]
		}
	}
	if item.Confidence < 0.2 {
		return "reject", "confidence_below_floor", nil
	}
	auto := policy.AutoSaveLowSensitive && item.ExplicitUserIntent && item.Sensitivity == aimemory.SensitivityNormal && item.Confidence >= 0.85 && (item.Type == aimemory.TypePreference || item.Type == aimemory.TypeFact)
	if !auto {
		return "pending_review", "policy_review_required", nil
	}
	if conflict := findConflict(active, item); conflict != nil {
		if item.Type == aimemory.TypePreference && item.AttributeKey != "" {
			return "supersede", "explicit_preference_correction", conflict
		}
		return "pending_review", "conflict_requires_review", conflict
	}
	return "activate", "explicit_low_sensitive", nil
}
func findConflict(active []MemoryItem, item MemoryItem) *MemoryItem {
	for i := range active {
		if item.AttributeKey != "" && active[i].AttributeKey == item.AttributeKey && active[i].AttributeValue != item.AttributeValue {
			return &active[i]
		}
	}
	return nil
}

type memorySourceRef struct {
	ID       string `json:"id"`
	Revision uint64 `json:"revision"`
}
type MemoryProjectionService struct {
	db         tenantdata.GuardedDatabase
	embeddings aimodelruntime.EmbeddingRuntime
}

func NewMemoryProjectionService(db *gorm.DB, embeddings ...aimodelruntime.EmbeddingRuntime) *MemoryProjectionService {
	var runtime aimodelruntime.EmbeddingRuntime
	if len(embeddings) > 0 {
		runtime = embeddings[0]
	}
	return &MemoryProjectionService{db: tenantdata.GuardedDB(db), embeddings: runtime}
}
func (s *MemoryProjectionService) Rebuild(ctx context.Context, userID string) error {
	now := time.Now().UTC()
	db := s.db.WithContext(ctx)
	var items []MemoryItem
	if err := db.Where("user_id=? AND status='active' AND (expires_at IS NULL OR expires_at>?) AND (valid_from IS NULL OR valid_from<=?) AND (valid_to IS NULL OR valid_to>?)", userID, now, now, now).Order("recorded_at,id").Find(&items).Error; err != nil {
		return err
	}
	vectorRows := map[string]MemoryProjection{}
	if s.embeddings != nil && len(items) > 0 {
		inputs := make([]aimodelruntime.EmbeddingItem, len(items))
		for i, item := range items {
			inputs[i] = aimodelruntime.EmbeddingItem{ID: item.ID, Text: item.Summary + "\n" + item.Content}
		}
		if response, embedErr := s.embeddings.Embed(ctx, aimodelruntime.EmbeddingRequest{ModelAlias: "default", Inputs: inputs, Purpose: aimodelruntime.EmbeddingPurposeMemory, Preprocessing: aimodelruntime.EmbeddingPreprocessingCosineNorm}); embedErr == nil && response != nil && response.Dimensions > 0 && response.Model.DeploymentID != "" {
			for _, vector := range response.Vectors {
				body, _ := json.Marshal(vector.Vector)
				vectorRows[vector.ID] = MemoryProjection{ID: uuid.NewString(), UserID: userID, ItemID: vector.ID, Kind: aimemory.ProjectionVector, Status: aimemory.ProjectionReady, ModelAlias: "default", DeploymentID: response.Model.DeploymentID, Dimensions: response.Dimensions, VectorJSON: body, CreatedAt: now, UpdatedAt: now}
			}
		}
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id=? AND kind IN ?", userID, []aimemory.ProjectionKind{aimemory.ProjectionExact, aimemory.ProjectionFTS, aimemory.ProjectionVector}).Delete(&MemoryProjection{}).Error; err != nil {
			return err
		}
		for _, item := range items {
			text := item.Summary + "\n" + item.Content
			hash := sha256Hex([]byte(text))
			for _, kind := range []aimemory.ProjectionKind{aimemory.ProjectionExact, aimemory.ProjectionFTS} {
				row := MemoryProjection{ID: uuid.NewString(), UserID: userID, ItemID: item.ID, Kind: kind, Status: aimemory.ProjectionReady, ItemRevision: item.Revision, ContentHash: hash, SearchText: text, CreatedAt: now, UpdatedAt: now}
				if err := tx.Create(&row).Error; err != nil {
					return err
				}
			}
			if row, ok := vectorRows[item.ID]; ok {
				row.ItemRevision = item.Revision
				row.ContentHash = hash
				if err := tx.Create(&row).Error; err != nil {
					return err
				}
			}
		}
		if err := tx.Where("user_id=?", userID).Delete(&MemoryDaySummary{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id=?", userID).Delete(&MemoryCurrentProfile{}).Error; err != nil {
			return err
		}
		groups := map[string][]MemoryItem{}
		for _, item := range items {
			key := dayGroupKey(item)
			groups[key] = append(groups[key], item)
		}
		keys := make([]string, 0, len(groups))
		for key := range groups {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			group := groups[key]
			date, dateGroup := projectionDate(group[0])
			refs, revision, policy := sourceRefs(group)
			encoded, _ := json.Marshal(refs)
			row := MemoryDaySummary{ID: uuid.NewString(), UserID: userID, LocalDate: date, Timezone: group[0].OccurredTimezone, SourceRevision: revision, Status: aimemory.ProjectionReady, Summary: fmt.Sprintf("%d active memory items", len(group)), ContentHash: sha256Hex(encoded), DateGroup: dateGroup, PolicyVersion: policy, SourceItemsJSON: encoded, CreatedAt: now, UpdatedAt: now}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		}
		profile := map[string]any{"preferred_address": "", "preferences": map[string]string{}, "facts": map[string]string{}}
		preferences := profile["preferences"].(map[string]string)
		facts := profile["facts"].(map[string]string)
		for _, item := range items {
			if item.AttributeKey == "preferred_address" {
				profile["preferred_address"] = item.AttributeValue
			}
			if item.AttributeKey != "" {
				if item.Type == aimemory.TypePreference {
					preferences[item.AttributeKey] = item.AttributeValue
				} else if item.Type == aimemory.TypeFact {
					facts[item.AttributeKey] = item.AttributeValue
				}
			}
		}
		payload, _ := json.Marshal(profile)
		refs, revision, policy := sourceRefs(items)
		encoded, _ := json.Marshal(refs)
		row := MemoryCurrentProfile{ID: uuid.NewString(), UserID: userID, SourceRevision: revision, Status: aimemory.ProjectionReady, ProfileJSON: payload, ContentHash: sha256Hex(payload), PolicyVersion: policy, SourceItemsJSON: encoded, CreatedAt: now, UpdatedAt: now}
		return tx.Create(&row).Error
	})
}
func dayGroupKey(item MemoryItem) string {
	date, group := projectionDate(item)
	return date.Format("2006-01-02") + "|" + item.OccurredTimezone + "|" + group
}
func projectionDate(item MemoryItem) (time.Time, string) {
	if item.DatePrecision == aimemory.DateUnknown || item.LocalDate == nil {
		return time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC), "unknown"
	}
	if item.DatePrecision == aimemory.DateRange {
		return *item.LocalDate, "range"
	}
	return *item.LocalDate, "exact"
}
func sourceRefs(items []MemoryItem) ([]memorySourceRef, uint64, string) {
	refs := make([]memorySourceRef, 0, len(items))
	var revision uint64
	policy := ""
	for _, item := range items {
		refs = append(refs, memorySourceRef{item.ID, item.Revision})
		revision += item.Revision
		var p aimemory.Policy
		if strictJSON(item.PolicyJSON, &p) == nil && p.PolicyVersion > policy {
			policy = p.PolicyVersion
		}
	}
	return refs, revision, policy
}
