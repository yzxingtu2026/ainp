package outbox

import (
	"fmt"

	"github.com/yzxingtu2026/ainp/platform/framework/status"
	"gorm.io/gorm"
)

// AutoMigrate creates outbox_events and event_deliveries tables.
func AutoMigrate(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("outbox: db is required")
	}
	if err := db.AutoMigrate(&Event{}, &Delivery{}); err != nil {
		return err
	}
	return ensureConstraints(db)
}

func ensureConstraints(db *gorm.DB) error {
	var invalid int64
	if err := db.Model(&Event{}).Where("status NOT IN ?", []status.OutboxStatus{
		status.OutboxPending, status.OutboxProcessing, status.OutboxPublished, status.OutboxFailed, status.OutboxDead,
	}).Count(&invalid).Error; err != nil {
		return err
	}
	if invalid > 0 {
		return fmt.Errorf("outbox_events contains %d invalid status values", invalid)
	}
	if err := db.Exec(`ALTER TABLE outbox_events DROP CONSTRAINT IF EXISTS ck_outbox_events_status;
ALTER TABLE outbox_events ADD CONSTRAINT ck_outbox_events_status CHECK (status IN ('pending','processing','published','failed','dead'))`).Error; err != nil {
		return err
	}
	invalid = 0
	if err := db.Model(&Delivery{}).Where("status NOT IN ?", []status.DeliveryStatus{
		status.DeliveryPending, status.DeliveryProcessing, status.DeliverySucceeded, status.DeliveryFailed,
	}).Count(&invalid).Error; err != nil {
		return err
	}
	if invalid > 0 {
		return fmt.Errorf("event_deliveries contains %d invalid status values", invalid)
	}
	return db.Exec(`ALTER TABLE event_deliveries DROP CONSTRAINT IF EXISTS ck_event_deliveries_status;
ALTER TABLE event_deliveries ADD CONSTRAINT ck_event_deliveries_status CHECK (status IN ('pending','processing','succeeded','failed'))`).Error
}
