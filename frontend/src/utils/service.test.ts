import assert from 'node:assert/strict';
import { readdirSync, readFileSync } from 'node:fs';
import test from 'node:test';
import { extname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { buildServiceURL } from './service';

const subscribePath = '/api/v1/ai/agent-runs/subscribe';

test('normalizes service base URL and path boundaries', () => {
  const cases = [
    ['/', subscribePath],
    ['', subscribePath],
    ['/proxy-default', `/proxy-default${subscribePath}`],
    ['/proxy-default/', `/proxy-default${subscribePath}`],
    ['https://api.example.com', `https://api.example.com${subscribePath}`],
    ['https://api.example.com/', `https://api.example.com${subscribePath}`]
  ] as const;

  for (const [baseURL, expected] of cases) {
    assert.equal(buildServiceURL(baseURL, subscribePath), expected);
  }
});

test('encodes structured query parameters', () => {
  const query = new URLSearchParams({
    client_id: 'client with spaces',
    redirect_uri: 'https://app.example.com/callback?a=1&b=2'
  });

  assert.equal(
    buildServiceURL('/', '/api/v1/mcp/oauth/authorize', query),
    '/api/v1/mcp/oauth/authorize?client_id=client+with+spaces&redirect_uri=https%3A%2F%2Fapp.example.com%2Fcallback%3Fa%3D1%26b%3D2'
  );
});

test('rejects absolute and protocol-relative paths', () => {
  assert.throws(() => buildServiceURL('/', 'https://api.example.com/v1'), /application-relative/);
  assert.throws(() => buildServiceURL('/', '//api.example.com/v1'), /application-relative/);
});

test('native fetch calls do not interpolate a service base URL with an API path', () => {
  const sourceRoot = fileURLToPath(new URL('../', import.meta.url));
  const sourceFiles = readdirSync(sourceRoot, { recursive: true, withFileTypes: true })
    .filter(entry => entry.isFile() && ['.ts', '.tsx', '.vue', '.js', '.jsx'].includes(extname(entry.name)))
    .map(entry => join(entry.parentPath, entry.name));
  const unsafeInterpolation = /\$\{[^}\n]*(?:baseURL|baseUrl)[^}\n]*\}\/(?:api|proxy-)/;
  const violations = sourceFiles.filter(file => unsafeInterpolation.test(readFileSync(file, 'utf8')));

  assert.deepEqual(violations, []);
});
