import type { ResolvedAdminApp } from './contract';

let activeApplication: ResolvedAdminApp | undefined;

export function installAdminApplication(application: ResolvedAdminApp) {
  if (activeApplication && activeApplication !== application) {
    throw new Error(`[admin-composition] application already installed as "${activeApplication.id}"`);
  }
  activeApplication = application;
  document.title = application.brand.titleSuffix ? `${application.name} - ${application.brand.titleSuffix}` : application.name;
  document.querySelector<HTMLLinkElement>('link[rel="icon"]')?.setAttribute('href', application.brand.favicon);
}

export function getAdminApplication(): ResolvedAdminApp {
  if (!activeApplication) throw new Error('[admin-composition] application is not installed');
  return activeApplication;
}
