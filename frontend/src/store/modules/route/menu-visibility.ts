interface MenuRouteProjection {
  meta?: { hideInMenu?: boolean | null } | null;
  children?: MenuRouteProjection[];
}

/** A parent route belongs in navigation only when at least one descendant is visible. */
export function hasVisibleMenuProjection(route: MenuRouteProjection): boolean {
  if (route.meta?.hideInMenu) return false;
  if (!route.children) return true;

  return route.children.some(hasVisibleMenuProjection);
}
