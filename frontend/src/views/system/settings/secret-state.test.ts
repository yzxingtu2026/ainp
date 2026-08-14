import { describe, expect, it } from 'vitest';
import { buildSettingUpdatePayload } from './secret-state';

const definition = {
  key: 'general.secret',
  category_key: 'general',
  title: 'Secret',
  order: 1,
  schema_version: 1,
  revision: 2,
  source: 'tenant',
  values: { endpoint: 'https://example.test' },
  fields: [
    { key: 'endpoint', type: 'text', label: 'Endpoint', required: true },
    { key: 'api_key', type: 'secret', label: 'API key', required: true, secret_configured: true },
    { key: 'optional', type: 'secret', label: 'Optional', required: false, secret_configured: true }
  ]
} satisfies Api.Setting.Definition;

describe('setting secret update payload', () => {
  it('omits unchanged and blank secret values', () => {
    const payload = buildSettingUpdatePayload(
      definition,
      { endpoint: 'https://example.test' },
      { api_key: '' },
      new Set()
    );
    expect(payload).toEqual({
      values: { endpoint: 'https://example.test' },
      secrets: {},
      clearSecretFields: [],
      hasSecretFields: true
    });
  });

  it('separates replacements and explicit clears from ordinary values', () => {
    const payload = buildSettingUpdatePayload(
      definition,
      { endpoint: 'https://new.test', api_key: 'must-not-leak' },
      { api_key: 'new-secret' },
      new Set(['optional'])
    );
    expect(payload.values).toEqual({ endpoint: 'https://new.test' });
    expect(payload.secrets).toEqual({ api_key: 'new-secret' });
    expect(payload.clearSecretFields).toEqual(['optional']);
  });
});
