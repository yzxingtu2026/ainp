package idempotency

import (
	"fmt"

	"gorm.io/gorm"
)

// AutoMigrate creates the idempotency_records table and enforces constraints.
func AutoMigrate(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("idempotency: db is required")
	}
	if err := db.AutoMigrate(&Record{}); err != nil {
		return err
	}
	return ensureConstraints(db)
}

func ensureConstraints(db *gorm.DB) error {
	var invalid int64
	if err := db.Model(&Record{}).
		Where("status NOT IN ?", []Status{StatusProcessing, StatusCompleted, StatusFailed, StatusConflict}).
		Count(&invalid).Error; err != nil {
		return err
	}
	if invalid > 0 {
		return fmt.Errorf("idempotency_records contains %d invalid status values", invalid)
	}
	return db.Exec(`ALTER TABLE idempotency_records DROP CONSTRAINT IF EXISTS ck_idempotency_records_status;
ALTER TABLE idempotency_records ADD CONSTRAINT ck_idempotency_records_status CHECK (status IN ('processing','completed','failed','conflict'))`).Error
}
