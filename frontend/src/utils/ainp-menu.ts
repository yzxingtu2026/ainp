import type { ElegantConstRoute } from '@elegant-router/types';

/** 将后端 component 路径转为 elegant view key，如 view.notification_list → notification_list，demo/hello → demo_hello */
export function toViewKey(component?: string, id?: string): string {
  const raw = (component || id || 'unknown').replace(/^\//, '').replace(/^view\./, '').replace(/\//g, '_').replace(/\./g, '_');
  return raw;
}

/**
 * Soybean 约定：一级路由 name 不能含 `_`，否则 children 会被拆成顶层路由。
 * directory 用能力域/路径首段，如 demo.root → demo，/system/profile → system
 */
export function toFirstLevelName(id: string, path?: string): string {
  if (path) {
    const seg = path.split('/').filter(Boolean)[0];
    if (seg && !seg.includes('_')) {
      return seg;
    }
  }
  const head = id.split('.')[0] || id;
  return head.replace(/_/g, '');
}

/**
 * 将 AINP 扁平菜单（directory/menu）转为 Soybean ElegantConstRoute 树。
 * button 不进路由，仅用于权限码。
 * 不设置 i18nKey，侧栏直接使用后端 title，避免出现 route.xxx 键名。
 */
export function transformAinPMenusToRoutes(items: Api.Route.AinPMenu[]): ElegantConstRoute[] {
  const nodes = items.filter(m => m.type === 'directory' || m.type === 'menu');
  const byParent = new Map<string, Api.Route.AinPMenu[]>();

  nodes.forEach(m => {
    const pid = m.parent_id || '';
    const list = byParent.get(pid) || [];
    list.push(m);
    byParent.set(pid, list);
  });

  byParent.forEach(list => list.sort((a, b) => a.order - b.order || a.id.localeCompare(b.id)));

  function build(parentId: string, parentFirstLevel?: string): ElegantConstRoute[] {
    const children = byParent.get(parentId) || [];
    return children.map(item => {
      const childRoutes = build(item.id, parentFirstLevel || toFirstLevelName(item.id, item.path));

      if (item.type === 'directory') {
        const firstChildPath = childRoutes[0]?.path;
        const path =
          item.path ||
          (typeof firstChildPath === 'string' && firstChildPath.includes('/')
            ? `/${firstChildPath.split('/').filter(Boolean)[0]}`
            : `/${toFirstLevelName(item.id)}`);
        const name = toFirstLevelName(item.id, path);

        return {
          name,
          path,
          component: 'layout.base',
          meta: {
            title: item.title,
            i18nKey: null,
            icon: item.icon || import.meta.env.VITE_MENU_ICON,
            order: item.order,
            hideInMenu: Boolean(item.hide_in_menu)
          },
          children: childRoutes
        } as ElegantConstRoute;
      }

      // menu leaf
      const viewKey = toViewKey(item.component, item.id);
      const path = item.path || `/${viewKey.replace(/_/g, '/')}`;
      const firstLevel = parentFirstLevel || toFirstLevelName(item.id, path);
      const name = viewKey.includes('_') ? viewKey : `${firstLevel}_${viewKey}`;

      if (childRoutes.length === 0 && !item.parent_id) {
        // 无父级的单级菜单：layout.base$view.xxx，name 仍须无 `_` 才是 single-level
        // 有下划线时改为一级 layout + 子 view
        if (!name.includes('_')) {
          return {
            name,
            path,
            component: `layout.base$view.${viewKey}`,
            meta: {
              title: item.title,
              i18nKey: null,
              icon: item.icon || import.meta.env.VITE_MENU_ICON,
              order: item.order,
              hideInMenu: Boolean(item.hide_in_menu),
              keepAlive: Boolean(item.keep_alive)
            }
          } as ElegantConstRoute;
        }

        return {
          name: firstLevel,
          path: `/${firstLevel}`,
          component: 'layout.base',
          meta: {
            title: item.title,
            i18nKey: null,
            icon: item.icon || import.meta.env.VITE_MENU_ICON,
            order: item.order,
            hideInMenu: Boolean(item.hide_in_menu)
          },
          children: [
            {
              name,
              path,
              component: `view.${viewKey}`,
              meta: {
                title: item.title,
                i18nKey: null,
                icon: item.icon || import.meta.env.VITE_MENU_ICON,
                order: item.order,
                hideInMenu: Boolean(item.hide_in_menu),
                keepAlive: Boolean(item.keep_alive)
              }
            } as ElegantConstRoute
          ]
        } as ElegantConstRoute;
      }

      return {
        name,
        path,
        component: `view.${viewKey}`,
        meta: {
          title: item.title,
          i18nKey: null,
          icon: item.icon || import.meta.env.VITE_MENU_ICON,
          order: item.order,
          hideInMenu: Boolean(item.hide_in_menu),
          keepAlive: Boolean(item.keep_alive)
        }
      } as ElegantConstRoute;
    });
  }

  return build('');
}

/** 按钮 permission 列表（对齐 action.id） */
export function extractButtonPermissions(buttons: Api.Route.AinPMenu[]): string[] {
  const codes = buttons
    .map(b => b.permission || b.bind_action || '')
    .filter(Boolean);
  return Array.from(new Set(codes));
}
