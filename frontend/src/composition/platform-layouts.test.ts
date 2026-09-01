import assert from 'node:assert/strict';
import test from 'node:test';
import { platformLayouts } from './platform-layouts';

test('platform layouts stay lazy so downstream applications initialize before the UI tree', () => {
  assert.deepEqual(Object.keys(platformLayouts).sort(), ['base', 'blank']);
  assert.equal(typeof platformLayouts.base, 'function');
  assert.equal(typeof platformLayouts.blank, 'function');
});
