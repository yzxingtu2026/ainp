import assert from 'node:assert/strict';
import test from 'node:test';
import { loadProfileExtensionComponent, resolveProfileTabId, shouldReplaceProfileTabQuery } from './profile-extension';

const extension = {
  id: 'example.identity',
  owner: 'example',
  placement: 'tab' as const,
  label: 'Identity',
  component: () => Promise.resolve({ name: 'IdentityExtension' })
};

test('restores builtin and registered profile tabs and rejects unknown query values', () => {
  assert.equal(resolveProfileTabId('security', [extension]), 'security');
  assert.equal(resolveProfileTabId('example.identity', [extension]), 'example.identity');
  assert.equal(resolveProfileTabId('unknown', [extension]), 'basic');
  assert.equal(resolveProfileTabId(['example.identity'], [extension]), 'basic');
});

test('preserves the legacy query-free default and replaces explicit invalid tabs', () => {
  assert.equal(shouldReplaceProfileTabQuery(undefined, 'basic'), false);
  assert.equal(shouldReplaceProfileTabQuery('security', 'security'), false);
  assert.equal(shouldReplaceProfileTabQuery('unknown', 'basic'), true);
  assert.equal(shouldReplaceProfileTabQuery(['example.identity'], 'basic'), true);
});

test('loads direct and default-export lazy profile components', async () => {
  assert.equal((await loadProfileExtensionComponent(extension)).name, 'IdentityExtension');
  assert.equal(
    (
      await loadProfileExtensionComponent({
        ...extension,
        component: () => Promise.resolve({ default: { name: 'DefaultIdentityExtension' } })
      })
    ).name,
    'DefaultIdentityExtension'
  );
});

test('propagates lazy profile component failures to the page error boundary', async () => {
  await assert.rejects(
    loadProfileExtensionComponent({ ...extension, component: () => Promise.reject(new Error('load failed')) }),
    /load failed/
  );
});
