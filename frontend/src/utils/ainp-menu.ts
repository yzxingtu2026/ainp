import type { ElegantConstRoute } from '@elegant-router/types';

/** 将后端 component 路径转为 elegant view key，如 demo/hello -> demo_hello */
export function toViewKey(component?: string, id?: string): string {
  const raw = (component || id || 'unknown').replace(/^\//, '').replace(/\//g, '_').replace(/\./g, '_');
  return raw;
}

/** 将菜单 id 转为路由 name */
export function toRouteName(id: string): string {
  return id.replace(/\./g, '_');
}

/**
 * 将 AINP 扁平菜单（directory/menu）转为 Soybean ElegantConstRoute 树。
 * button 不进路由，仅用于权限码。
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

  function build(parentId: string): ElegantConstRoute[] {
    const children = byParent.get(parentId) || [];
    return children.map(item => {
      const name = toRouteName(item.id);
      const childRoutes = build(item.id);

      if (item.type === 'directory') {
        const firstChildPath = childRoutes[0]?.path;
        const path =
          item.path ||
          (typeof firstChildPath === 'string' && firstChildPath.includes('/')
            ? `/${firstChildPath.split('/').filter(Boolean)[0]}`
            : `/${name}`);

        return {
          name,
          path,
          component: 'layout.base',
          meta: {
            title: item.title,
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

      if (childRoutes.length === 0 && !item.parent_id) {
        return {
          name,
          path,
          component: `layout.base$view.${viewKey}`,
          meta: {
            title: item.title,
            icon: item.icon || import.meta.env.VITE_MENU_ICON,
            order: item.order,
            hideInMenu: Boolean(item.hide_in_menu),
            keepAlive: Boolean(item.keep_alive)
          }
        } as ElegantConstRoute;
      }

      return {
        name,
        path,
        component: `view.${viewKey}`,
        meta: {
          title: item.title,
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
