import assert from 'node:assert/strict';
import test from 'node:test';
import { assertRegisteredRouteComponents, defineAdminApp } from './define-app';

const component = () => Promise.resolve({});

function validDefinition() {
  return {
    id: 'example.admin',
    name: 'Example',
    version: 'v1.0.0',
    baselineVersion: 'v1.0.0',
    brand: { name: 'Example', logo: '/brand/logo.svg', favicon: '/brand/favicon.svg' },
    modules: ['core', 'system'] as const,
    pages: [{ id: 'example_orders', owner: 'example', component }],
    routes: [{ id: 'example.orders', owner: 'example', path: '/example/orders', pageKey: 'example_orders' }]
  };
}

test('resolves a deterministic application registry', () => {
  const app = defineAdminApp(validDefinition());
  assert.deepEqual(app.modules, ['core', 'system']);
  assert.equal(app.pageMap.example_orders, component);
});

test('rejects duplicate registrations with both owners', () => {
  const definition = validDefinition();
  definition.pages.push({ id: 'example_orders', owner: 'other', component });
  assert.throws(() => defineAdminApp(definition), /owners "example" and "other"/);
});

test('rejects unknown pages, unsafe paths and illegal module combinations', () => {
  assert.throws(
    () => defineAdminApp({ ...validDefinition(), routes: [{ id: 'x.route', owner: 'x', path: '/x', pageKey: 'missing' }] }),
    /unknown page/
  );
  assert.throws(
    () => defineAdminApp({ ...validDefinition(), routes: [{ id: 'x.route', owner: 'x', path: 'https://evil.test/x', pageKey: 'example_orders' }] }),
    /invalid route path/
  );
  assert.throws(() => defineAdminApp({ ...validDefinition(), modules: ['core', 'operations'] }), /requires "system"/);
});

test('rejects remote brand assets', () => {
  assert.throws(
    () => defineAdminApp({ ...validDefinition(), brand: { ...validDefinition().brand, logo: 'https://cdn.test/logo.svg' } }),
    /build-local/
  );
});

test('fails closed before generated router transformation', () => {
  const layouts = { base: component };
  const pages = { example_orders: component };
  assert.doesNotThrow(() =>
    assertRegisteredRouteComponents(
      [{ name: 'example', component: 'layout.base', children: [{ name: 'example_orders', component: 'view.example_orders' }] }],
      pages,
      layouts
    )
  );
  assert.throws(
    () => assertRegisteredRouteComponents([{ name: 'evil', component: 'https://evil.test/module.js' }], pages, layouts),
    /unsafe component/
  );
  assert.throws(
    () => assertRegisteredRouteComponents([{ name: 'missing', component: 'view.missing' }], pages, layouts),
    /unknown page/
  );
});
