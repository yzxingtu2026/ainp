import type { AdminAppDefinition, AdminModule, OwnedRegistration, ResolvedAdminApp } from './contract';

const ID_PATTERN = /^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*$/;
const PAGE_KEY_PATTERN = /^[a-z0-9]+(?:[_-][a-z0-9]+)*$/;
const SAFE_PATH_PATTERN = /^\/(?:[a-z0-9][a-z0-9._~-]*)(?:\/[a-z0-9][a-z0-9._~-]*)*$/;

const MODULE_DEPENDENCIES: Readonly<Record<AdminModule, readonly AdminModule[]>> = {
  core: [],
  system: ['core'],
  operations: ['core', 'system'],
  ai: ['core'],
  demo: ['core']
};

function assertId(kind: string, value: string) {
  if (!ID_PATTERN.test(value)) throw new Error(`[admin-composition] invalid ${kind} "${value}"`);
}

function assertUnique(kind: string, registrations: readonly OwnedRegistration[], idPattern = ID_PATTERN) {
  const owners = new Map<string, string>();
  for (const item of registrations) {
    if (!idPattern.test(item.id)) throw new Error(`[admin-composition] invalid ${kind} id "${item.id}"`);
    assertId(`${kind} owner`, item.owner);
    const previous = owners.get(item.id);
    if (previous) {
      throw new Error(`[admin-composition] duplicate ${kind} "${item.id}" from owners "${previous}" and "${item.owner}"`);
    }
    owners.set(item.id, item.owner);
  }
}

export function defineAdminApp(definition: AdminAppDefinition): ResolvedAdminApp {
  assertId('application id', definition.id);
  if (!definition.name.trim()) throw new Error('[admin-composition] application name is required');
  if (!definition.brand.logo.startsWith('/') || !definition.brand.favicon.startsWith('/')) {
    throw new Error('[admin-composition] brand assets must be build-local absolute paths');
  }

  const modules = [...new Set(definition.modules)].sort();
  const enabled = new Set(modules);
  if (!enabled.has('core')) throw new Error('[admin-composition] module "core" is required');
  for (const module of modules) {
    for (const dependency of MODULE_DEPENDENCIES[module]) {
      if (!enabled.has(dependency)) {
        throw new Error(`[admin-composition] module "${module}" requires "${dependency}"`);
      }
    }
  }

  const pages = [...definition.pages].sort((a, b) => a.id.localeCompare(b.id));
  const routes = [...(definition.routes || [])].sort((a, b) => a.id.localeCompare(b.id));
  const i18n = [...(definition.i18n || [])].sort((a, b) => a.id.localeCompare(b.id));
  const initializers = [...(definition.initializers || [])].sort((a, b) => a.id.localeCompare(b.id));
  const services = [...(definition.services || [])].sort((a, b) => a.id.localeCompare(b.id));

  assertUnique('page', pages, PAGE_KEY_PATTERN);
  assertUnique('route', routes);
  assertUnique('i18n namespace', i18n);
  assertUnique('initializer', initializers);
  assertUnique('service', services);

  const pageMap = Object.fromEntries(pages.map(page => [page.id, page.component]));
  for (const route of routes) {
    if (!SAFE_PATH_PATTERN.test(route.path) || route.path.includes('..')) {
      throw new Error(`[admin-composition] invalid route path "${route.path}" from owner "${route.owner}"`);
    }
    if (!pageMap[route.pageKey]) {
      throw new Error(`[admin-composition] route "${route.id}" references unknown page "${route.pageKey}"`);
    }
  }

  return Object.freeze({
    ...definition,
    modules: Object.freeze(modules),
    pages: Object.freeze(pages),
    routes: Object.freeze(routes),
    i18n: Object.freeze(i18n),
    initializers: Object.freeze(initializers),
    services: Object.freeze(services),
    pageMap: Object.freeze(pageMap),
    servicesMap: new Map(services.map(item => [item.id, item.service]))
  });
}

export function assertRegisteredPage(app: ResolvedAdminApp, pageKey: string, source: string) {
  if (!PAGE_KEY_PATTERN.test(pageKey) || !app.pageMap[pageKey]) {
    throw new Error(`[admin-composition] ${source} references unknown page "${pageKey}"`);
  }
}

interface ComponentRoute {
  name: string;
  component?: string;
  children?: ComponentRoute[];
}

export function assertRegisteredRouteComponents(
  routes: readonly ComponentRoute[],
  pages: Readonly<Record<string, unknown>>,
  layouts: Readonly<Record<string, unknown>>
) {
  for (const route of routes) {
    for (const component of (route.component || '').split('$')) {
      if (component.startsWith('view.')) {
        const pageKey = component.slice('view.'.length);
        if (!pages[pageKey]) throw new Error(`[admin-composition] route "${route.name}" references unknown page "${pageKey}"`);
      } else if (component.startsWith('layout.')) {
        const layoutKey = component.slice('layout.'.length);
        if (!layouts[layoutKey]) {
          throw new Error(`[admin-composition] route "${route.name}" references unknown layout "${layoutKey}"`);
        }
      } else if (component) {
        throw new Error(`[admin-composition] route "${route.name}" uses unsafe component "${component}"`);
      }
    }
    assertRegisteredRouteComponents(route.children || [], pages, layouts);
  }
}
