import { platformPage as page } from './page';

export const operationsPages = [
  page('operations_audit-logs', () => import('@/views/operations/audit-logs/index.vue')),
  page('operations_jobs', () => import('@/views/operations/jobs/index.vue')),
  page('operations_logs', () => import('@/views/operations/logs/index.vue')),
  page('operations_notification-deliveries', () => import('@/views/operations/notification-deliveries/index.vue')),
  page('operations_outbox', () => import('@/views/operations/outbox/index.vue'))
] as const;
