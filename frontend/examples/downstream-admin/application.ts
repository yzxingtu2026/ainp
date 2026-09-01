import { defineAdminApp } from '../../src/composition';
import { aiPages } from '../../src/composition/ai-pages';
import { corePages } from '../../src/composition/core-pages';
import { systemPages } from '../../src/composition/system-pages';
import OrdersPage from './src/orders-page.vue';
import { ordersService } from './src/orders-service';
import { installOrdersStore } from './src/orders-store';
import zhCN from './src/locales/zh-CN';

export const adminApplication = defineAdminApp({
  id: 'example.orders-admin',
  name: 'Example Orders',
  version: 'v0.1.0',
  baselineVersion: 'v0.1.0',
  brand: {
    name: 'Example Orders',
    logo: '/brand/example-logo.svg',
    favicon: '/brand/example-logo.svg',
    themeColor: '#1677ff'
  },
  modules: ['core', 'system', 'ai'],
  pages: [
    ...corePages,
    ...systemPages,
    ...aiPages,
    { id: 'example_orders', owner: 'example.orders', component: OrdersPage }
  ],
  routes: [{ id: 'example.orders', owner: 'example.orders', path: '/example/orders', pageKey: 'example_orders' }],
  i18n: [{ id: 'example.orders', owner: 'example.orders', messages: { 'zh-CN': zhCN } }],
  services: [{ id: 'example.orders', owner: 'example.orders', service: ordersService }],
  initializers: [{ id: 'example.orders.store', owner: 'example.orders', setup: installOrdersStore }],
  profileExtensions: [
    {
      id: 'example.orders.account',
      owner: 'example.orders',
      placement: 'tab',
      label: '账号扩展示例',
      order: 100,
      component: () => import('./src/profile-extension.vue')
    }
  ],
  shellExtensions: [
    {
      id: 'example.orders.business-context',
      owner: 'example.orders',
      placement: 'header.context',
      order: 100,
      component: () => import('./src/business-context-switcher.vue')
    }
  ],
  homePageKey: 'example_orders'
});
