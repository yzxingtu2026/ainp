import assert from 'node:assert/strict';
import test from 'node:test';
import type { ElegantConstRoute } from '@elegant-router/types';
import { hasVisibleMenuProjection } from './menu-visibility';

function route(name: string, hideInMenu = false, children?: ElegantConstRoute[]): ElegantConstRoute {
  return {
    name,
    path: `/${name}`,
    component: children ? 'layout.base' : `view.${name}`,
    meta: { title: name, hideInMenu },
    children
  } as ElegantConstRoute;
}

test('hides a parent whose descendants are all hidden', () => {
  const ai = route('ai', false, [route('ai_workspace', true), route('ai_agent-builder', true)]);

  assert.equal(hasVisibleMenuProjection(ai), false);
});

test('keeps a parent when any nested route is visible', () => {
  const system = route('system', false, [route('system_users', true), route('system_profile')]);

  assert.equal(hasVisibleMenuProjection(system), true);
});

test('keeps visible leaf routes and excludes hidden leaf routes', () => {
  assert.equal(hasVisibleMenuProjection(route('home')), true);
  assert.equal(hasVisibleMenuProjection(route('workspace', true)), false);
});
