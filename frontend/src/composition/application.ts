import { defineAdminApp } from './define-app';
import { aiPages } from './ai-pages';
import { corePages } from './core-pages';
import { demoPages } from './demo-pages';
import { operationsPages } from './operations-pages';
import { systemPages } from './system-pages';

export const adminApplication = defineAdminApp({
  id: 'ainp.admin',
  name: 'AINP',
  version: import.meta.env.VITE_BUILD_VERSION || 'dev',
  baselineVersion: import.meta.env.VITE_AINP_BASELINE_VERSION || import.meta.env.VITE_BUILD_VERSION || 'dev',
  brand: {
    name: 'AINP',
    logo: '/brand/ainp-logo.svg',
    favicon: '/brand/ainp-logo.svg',
    loginVisual: '/brand/ainp-login-visual.png',
    loginVisualDark: '/brand/ainp-login-visual-dark.png',
    themeColor: '#646cff'
  },
  modules: ['core', 'system', 'operations', 'ai', 'demo'],
  pages: [...corePages, ...systemPages, ...operationsPages, ...aiPages, ...demoPages]
});
