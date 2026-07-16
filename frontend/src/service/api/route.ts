import { request } from '../request';
import { transformAinPMenusToRoutes } from '@/utils/ainp-menu';
import { createStaticRoutes } from '@/router/routes';

/** 本地始终保留的路由：home 为欢迎页，notification 为铃铛入口（不依赖后端菜单） */
const LOCAL_AUTH_ROUTE_NAMES = new Set(['home', 'notification']);

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
    const localRoutes = authRoutes.filter(r => LOCAL_AUTH_ROUTE_NAMES.has(String(r.name)));

    const routes = [...localRoutes, ...dynamicRoutes] as Api.Route.MenuRoute[];

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
