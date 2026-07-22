package ai

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/aimemory"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/outbox"
	"github.com/yzxingtu2026/ainp/platform/framework/pagination"
	tenantdata "github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"gorm.io/gorm"
)

type MemoryManagementService struct {
	db         tenantdata.GuardedDatabase
	governance *MemoryGovernanceService
	writer     outbox.Writer
}

func NewMemoryManagementService(db *gorm.DB, g *MemoryGovernanceService, w outbox.Writer) *MemoryManagementService {
	return &MemoryManagementService{db: tenantdata.GuardedDB(db), governance: g, writer: w}
}

type MemoryListRequest struct {
	pagination.Request
	Keyword     string               `json:"keyword" validate:"omitempty"`
	Status      aimemory.ItemStatus  `json:"status" validate:"omitempty"`
	Type        aimemory.ItemType    `json:"type" validate:"omitempty"`
	Sensitivity aimemory.Sensitivity `json:"sensitivity" validate:"omitempty"`
	AgentID     string               `json:"agent_id" validate:"omitempty"`
	DateFrom    string               `json:"date_from" validate:"omitempty"`
	DateTo      string               `json:"date_to" validate:"omitempty"`
}
type MemoryIDRequest struct {
	ID       string `json:"id" validate:"required"`
	Revision uint64 `json:"revision" validate:"required"`
}
type MemoryCorrectRequest struct {
	ID            string                 `json:"id" validate:"required"`
	Revision      uint64                 `json:"revision" validate:"required"`
	Summary       string                 `json:"summary" validate:"required"`
	Content       string                 `json:"content" validate:"required"`
	LocalDate     string                 `json:"local_date" validate:"omitempty"`
	DatePrecision aimemory.DatePrecision `json:"date_precision" validate:"omitempty"`
}
type MemoryClearRequest struct {
	RevisionToken string `json:"revision_token" validate:"required"`
	Scope         string `json:"scope" validate:"required"`
	AgentID       string `json:"agent_id" validate:"omitempty"`
	BeforeDate    string `json:"before_date" validate:"omitempty"`
}
type MemoryItemView struct {
	MemoryItem
	Evidence []MemoryEvidence `json:"evidence"`
	Current  bool             `json:"is_current"`
	Stale    bool             `json:"stale"`
}

type MemoryListResult struct {
	Items         []MemoryItemView `json:"items"`
	Total         int64            `json:"total"`
	Page          int              `json:"page"`
	PageSize      int              `json:"page_size"`
	RevisionToken string           `json:"revision_token"`
}

func memoryRevisionToken(tx *gorm.DB, userID string) (string, error) {
	var rows []struct {
		ID       string
		Revision uint64
	}
	err := tx.Model(&MemoryItem{}).
		Select("id, revision").
		Where("user_id=? AND status IN ?", userID, []aimemory.ItemStatus{aimemory.StatusActive, aimemory.StatusPendingReview, aimemory.StatusCandidate}).
		Order("id").Find(&rows).Error
	if err != nil {
		return "", err
	}
	h := sha256.New()
	for _, row := range rows {
		_, _ = fmt.Fprintf(h, "%s:%d\n", row.ID, row.Revision)
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func memoryActor(ctx context.Context) (*auth.Context, error) {
	ac := auth.MustFromContext(ctx)
	if !ac.Authenticated || ac.TenantID == "" || ac.UserID == "" {
		return nil, errs.New(errs.CodeUnauthorized, "unauthorized")
	}
	return ac, nil
}
func (s *MemoryManagementService) List(ctx context.Context, r MemoryListRequest) (any, error) {
	ac, err := memoryActor(ctx)
	if err != nil {
		return nil, err
	}
	r.Normalize()
	q := s.db.WithContext(ctx).Where("user_id=?", ac.UserID)
	if r.Keyword = strings.TrimSpace(r.Keyword); r.Keyword != "" {
		like := "%" + r.Keyword + "%"
		q = q.Where("(summary ILIKE ? OR content ILIKE ?)", like, like)
	}
	if r.Status != "" {
		q = q.Where("status=?", r.Status)
	}
	if r.Type != "" {
		q = q.Where("type=?", r.Type)
	}
	if r.Sensitivity != "" {
		q = q.Where("sensitivity=?", r.Sensitivity)
	}
	if r.AgentID != "" {
		q = q.Where("agent_id=?", r.AgentID)
	}
	if r.DateFrom != "" {
		q = q.Where("local_date>=?", r.DateFrom)
	}
	if r.DateTo != "" {
		q = q.Where("local_date<=?", r.DateTo)
	}
	var total int64
	if err = q.Model(&MemoryItem{}).Count(&total).Error; err != nil {
		return nil, err
	}
	var rows []MemoryItem
	err = q.Order("COALESCE(local_date,DATE '0001-01-01') DESC,recorded_at DESC,id").Offset(r.Offset()).Limit(r.PageSize).Find(&rows).Error
	if err != nil {
		return nil, err
	}
	views := make([]MemoryItemView, len(rows))
	now := time.Now().UTC()
	for i, row := range rows {
		views[i] = MemoryItemView{MemoryItem: row, Current: row.Status == aimemory.StatusActive && (row.ExpiresAt == nil || row.ExpiresAt.After(now)) && (row.ValidFrom == nil || !row.ValidFrom.After(now)) && (row.ValidTo == nil || row.ValidTo.After(now))}
	}
	token, err := memoryRevisionToken(s.db.WithContext(ctx), ac.UserID)
	if err != nil {
		return nil, err
	}
	page := pagination.NewResult(views, total, r.Request)
	return MemoryListResult{Items: page.Items, Total: page.Total, Page: page.Page, PageSize: page.PageSize, RevisionToken: token}, nil
}
func (s *MemoryManagementService) Get(ctx context.Context, r MemoryIDRequest) (any, error) {
	ac, err := memoryActor(ctx)
	if err != nil {
		return nil, err
	}
	var item MemoryItem
	if err = s.db.WithContext(ctx).Where("id=? AND user_id=?", r.ID, ac.UserID).First(&item).Error; err != nil {
		return nil, errs.New(errs.CodeNotFound, "memory not found")
	}
	var evidence []MemoryEvidence
	if err = s.db.WithContext(ctx).Where("item_id=? AND user_id=?", item.ID, ac.UserID).Order("created_at,id").Find(&evidence).Error; err != nil {
		return nil, err
	}
	for i := range evidence {
		evidence[i].Citation = ""
	}
	return MemoryItemView{MemoryItem: item, Evidence: evidence, Current: item.Status == aimemory.StatusActive}, nil
}
func (s *MemoryManagementService) Profile(ctx context.Context, _ json.RawMessage) (any, error) {
	ac, err := memoryActor(ctx)
	if err != nil {
		return nil, err
	}
	var row MemoryCurrentProfile
	if err = s.db.WithContext(ctx).Where("user_id=? AND status=?", ac.UserID, aimemory.ProjectionReady).Order("source_revision DESC").First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return map[string]any{"profile": map[string]any{}, "source_items": []any{}}, nil
		}
		return nil, err
	}
	var profile any
	_ = json.Unmarshal(row.ProfileJSON, &profile)
	var refs any
	_ = json.Unmarshal(row.SourceItemsJSON, &refs)
	return map[string]any{"profile": profile, "source_items": refs, "source_revision": row.SourceRevision, "updated_at": row.UpdatedAt}, nil
}
func (s *MemoryManagementService) Decision(ctx context.Context, r MemoryIDRequest, action string) (any, error) {
	ac, err := memoryActor(ctx)
	if err != nil {
		return nil, err
	}
	var item MemoryItem
	if err = s.db.WithContext(ctx).Where("id=? AND user_id=?", r.ID, ac.UserID).First(&item).Error; err != nil {
		return nil, errs.New(errs.CodeNotFound, "memory not found")
	}
	key := strings.TrimSpace(ac.IdempotencyKey)
	if key == "" {
		return nil, errs.New(errs.CodeIdempotencyKeyRequired, "idempotency key required")
	}
	if err = s.governance.Decide(ctx, MemoryDecisionRequest{ItemID: item.ID, Revision: r.Revision, Action: action, IdempotencyKey: key}); err != nil {
		return nil, err
	}
	return map[string]any{"id": item.ID, "status": action}, nil
}
func (s *MemoryManagementService) Correct(ctx context.Context, r MemoryCorrectRequest) (any, error) {
	ac, err := memoryActor(ctx)
	if err != nil {
		return nil, err
	}
	key := strings.TrimSpace(ac.IdempotencyKey)
	if key == "" {
		return nil, errs.New(errs.CodeIdempotencyKeyRequired, "idempotency key required")
	}
	r.Summary = strings.TrimSpace(r.Summary)
	r.Content = strings.TrimSpace(r.Content)
	if r.ID == "" || r.Revision == 0 || r.Summary == "" || r.Content == "" || !aimemory.ValidDatePrecision(r.DatePrecision) {
		return nil, errs.New(errs.CodeValidation, "invalid correction")
	}
	var local *time.Time
	if r.LocalDate != "" {
		value, e := time.Parse("2006-01-02", r.LocalDate)
		if e != nil {
			return nil, errs.New(errs.CodeValidation, "invalid local date")
		}
		local = &value
	}
	now := time.Now().UTC()
	var created MemoryItem
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var prior MemoryGovernanceDecision
		if res := tx.Where("idempotency_key=?", key).Find(&prior); res.Error != nil {
			return res.Error
		} else if res.RowsAffected == 1 {
			return tx.Where("id=? AND user_id=?", prior.ItemID, ac.UserID).First(&created).Error
		}
		var old MemoryItem
		if e := tx.Where("id=? AND user_id=? AND revision=? AND status='active'", r.ID, ac.UserID, r.Revision).First(&old).Error; e != nil {
			return errs.New(errs.CodeConflict, "memory revision conflict")
		}
		if res := tx.Model(&old).Where("revision=?", r.Revision).Updates(map[string]any{"status": aimemory.StatusSuperseded, "valid_to": now, "revision": gorm.Expr("revision+1")}); res.Error != nil || res.RowsAffected != 1 {
			return errs.New(errs.CodeConflict, "memory revision conflict")
		}
		created = MemoryItem{ID: uuid.NewString(), UserID: ac.UserID, AgentID: old.AgentID, Type: old.Type, Status: aimemory.StatusActive, Summary: r.Summary, Content: r.Content, Confidence: 1, Sensitivity: old.Sensitivity, Importance: old.Importance, Revision: 1, OccurredTimezone: old.OccurredTimezone, LocalDate: local, DatePrecision: r.DatePrecision, ObservedAt: now, RecordedAt: now, ValidFrom: &now, SupersedesItemID: &old.ID, ExplicitUserIntent: true, CreatedAt: now, UpdatedAt: now}
		if e := tx.Create(&created).Error; e != nil {
			return e
		}
		hash := fmt.Sprintf("%x", sha256.Sum256([]byte(r.Summary+"\x00"+r.Content)))
		if e := tx.Create(&MemoryItemVersion{ID: uuid.NewString(), UserID: ac.UserID, ItemID: created.ID, Revision: 1, Summary: r.Summary, Content: r.Content, ContentHash: hash, CreatedAt: now}).Error; e != nil {
			return e
		}
		if e := tx.Create(&MemoryEvidence{ID: uuid.NewString(), UserID: ac.UserID, ItemID: created.ID, SourceKind: aimemory.SourceUserStatement, ActorUserID: &ac.UserID, ContentHash: hash, SourceRevision: 1, CreatedAt: now}).Error; e != nil {
			return e
		}
		if e := tx.Create(&MemoryRelation{ID: uuid.NewString(), UserID: ac.UserID, FromItemID: created.ID, ToItemID: old.ID, Kind: "supersedes", CreatedAt: now}).Error; e != nil {
			return e
		}
		if e := tx.Create(&MemoryGovernanceDecision{ID: uuid.NewString(), UserID: ac.UserID, ItemID: created.ID, ItemRevision: 1, Action: "correct", Reason: "user_correction", RelatedItemID: &old.ID, IdempotencyKey: key, CreatedAt: now}).Error; e != nil {
			return e
		}
		if s.writer != nil {
			_, e := s.writer.Append(ctx, tx, outbox.AppendInput{EventType: MemoryItemChangedEvent, AggregateType: "ai_memory_item", AggregateID: created.ID, AggregateVersion: 1, Payload: map[string]any{"item_id": created.ID, "user_id": ac.UserID, "status": aimemory.StatusActive, "revision": 1, "action": "correct"}})
			return e
		}
		return nil
	})
	return created, err
}
func (s *MemoryManagementService) Clear(ctx context.Context, r MemoryClearRequest) (any, error) {
	ac, err := memoryActor(ctx)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(ac.IdempotencyKey) == "" {
		return nil, errs.New(errs.CodeIdempotencyKeyRequired, "idempotency key required")
	}
	if !((r.Scope == "all") || (r.Scope == "agent" && r.AgentID != "") || (r.Scope == "before" && r.BeforeDate != "")) {
		return nil, errs.New(errs.CodeValidation, "invalid clear scope")
	}
	var count int64
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		currentToken, e := memoryRevisionToken(tx, ac.UserID)
		if e != nil {
			return e
		}
		if strings.TrimSpace(r.RevisionToken) == "" || r.RevisionToken != currentToken {
			return errs.New(errs.CodeConflict, "memory revision conflict")
		}
		now := time.Now().UTC()
		q := tx.Model(&MemoryItem{}).Where("user_id=? AND status IN ?", ac.UserID, []aimemory.ItemStatus{aimemory.StatusActive, aimemory.StatusPendingReview, aimemory.StatusCandidate})
		if r.Scope == "agent" {
			q = q.Where("agent_id=?", r.AgentID)
		} else if r.Scope == "before" {
			q = q.Where("local_date<?", r.BeforeDate)
		}
		res := q.Updates(map[string]any{"status": aimemory.StatusDeleted, "revision": gorm.Expr("revision+1"), "updated_at": now})
		if res.Error != nil {
			return res.Error
		}
		count = res.RowsAffected
		if s.writer != nil {
			version := int64(1)
			if count > 0 {
				version = now.UnixNano()
			}
			_, e := s.writer.Append(ctx, tx, outbox.AppendInput{EventType: MemoryItemChangedEvent, AggregateType: "ai_memory_user", AggregateID: ac.UserID, AggregateVersion: version, Payload: map[string]any{"user_id": ac.UserID, "status": aimemory.StatusDeleted, "action": "clear", "count": count}})
			return e
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"deleted_count": count}, nil
}
