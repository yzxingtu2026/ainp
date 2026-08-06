import type { ElegantConstRoute } from '@elegant-router/types';
import { assertRegisteredPage, getAdminApplication } from '@/composition';

/** 将后端 component 路径转为 elegant view key，如 view.notification_list → notification_list，demo/hello → demo_hello */
export function toViewKey(component?: string, id?: string): string {
  const source = (component || id || '').trim();
  if (!source || source.includes('://') || source.includes('..') || source.startsWith('//')) {
    throw new Error(`[admin-composition] invalid backend component "${source}"`);
  }
  const raw = source
    .replace(/^\//, '')
    .replace(/^view\./, '')
    .replace(/\//g, '_')
    .replace(/\./g, '_');
  return raw;
}

/**
 * Elegant Router 约定：一级路由 name 不能含 `_`，否则 children 会被拆成顶层路由。
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

/** 后端 i18n 缺失时 Title 会回退成 title_key；此时改走前端 $t(title_key)。 */
function menuTitleMeta(item: Api.Route.AinPMenu): { title: string; i18nKey: string | null } {
  const titleKey = item.title_key?.trim() || '';
  const title = item.title?.trim() || titleKey || item.id;
  if (titleKey && title === titleKey) {
    return { title, i18nKey: titleKey };
  }
  return { title, i18nKey: null };
}

/**
 * 将 AINP 扁平菜单（directory/menu）转为 ElegantConstRoute 树。
 * button 不进路由，仅用于权限码。
 * 默认不设置 i18nKey，侧栏直接使用后端已本地化 title；仅当 title 回退为 title_key 时启用前端键。
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
        const titles = menuTitleMeta(item);

        return {
          name,
          path,
          component: 'layout.base',
          meta: {
            title: titles.title,
            i18nKey: titles.i18nKey as App.I18n.I18nKey | null,
            icon: item.icon || import.meta.env.VITE_MENU_ICON,
            order: item.order,
            hideInMenu: Boolean(item.hide_in_menu),
            shellPlacement: item.placement || null,
            immersive: Boolean(item.immersive),
            backendMenuId: item.id
          },
          children: childRoutes
        } as ElegantConstRoute;
      }

      // menu leaf
      const viewKey = toViewKey(item.component, item.id);
      assertRegisteredPage(getAdminApplication(), viewKey, `backend menu "${item.id}"`);
      const path = item.path || `/${viewKey.replace(/_/g, '/')}`;
      const firstLevel = parentFirstLevel || toFirstLevelName(item.id, path);
      const name = viewKey.includes('_') ? viewKey : `${firstLevel}_${viewKey}`;
      const titles = menuTitleMeta(item);

      if (childRoutes.length === 0 && !item.parent_id) {
        // 无父级的单级菜单：layout.base$view.xxx，name 仍须无 `_` 才是 single-level
        // 有下划线时改为一级 layout + 子 view
        if (!name.includes('_')) {
          return {
            name,
            path,
            component: `layout.base$view.${viewKey}`,
            meta: {
              title: titles.title,
              i18nKey: titles.i18nKey as App.I18n.I18nKey | null,
              icon: item.icon || import.meta.env.VITE_MENU_ICON,
              order: item.order,
              hideInMenu: Boolean(item.hide_in_menu),
              keepAlive: Boolean(item.keep_alive),
              shellPlacement: item.placement || null,
              immersive: Boolean(item.immersive),
              backendMenuId: item.id
            }
          } as ElegantConstRoute;
        }

        return {
          name: firstLevel,
          path: `/${firstLevel}`,
          component: 'layout.base',
          meta: {
            title: titles.title,
            i18nKey: titles.i18nKey as App.I18n.I18nKey | null,
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
                title: titles.title,
                i18nKey: titles.i18nKey as App.I18n.I18nKey | null,
                icon: item.icon || import.meta.env.VITE_MENU_ICON,
                order: item.order,
                hideInMenu: Boolean(item.hide_in_menu),
                keepAlive: Boolean(item.keep_alive),
                shellPlacement: item.placement || null,
                immersive: Boolean(item.immersive),
                backendMenuId: item.id
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
          title: titles.title,
          i18nKey: titles.i18nKey as App.I18n.I18nKey | null,
          icon: item.icon || import.meta.env.VITE_MENU_ICON,
          order: item.order,
          hideInMenu: Boolean(item.hide_in_menu),
          keepAlive: Boolean(item.keep_alive),
          shellPlacement: item.placement || null,
          immersive: Boolean(item.immersive),
          backendMenuId: item.id
        }
      } as ElegantConstRoute;
    });
  }

  return build('');
}

/** 按钮 permission 列表（对齐 action.id） */
export function extractButtonPermissions(buttons: Api.Route.AinPMenu[]): string[] {
  const codes = buttons.map(b => b.permission || b.bind_action || '').filter(Boolean);
  return Array.from(new Set(codes));
}
