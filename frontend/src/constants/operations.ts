export const OPERATIONS_ACTIONS = {
  jobList: 'operations.job_list',
  jobGet: 'operations.job_get',
  jobCancel: 'operations.job_cancel',
  jobRetry: 'operations.job_retry',
  outboxList: 'operations.outbox_list',
  outboxSummary: 'operations.outbox_summary',
  outboxListDeliveries: 'operations.outbox_list_deliveries',
  outboxRetry: 'operations.outbox_retry',
  notificationDeliveryList: 'operations.notification_delivery_list',
  notificationDeliveryGet: 'operations.notification_delivery_get',
  notificationDeliveryRetry: 'operations.notification_delivery_retry'
} as const;
