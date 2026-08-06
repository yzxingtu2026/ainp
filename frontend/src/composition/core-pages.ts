import { platformPage as page } from './page';

export const corePages = [
  page('403', () => import('@/views/_builtin/403/index.vue')),
  page('404', () => import('@/views/_builtin/404/index.vue')),
  page('500', () => import('@/views/_builtin/500/index.vue')),
  page('iframe-page', () => import('@/views/_builtin/iframe-page/[url].vue')),
  page('login', () => import('@/views/_builtin/login/index.vue')),
  page('setup', () => import('@/views/_builtin/setup/index.vue')),
  page('home', () => import('@/views/home/index.vue')),
  page('mcp_confirm', () => import('@/views/mcp/confirm/index.vue')),
  page('notification_list', () => import('@/views/notification/list/index.vue')),
  page('oauth_authorize', () => import('@/views/oauth/authorize/index.vue')),
  page('profile_memory', () => import('@/views/profile/memory/index.vue'))
] as const;
