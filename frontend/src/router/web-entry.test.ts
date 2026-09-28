import assert from 'node:assert/strict';
import { test } from 'node:test';
import { resolveWebEntry } from './web-entry';

test('current and pinned Admin entries keep independent route bases', () => {
  const assets = '/__web/v1.2.3/';
  assert.deepEqual(resolveWebEntry('/system/users', assets, '/'), { base: '/', versioned: false });
  assert.deepEqual(resolveWebEntry('/__web/v1.2.3/', assets, '/'), { base: assets, versioned: true });
  assert.deepEqual(resolveWebEntry('/__web/v1.2.4/', assets, '/'), { base: '/', versioned: false });
});

test('mounted Web uses its mount for current entry and versioned base for pinned entry', () => {
  const assets = '/__web/v1.2.3/bigscreen/';
  assert.deepEqual(resolveWebEntry('/bigscreen/orders', assets, '/bigscreen/'), { base: '/bigscreen/', versioned: false });
  assert.deepEqual(resolveWebEntry('/__web/v1.2.3/bigscreen/', assets, '/bigscreen/'), { base: assets, versioned: true });
});
