import type { RouteComponent } from 'vue-router';
import BaseLayout from '@/layouts/base-layout/index.vue';
import BlankLayout from '@/layouts/blank-layout/index.vue';

export const platformLayouts: Readonly<Record<string, RouteComponent | (() => Promise<RouteComponent>)>> = {
  base: BaseLayout,
  blank: BlankLayout
};
