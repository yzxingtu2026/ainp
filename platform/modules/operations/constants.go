package operations

import operationsruntime "github.com/yzxingtu2026/ainp/platform/modules/operations/center"

const (
	ActionNotificationDeliveryList  = operationsruntime.ActionNotificationDeliveryList
	ActionNotificationDeliveryGet   = operationsruntime.ActionNotificationDeliveryGet
	ActionNotificationDeliveryRetry = operationsruntime.ActionNotificationDeliveryRetry
	ActionAuditLogList              = operationsruntime.ActionAuditLogList
	ActionAuditLogGet               = operationsruntime.ActionAuditLogGet
	ActionJobList                   = operationsruntime.ActionJobList
	ActionJobGet                    = operationsruntime.ActionJobGet
	ActionJobCancel                 = operationsruntime.ActionJobCancel
	ActionJobRetry                  = operationsruntime.ActionJobRetry
	ActionOutboxList                = operationsruntime.ActionOutboxList
	ActionOutboxSummary             = operationsruntime.ActionOutboxSummary
	ActionOutboxListDeliveries      = operationsruntime.ActionOutboxListDeliveries
	ActionOutboxRetry               = operationsruntime.ActionOutboxRetry
	PageTypeAuditLogs               = operationsruntime.PageTypeAuditLogs
)
