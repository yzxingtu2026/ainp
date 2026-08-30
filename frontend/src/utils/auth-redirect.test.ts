import assert from 'node:assert/strict';
import test from 'node:test';
import { isSafeInternalRedirect, normalizeRedirectMethods, safeAuthorizeURL } from './auth-redirect';

test('internal redirect rejects cross-origin and protocol-relative targets', () => {
  assert.equal(isSafeInternalRedirect('/system/accounts?tab=active'), true);
  for (const value of ['//evil.example', 'https://evil.example', 'javascript:alert(1)', '']) {
    assert.equal(isSafeInternalRedirect(value), false);
  }
});

test('authorize URL only accepts safe http transports', () => {
  assert.equal(safeAuthorizeURL('https://login.example/authorize?state=x').startsWith('https://login.example/'), true);
  assert.equal(safeAuthorizeURL('http://localhost:3000/authorize').startsWith('http://localhost:3000/'), true);
  assert.equal(safeAuthorizeURL('http://login.example/authorize'), '');
  assert.equal(safeAuthorizeURL('javascript:alert(1)'), '');
  assert.equal(safeAuthorizeURL('https://user:pass@login.example/authorize'), '');
});

test('redirect methods are validated, deduplicated and sorted', () => {
  const methods = normalizeRedirectMethods([
    { method: 'wecom', capabilities: { challenge: true, redirect: true, order: 20 } },
    { method: 'dingtalk', capabilities: { challenge: true, redirect: true, icon: 'simple-icons:dingtalk', order: 10 } },
    { method: 'dingtalk', capabilities: { challenge: true, redirect: true, order: 1 } },
    { method: 'unsafe', capabilities: { challenge: true, redirect: true, icon: '<svg>' } },
    { method: 'password', capabilities: { challenge: false, redirect: false } }
  ]);
  assert.deepEqual(
    methods.map(item => item.method),
    ['dingtalk', 'wecom']
  );
});
