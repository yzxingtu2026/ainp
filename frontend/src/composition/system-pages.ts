import { platformPage as page } from './page';

export const systemPages = [
  page('system_accounts', () => import('@/views/system/accounts/index.vue')),
  page('system_attachments', () => import('@/views/system/attachments/index.vue')),
  page('system_notification-templates', () => import('@/views/system/notification-templates/index.vue')),
  page('system_profile', () => import('@/views/system/profile/index.vue')),
  page('system_roles', () => import('@/views/system/roles/index.vue')),
  page('system_settings', () => import('@/views/system/settings/index.vue'))
] as const;
