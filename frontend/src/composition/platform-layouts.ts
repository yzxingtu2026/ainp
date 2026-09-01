import type { RouteComponent } from 'vue-router';

export const platformLayouts: Readonly<Record<string, RouteComponent | (() => Promise<RouteComponent>)>> = {
  base: async () => (await import('@/layouts/base-layout/index.vue')).default,
  blank: async () => (await import('@/layouts/blank-layout/index.vue')).default
};
