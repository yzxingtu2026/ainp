package operations

import operationsruntime "github.com/yzxingtu2026/ainp/platform/modules/operations/center"

// NotificationDeliveryService remains at the module boundary for boot-time injection.
type NotificationDeliveryService = operationsruntime.NotificationDeliveryService

type (
	AuditLogItem    = operationsruntime.AuditLogItem
	AuditLogService = operationsruntime.AuditLogService
	JobService      = operationsruntime.JobService
	LogService      = operationsruntime.LogService
	OutboxService   = operationsruntime.OutboxService
)
