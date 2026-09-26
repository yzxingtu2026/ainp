import assert from 'node:assert/strict';
import { copyFileSync, mkdtempSync, readdirSync, readFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import test from 'node:test';
import { extname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import axios from 'axios';
import { loadEnv } from 'vite';
import { buildServiceURL, getServiceBaseURL } from './service';

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

test('production config keeps Axios and native requests same-origin while development keeps its proxy', () => {
  const envDir = mkdtempSync(join(tmpdir(), 'ainp-service-env-'));
  const keys = ['VITE_SERVICE_BASE_URL', 'VITE_OTHER_SERVICE_BASE_URL'] as const;
  const inherited = keys.map(key => process.env[key]);

  try {
    // Test the committed defaults, independent of developer .env.local / shell overrides.
    for (const filename of ['.env', '.env.prod', '.env.test']) {
      copyFileSync(fileURLToPath(new URL(`../../${filename}`, import.meta.url)), join(envDir, filename));
    }
    for (const key of keys) delete process.env[key];

    const prod = loadEnv('prod', envDir) as Env.ImportMeta;
    const { baseURL, otherBaseURL } = getServiceBaseURL(prod, false);
    assert.equal(baseURL, '/');
    assert.equal(otherBaseURL.demo, '/');
    const origin = 'https://deployed.example.com';
    const loginPath = '/api/v1/system/auth/login';
    const loginURL = axios.getUri({ baseURL, url: loginPath });
    assert.equal(new URL(loginURL, origin).href, `${origin}${loginPath}`);
    for (const path of [subscribePath, '/api/v1/attachments/transfer', '/api/v1/mcp/oauth/authorize']) {
      assert.equal(new URL(buildServiceURL(baseURL, path), origin).href, `${origin}${path}`);
    }
    const dev = loadEnv('test', envDir) as Env.ImportMeta;
    assert.equal(getServiceBaseURL(dev, false).baseURL, 'http://127.0.0.1:8080');
    assert.deepEqual(getServiceBaseURL(dev, true), {
      baseURL: '/proxy-default',
      otherBaseURL: { demo: '/proxy-demo' }
    });
  } finally {
    keys.forEach((key, index) => {
      if (inherited[index] === undefined) delete process.env[key];
      else process.env[key] = inherited[index];
    });
    rmSync(envDir, { recursive: true, force: true });
  }
});
