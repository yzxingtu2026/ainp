package operations

import (
	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/job"
	"github.com/yzxingtu2026/ainp/platform/framework/logcenter"
	operationsruntime "github.com/yzxingtu2026/ainp/platform/modules/operations/center"
	"gorm.io/gorm"
)

func NewAuditLogService(db *gorm.DB, actions ...*action.Registry) *AuditLogService {
	return operationsruntime.NewAuditLogService(db, actions...)
}

func NewJobService(rt *job.Runtime) *JobService {
	return operationsruntime.NewJobService(rt)
}

func NewLogService(providers *logcenter.Registry) *LogService {
	return operationsruntime.NewLogService(providers)
}

func NewOutboxService(db *gorm.DB) *OutboxService {
	return operationsruntime.NewOutboxService(db)
}
