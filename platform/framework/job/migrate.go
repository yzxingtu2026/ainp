package job

import (
	"fmt"

	"github.com/yzxingtu2026/ainp/platform/framework/status"
	"gorm.io/gorm"
)

func AutoMigrate(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("job: db is required")
	}
	if err := db.AutoMigrate(&Run{}); err != nil {
		return err
	}
	return ensureConstraints(db)
}

func ensureConstraints(db *gorm.DB) error {
	var invalid int64
	if err := db.Model(&Run{}).Where("status NOT IN ?", []status.JobRunStatus{
		status.JobPendingEnqueue, status.JobQueued, status.JobRunning, status.JobRetrying,
		status.JobSucceeded, status.JobFailed, status.JobDead, status.JobCancelled,
	}).Count(&invalid).Error; err != nil {
		return err
	}
	if invalid > 0 {
		return fmt.Errorf("job_runs contains %d invalid status values", invalid)
	}
	if err := db.Exec(`ALTER TABLE job_runs DROP CONSTRAINT IF EXISTS ck_job_runs_status;
ALTER TABLE job_runs ADD CONSTRAINT ck_job_runs_status CHECK (status IN (
  'pending_enqueue','queued','running','retrying','succeeded','failed','dead','cancelled'
))`).Error; err != nil {
		return err
	}
	return db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS ux_job_runs_idem
ON job_runs (tenant_id, job_type, idempotency_key)
WHERE idempotency_key IS NOT NULL AND idempotency_key <> ''`).Error
}
