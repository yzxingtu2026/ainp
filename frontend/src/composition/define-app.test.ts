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
  const definition = validDefinition();
  definition.pages.push({ id: '403', owner: 'ainp', component });
  const app = defineAdminApp(definition);
  assert.deepEqual(app.modules, ['core', 'system']);
  assert.deepEqual(app.profileExtensions, []);
  assert.deepEqual(app.shellExtensions, []);
  assert.equal(app.pageMap.example_orders, component);
  assert.equal(app.pageMap['403'], component);
});

test('resolves deterministic frozen shell extensions', () => {
  const componentA = () => Promise.resolve({});
  const componentB = () => Promise.resolve({});
  const app = defineAdminApp({
    ...validDefinition(),
    shellExtensions: [
      { id: 'other.second', owner: 'other', placement: 'header.context', order: 20, component: componentB },
      { id: 'example.second', owner: 'example', placement: 'header.context', order: 20, component: componentA },
      { id: 'example.first', owner: 'example', placement: 'header.context', order: 10, component: componentA }
    ]
  });
  assert.deepEqual(
    app.shellExtensions.map(item => item.id),
    ['example.first', 'example.second', 'other.second']
  );
  assert.ok(Object.isFrozen(app.shellExtensions));
  assert.ok(app.shellExtensions.every(Object.isFrozen));
});

test('fails closed for invalid shell extensions', () => {
  const base = validDefinition();
  const extension = {
    id: 'example.context',
    owner: 'example',
    placement: 'header.context' as const,
    order: 10,
    component
  };
  assert.throws(
    () => defineAdminApp({ ...base, shellExtensions: [extension, extension] }),
    /duplicate shell extension/
  );
  assert.throws(
    () => defineAdminApp({ ...base, shellExtensions: [{ ...extension, id: 'other.context' }] }),
    /owner namespace/
  );
  assert.throws(
    () => defineAdminApp({ ...base, shellExtensions: [{ ...extension, owner: 'Bad Owner' }] }),
    /invalid shell extension owner/
  );
  assert.throws(
    () =>
      defineAdminApp({
        ...base,
        shellExtensions: [{ ...extension, placement: 'header.tools' as 'header.context' }]
      }),
    /unknown shell extension placement/
  );
  for (const order of [-1, 1.5, Number.MAX_SAFE_INTEGER + 1]) {
    assert.throws(
      () => defineAdminApp({ ...base, shellExtensions: [{ ...extension, order }] }),
      /invalid shell extension order/
    );
  }
  assert.throws(
    () => defineAdminApp({ ...base, shellExtensions: [{ ...extension, component: null as never }] }),
    /invalid shell extension component/
  );
});

test('rejects duplicate registrations with both owners', () => {
  const definition = validDefinition();
  definition.pages.push({ id: 'example_orders', owner: 'other', component });
  assert.throws(() => defineAdminApp(definition), /owners "example" and "other"/);
});

test('rejects malformed page keys while allowing numeric builtin keys', () => {
  const definition = validDefinition();
  assert.doesNotThrow(() =>
    defineAdminApp({ ...definition, pages: [...definition.pages, { id: '404', owner: 'ainp', component }] })
  );
  assert.throws(
    () => defineAdminApp({ ...definition, pages: [...definition.pages, { id: 'view.404', owner: 'ainp', component }] }),
    /invalid page id/
  );
});

test('rejects unknown pages, unsafe paths and illegal module combinations', () => {
  assert.throws(
    () =>
      defineAdminApp({ ...validDefinition(), routes: [{ id: 'x.route', owner: 'x', path: '/x', pageKey: 'missing' }] }),
    /unknown page/
  );
  assert.throws(
    () =>
      defineAdminApp({
        ...validDefinition(),
        routes: [{ id: 'x.route', owner: 'x', path: 'https://evil.test/x', pageKey: 'example_orders' }]
      }),
    /invalid route path/
  );
  assert.throws(() => defineAdminApp({ ...validDefinition(), modules: ['core', 'operations'] }), /requires "system"/);
});

test('rejects remote brand assets', () => {
  assert.throws(
    () =>
      defineAdminApp({
        ...validDefinition(),
        brand: { ...validDefinition().brand, logo: 'https://cdn.test/logo.svg' }
      }),
    /build-local/
  );
});

test('resolves deterministic frozen profile extensions', () => {
  const componentA = () => Promise.resolve({});
  const componentB = () => Promise.resolve({});
  const app = defineAdminApp({
    ...validDefinition(),
    profileExtensions: [
      { id: 'other.second', owner: 'other', placement: 'tab', label: 'Second', order: 20, component: componentB },
      {
        id: 'example.first',
        owner: 'example',
        placement: 'tab',
        labelKey: 'example.profile.first',
        order: 10,
        component: componentA
      }
    ]
  });
  assert.deepEqual(
    app.profileExtensions.map(item => item.id),
    ['example.first', 'other.second']
  );
  assert.ok(Object.isFrozen(app.profileExtensions));
  assert.ok(app.profileExtensions.every(Object.isFrozen));
});

test('fails closed for invalid profile extensions', () => {
  const base = validDefinition();
  const extension = {
    id: 'example.identity',
    owner: 'example',
    placement: 'tab' as const,
    label: 'Identity',
    component
  };
  assert.throws(
    () => defineAdminApp({ ...base, profileExtensions: [extension, extension] }),
    /duplicate profile extension/
  );
  assert.throws(
    () => defineAdminApp({ ...base, profileExtensions: [{ ...extension, id: 'basic' }] }),
    /reserved platform tab/
  );
  assert.throws(
    () => defineAdminApp({ ...base, profileExtensions: [{ ...extension, id: 'other.identity' }] }),
    /owner namespace/
  );
  assert.throws(
    () => defineAdminApp({ ...base, profileExtensions: [{ ...extension, placement: 'panel' as 'tab' }] }),
    /unknown profile extension placement/
  );
  assert.throws(
    () => defineAdminApp({ ...base, profileExtensions: [{ ...extension, component: null as never }] }),
    /invalid profile extension component/
  );
  assert.throws(
    () => defineAdminApp({ ...base, profileExtensions: [{ ...extension, label: '' }] }),
    /requires label or labelKey/
  );
});

test('fails closed before generated router transformation', () => {
  const layouts = { base: component };
  const pages = { example_orders: component };
  assert.doesNotThrow(() =>
    assertRegisteredRouteComponents(
      [
        {
          name: 'example',
          component: 'layout.base',
          children: [{ name: 'example_orders', component: 'view.example_orders' }]
        }
      ],
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
