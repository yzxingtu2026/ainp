import type { RouteMeta } from 'vue-router';
import ElegantVueRouter from '@elegant-router/vue/vite';
import type { RouteKey } from '@elegant-router/types';

export function setupElegantRouter() {
  return ElegantVueRouter({
    layouts: {
      base: 'src/layouts/base-layout/index.vue',
      blank: 'src/layouts/blank-layout/index.vue'
    },
    routePathTransformer(routeName, routePath) {
      const key = routeName as RouteKey;

      if (key === 'login') {
        const modules: UnionKey.LoginModule[] = ['pwd-login', 'code-login', 'register', 'reset-pwd', 'bind-wechat'];

        const moduleReg = modules.join('|');

        return `/login/:module(${moduleReg})?`;
      }

      return routePath;
    },
    onRouteMetaGen(routeName) {
      const key = routeName as RouteKey;

      const constantRoutes: RouteKey[] = ['login', 'setup', '403', '404', '500'];
      // 业务页由后端 Metadata 注入菜单；扫目录生成的静态路由不进侧栏，避免 route.xxx 与中文菜单重复
      const backendDrivenMenus: string[] = [
        'demo',
        'demo_hello',
        'demo_attachments',
        'system',
        'system_profile',
        'system_attachments',
        'notification',
        'notification_list'
      ];

      const meta: Partial<RouteMeta> = {
        title: key,
        i18nKey: `route.${key}` as App.I18n.I18nKey
      };

      if (constantRoutes.includes(key)) {
        meta.constant = true;
      }

      if (backendDrivenMenus.includes(key)) {
        meta.hideInMenu = true;
      }

      return meta;
    }
  });
}
