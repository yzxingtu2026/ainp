import { platformPage as page } from './page';

export const demoPages = [
  page('demo_attachments', () => import('@/views/demo/attachments/index.vue')),
  page('demo_hello', () => import('@/views/demo/hello/index.vue'))
] as const;
