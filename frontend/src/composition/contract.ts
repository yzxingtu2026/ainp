import type { App as VueApp } from 'vue';
import type { RouteComponent } from 'vue-router';

export const ADMIN_MODULES = ['core', 'system', 'operations', 'ai', 'demo'] as const;
export type AdminModule = (typeof ADMIN_MODULES)[number];

export interface BrandDefinition {
  name: string;
  logo: string;
  favicon: string;
  loginVisual?: string;
  loginVisualDark?: string;
  titleSuffix?: string;
  themeColor?: string;
}

export interface OwnedRegistration {
  id: string;
  owner: string;
}

export interface PageRegistration extends OwnedRegistration {
  component: RouteComponent | (() => Promise<RouteComponent>);
}

export interface RouteRegistration extends OwnedRegistration {
  path: string;
  pageKey: string;
}

export interface I18nRegistration extends OwnedRegistration {
  messages: Record<string, unknown>;
}

export interface InitializerRegistration extends OwnedRegistration {
  setup: (context: AdminInitializerContext) => void | Promise<void>;
}

export interface ServiceRegistration<T = unknown> extends OwnedRegistration {
  service: T;
}

export interface AdminInitializerContext {
  app: VueApp;
  services: ReadonlyMap<string, unknown>;
}

export interface AdminAppDefinition {
  id: string;
  name: string;
  version: string;
  baselineVersion: string;
  brand: BrandDefinition;
  modules: readonly AdminModule[];
  pages: readonly PageRegistration[];
  routes?: readonly RouteRegistration[];
  i18n?: readonly I18nRegistration[];
  initializers?: readonly InitializerRegistration[];
  services?: readonly ServiceRegistration[];
  homePageKey?: string;
}

export interface ResolvedAdminApp
  extends Omit<AdminAppDefinition, 'modules' | 'pages' | 'routes' | 'i18n' | 'initializers' | 'services'> {
  modules: readonly AdminModule[];
  pages: readonly PageRegistration[];
  routes: readonly RouteRegistration[];
  i18n: readonly I18nRegistration[];
  initializers: readonly InitializerRegistration[];
  services: readonly ServiceRegistration[];
  pageMap: Readonly<Record<string, PageRegistration['component']>>;
  servicesMap: ReadonlyMap<string, unknown>;
}
