package ai

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/aimemory"
	tenantdata "github.com/yzxingtu2026/ainp/platform/framework/tenant"
	"gorm.io/gorm"
)

type MemoryService struct {
	db   tenantdata.GuardedDatabase
	repo *MemoryRepository
}

func NewMemoryService(db *gorm.DB) (*MemoryService, error) {
	if err := registerMemoryTenantModels(); err != nil {
		return nil, err
	}
	return &MemoryService{db: tenantdata.GuardedDB(db), repo: NewMemoryRepository()}, nil
}

type MemoryCreateRequest struct {
	UserID                                                                                               string
	AgentID                                                                                              *string
	Type                                                                                                 aimemory.ItemType
	Status                                                                                               aimemory.ItemStatus
	Summary, Content                                                                                     string
	Confidence                                                                                           float64
	Sensitivity                                                                                          aimemory.Sensitivity
	Importance                                                                                           int
	OccurredAt, OccurredEndAt                                                                            *time.Time
	OccurredTimezone                                                                                     string
	LocalDate                                                                                            *time.Time
	DatePrecision                                                                                        aimemory.DatePrecision
	ObservedAt                                                                                           time.Time
	ValidFrom, ValidTo, ExpiresAt                                                                        *time.Time
	SupersedesItemID                                                                                     *string
	SourceKind                                                                                           aimemory.SourceKind
	ConversationID, MessageID, RunID, SourceAgentID, AgentVersionID, ActorUserID, ExtractionInvocationID *string
	Citation                                                                                             string
}

func (s *MemoryService) Create(ctx context.Context, req MemoryCreateRequest) (*MemoryItem, error) {
	req.UserID, req.Summary, req.Content = strings.TrimSpace(req.UserID), strings.TrimSpace(req.Summary), strings.TrimSpace(req.Content)
	if err := ValidateMemoryCreateRequest(req); err != nil {
		return nil, err
	}
	if req.SupersedesItemID != nil {
		previous, err := s.repo.Get(ctx, s.db, *req.SupersedesItemID)
		if err != nil {
			return nil, fmt.Errorf("superseded item: %w", err)
		}
		if previous.UserID != req.UserID {
			return nil, aimemory.ErrInvalidMemory
		}
	}
	now := time.Now().UTC()
	if req.ObservedAt.IsZero() {
		req.ObservedAt = now
	}
	item := &MemoryItem{ID: uuid.NewString(), UserID: req.UserID, AgentID: req.AgentID, Type: req.Type, Status: req.Status, Summary: req.Summary, Content: req.Content, Confidence: req.Confidence, Sensitivity: req.Sensitivity, Importance: req.Importance, Revision: 1, OccurredAt: req.OccurredAt, OccurredEndAt: req.OccurredEndAt, OccurredTimezone: req.OccurredTimezone, LocalDate: req.LocalDate, DatePrecision: req.DatePrecision, ObservedAt: req.ObservedAt, RecordedAt: now, ValidFrom: req.ValidFrom, ValidTo: req.ValidTo, ExpiresAt: req.ExpiresAt, SupersedesItemID: req.SupersedesItemID}
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(req.Summary+"\x00"+req.Content)))
	version := &MemoryItemVersion{ID: uuid.NewString(), UserID: req.UserID, ItemID: item.ID, Revision: 1, Summary: req.Summary, Content: req.Content, ContentHash: hash, CreatedAt: now}
	evidence := &MemoryEvidence{ID: uuid.NewString(), UserID: req.UserID, ItemID: item.ID, SourceKind: req.SourceKind, ConversationID: req.ConversationID, MessageID: req.MessageID, RunID: req.RunID, AgentID: req.SourceAgentID, AgentVersionID: req.AgentVersionID, ActorUserID: req.ActorUserID, ExtractionInvocationID: req.ExtractionInvocationID, Citation: strings.TrimSpace(req.Citation), ContentHash: hash, SourceRevision: 1, CreatedAt: now}
	if err := s.repo.Create(ctx, s.db, item, version, evidence); err != nil {
		return nil, err
	}
	return item, nil
}

func ValidateMemoryCreateRequest(req MemoryCreateRequest) error {
	req.UserID, req.Summary, req.Content = strings.TrimSpace(req.UserID), strings.TrimSpace(req.Summary), strings.TrimSpace(req.Content)
	if req.UserID == "" || req.Summary == "" || req.Content == "" || !aimemory.ValidItemType(req.Type) || !aimemory.ValidStatus(req.Status) || !aimemory.ValidSource(req.SourceKind) || !aimemory.ValidDatePrecision(req.DatePrecision) || req.Confidence < 0 || req.Confidence > 1 || req.Importance < 0 || req.Importance > 100 {
		return aimemory.ErrInvalidMemory
	}
	if req.Status == aimemory.StatusActive && req.SourceKind != aimemory.SourceUserStatement {
		return aimemory.ErrUnsafeSource
	}
	if req.OccurredAt != nil && req.OccurredEndAt != nil && req.OccurredEndAt.Before(*req.OccurredAt) {
		return aimemory.ErrInvalidMemory
	}
	if req.ValidFrom != nil && req.ValidTo != nil && !req.ValidTo.After(*req.ValidFrom) {
		return aimemory.ErrInvalidMemory
	}
	return nil
}
func (s *MemoryService) Current(ctx context.Context, userID string, now time.Time) ([]MemoryItem, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, aimemory.ErrInvalidMemory
	}
	return s.repo.Current(ctx, s.db, userID, now)
}
