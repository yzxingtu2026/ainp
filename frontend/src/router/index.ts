import type { App } from 'vue';
import {
  type RouterHistory,
  createMemoryHistory,
  createRouter,
  createWebHashHistory,
  createWebHistory
} from 'vue-router';
import { createBuiltinVueRoutes } from './routes/builtin';
import { createRouterGuard } from './guard';
import { resolveWebEntry } from './web-entry';

const { VITE_ROUTER_HISTORY_MODE = 'history', VITE_BASE_URL, VITE_APP_MOUNT = '/' } = import.meta.env;
const entry = resolveWebEntry(window.location.pathname, VITE_BASE_URL, VITE_APP_MOUNT);

const historyCreatorMap: Record<Env.RouterHistoryMode, (base?: string) => RouterHistory> = {
  hash: createWebHashHistory,
  history: createWebHistory,
  memory: createMemoryHistory
};

export const router = createRouter({
  history: historyCreatorMap[entry.versioned ? 'hash' : VITE_ROUTER_HISTORY_MODE](entry.base),
  routes: createBuiltinVueRoutes()
});

/** Setup Vue Router */
export async function setupRouter(app: App) {
  app.use(router);
  createRouterGuard(router);
  await router.isReady();
}
