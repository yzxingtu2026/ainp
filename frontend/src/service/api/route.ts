import type { ElegantConstRoute } from '@elegant-router/types';
import { request } from '../request';
import { transformAinPMenusToRoutes } from '@/utils/ainp-menu';
import { createStaticRoutes } from '@/router/routes';

/** 本地始终保留的一级路由：home 为欢迎页，notification 为铃铛入口（不依赖后端菜单） */
const LOCAL_AUTH_ROUTE_NAMES = new Set(['home', 'notification']);

/**
 * 本地隐藏子路由：无后端菜单也会注入，供列表内跳转（如 Agent Builder）。
 * 必须挂在已有一级路由下（name 含 `_` 的叶子不能单独做一级路由）。
 */
const LOCAL_HIDDEN_CHILD_ROUTE_NAMES = new Set(['ai_agent-builder', 'profile_memory']);

function cloneRoute(route: ElegantConstRoute): ElegantConstRoute {
  return {
    ...route,
    meta: route.meta ? { ...route.meta } : route.meta,
    children: route.children?.map(cloneRoute)
  };
}

/** 提取需要本地常驻的一级路由，以及仅含隐藏子路由的父级壳 */
function collectLocalAuthRoutes(authRoutes: ElegantConstRoute[]): ElegantConstRoute[] {
  const out: ElegantConstRoute[] = [];
  for (const route of authRoutes) {
    if (LOCAL_AUTH_ROUTE_NAMES.has(String(route.name))) {
      out.push(cloneRoute(route));
      continue;
    }
    const hiddenChildren = (route.children || []).filter(child =>
      LOCAL_HIDDEN_CHILD_ROUTE_NAMES.has(String(child.name))
    );
    if (hiddenChildren.length === 0) continue;
    out.push({
      ...cloneRoute(route),
      meta: { ...route.meta, title: route.meta?.title || String(route.name), hideInMenu: true },
      children: hiddenChildren.map(cloneRoute)
    });
  }
  return out;
}

/** 按 name 合并一级路由，隐藏子路由并入已有后端菜单树，避免重复 ai 父级 */
function mergeAuthRoutes(localRoutes: ElegantConstRoute[], dynamicRoutes: ElegantConstRoute[]): ElegantConstRoute[] {
  const merged = new Map<string, ElegantConstRoute>();
  for (const route of dynamicRoutes) {
    merged.set(String(route.name), cloneRoute(route));
  }
  for (const local of localRoutes) {
    const key = String(local.name);
    const existing = merged.get(key);
    if (!existing) {
      merged.set(key, cloneRoute(local));
      continue;
    }
    const children = [...(existing.children || [])];
    for (const child of local.children || []) {
      const idx = children.findIndex(item => String(item.name) === String(child.name));
      if (idx >= 0) {
        // 本地静态路由带前端 i18nKey；覆盖后端未翻译时把 title_key 原样当 title 的情况
        const current = children[idx];
        children[idx] = {
          ...current,
          ...cloneRoute(child),
          meta: {
            ...current.meta,
            ...child.meta,
            title: child.meta?.title || current.meta?.title || String(child.name),
            icon: current.meta?.icon || child.meta?.icon
          }
        };
      } else {
        children.push(cloneRoute(child));
      }
    }
    merged.set(key, { ...existing, children });
  }
  return Array.from(merged.values());
}

/** get constant routes — 使用本地内置常量路由 */
export function fetchGetConstantRoutes() {
  const { constantRoutes } = createStaticRoutes();
  return Promise.resolve({
    data: constantRoutes as Api.Route.MenuRoute[],
    error: null,
    response: null
  });
}

/** get user routes — 消费 AINP /api/v1/menus */
export function fetchGetUserRoutes() {
  return request<Api.Route.AinPMenusData>({ url: '/api/v1/menus' }).then(res => {
    if (res.error || !res.data) {
      return { data: null, error: res.error, response: res.response } as {
        data: Api.Route.UserRoute | null;
        error: typeof res.error;
        response: typeof res.response;
      };
    }

    const dynamicRoutes = transformAinPMenusToRoutes(res.data.items || []);
    const { authRoutes } = createStaticRoutes();
    const localRoutes = collectLocalAuthRoutes(authRoutes);
    const routes = mergeAuthRoutes(localRoutes, dynamicRoutes) as Api.Route.MenuRoute[];

    const data: Api.Route.UserRoute = {
      routes,
      home: 'home'
    };

    return { data, error: null, response: res.response };
  });
}

/**
 * whether the route is exist — 动态模式下以前端已注入路由为准，缺省 true 避免误拦
 */
export function fetchIsRouteExist(_routeName: string) {
  return Promise.resolve({ data: true, error: null, response: null });
}

/** Search the current authenticated Menu Registry projection. */
export function fetchNavigationSearch(query: string, limit = 5) {
  return request<Api.Route.NavigationSearchData>({
    url: '/api/v1/navigation/search',
    params: { query, limit }
  });
}
