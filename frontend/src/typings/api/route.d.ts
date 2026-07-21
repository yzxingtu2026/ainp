declare namespace Api {
  namespace Route {
    type ElegantConstRoute = import('@elegant-router/types').ElegantConstRoute;

    interface MenuRoute extends ElegantConstRoute {
      id?: string;
    }

    interface UserRoute {
      routes: MenuRoute[];
      home: import('@elegant-router/types').LastLevelRouteKey;
    }

    /** AINP GET /api/v1/menus */
    interface AinPMenu {
      id: string;
      parent_id?: string;
      title: string;
      title_key?: string;
      icon?: string;
      local_icon?: string;
      order: number;
      type: 'directory' | 'menu' | 'button';
      path?: string;
      component?: string;
      permission?: string;
      bind_action?: string;
      hide_in_menu?: boolean;
      keep_alive?: boolean;
      placement?: 'global_ai_launcher';
      immersive?: boolean;
      module?: string;
    }

    interface AinPMenusData {
      items: AinPMenu[];
      buttons: AinPMenu[];
    }

    interface NavigationSearchResult {
      menu_id: string;
      title: string;
      description?: string;
      breadcrumb: string[];
      path: string;
      icon?: string;
      local_icon?: string;
      match_type: 'title' | 'breadcrumb' | 'keyword' | 'description' | 'use_case';
      match_summary: string;
    }

    interface NavigationSearchData {
      items: NavigationSearchResult[];
      degraded: boolean;
    }
  }
}
