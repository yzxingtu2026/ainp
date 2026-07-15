package outbox

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/yzxingtu2026/ainp/platform/framework/status"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// DeliveryStore tracks per (event_id, handler_name) processing for at-least-once idempotency.
type DeliveryStore interface {
	// RunHandler executes h if not already succeeded; records success/failure atomically with handler DB work when possible.
	RunHandler(ctx context.Context, env Envelope, h Handler) error
	ListByEvent(ctx context.Context, eventID string) ([]Delivery, error)
}

type gormDeliveryStore struct {
	db *gorm.DB
}

func NewDeliveryStore(db *gorm.DB) DeliveryStore {
	return &gormDeliveryStore{db: db}
}

func (s *gormDeliveryStore) ListByEvent(ctx context.Context, eventID string) ([]Delivery, error) {
	var rows []Delivery
	err := s.db.WithContext(ctx).Where("event_id = ?", eventID).Order("handler_name").Find(&rows).Error
	return rows, err
}

func (s *gormDeliveryStore) RunHandler(ctx context.Context, env Envelope, h Handler) error {
	if h == nil {
		return fmt.Errorf("outbox: handler is nil")
	}
	now := time.Now().UTC()
	var existing Delivery
	err := s.db.WithContext(ctx).Where("event_id = ? AND handler_name = ?", env.ID, h.Name()).First(&existing).Error
	if err == nil && existing.Status == status.DeliverySucceeded {
		return nil
	}
	if err != nil && err != gorm.ErrRecordNotFound {
		return err
	}

	if err == gorm.ErrRecordNotFound {
		existing = Delivery{
			ID:          uuid.NewString(),
			EventID:     env.ID,
			HandlerName: h.Name(),
			Status:      status.DeliveryProcessing,
			Attempts:    1,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if createErr := s.db.WithContext(ctx).Create(&existing).Error; createErr != nil {
			// concurrent insert: re-check success
			var again Delivery
			if s.db.WithContext(ctx).Where("event_id = ? AND handler_name = ?", env.ID, h.Name()).First(&again).Error == nil {
				if again.Status == status.DeliverySucceeded {
					return nil
				}
				existing = again
				existing.Attempts++
				existing.Status = status.DeliveryProcessing
				existing.UpdatedAt = now
				if err := s.db.WithContext(ctx).Save(&existing).Error; err != nil {
					return err
				}
			} else {
				return createErr
			}
		}
	} else {
		existing.Attempts++
		existing.Status = status.DeliveryProcessing
		existing.UpdatedAt = now
		existing.LastError = ""
		if err := s.db.WithContext(ctx).Save(&existing).Error; err != nil {
			return err
		}
	}

	handleErr := h.Handle(ctx, env)
	processed := time.Now().UTC()
	if handleErr != nil {
		_ = s.db.WithContext(ctx).Model(&Delivery{}).Where("id = ?", existing.ID).Updates(map[string]any{
			"status":     status.DeliveryFailed,
			"last_error": truncateError(handleErr),
			"updated_at": processed,
		}).Error
		return handleErr
	}
	return s.db.WithContext(ctx).Model(&Delivery{}).Where("id = ?", existing.ID).Updates(map[string]any{
		"status":       status.DeliverySucceeded,
		"processed_at": processed,
		"last_error":   "",
		"updated_at":   processed,
	}).Error
}

// MarkSucceededInTx records delivery success inside the caller's transaction
// (for handlers that write business data in the same tx as the receipt).
func MarkSucceededInTx(tx *gorm.DB, eventID, handlerName string) error {
	now := time.Now().UTC()
	row := Delivery{
		ID:          uuid.NewString(),
		EventID:     eventID,
		HandlerName: handlerName,
		Status:      status.DeliverySucceeded,
		Attempts:    1,
		ProcessedAt: &now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	return tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "event_id"}, {Name: "handler_name"}},
		DoUpdates: clause.AssignmentColumns([]string{"status", "processed_at", "last_error", "updated_at", "attempts"}),
	}).Create(&row).Error
}
