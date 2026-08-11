import assert from 'node:assert/strict';
import test from 'node:test';
import {
  getRequestErrorMessage,
  getRequestErrorMode,
  getStandaloneErrorMessage,
  shouldShowGlobalRequestError
} from './error';

function axiosError(options?: { errorMode?: 'global' | 'silent' | 'inline' | 'custom'; message?: string }) {
  return {
    isAxiosError: true,
    message: 'Request failed with status code 409',
    config: options?.errorMode ? { errorMode: options.errorMode } : {},
    response: {
      data: {
        code: 409,
        message: options?.message,
        message_key: 'common.conflict',
        data: null
      }
    }
  };
}

test('global mode is the default and explicit ownership modes suppress it', () => {
  assert.equal(getRequestErrorMode(axiosError()), 'global');
  assert.equal(shouldShowGlobalRequestError(axiosError()), true);
  for (const errorMode of ['silent', 'inline', 'custom'] as const) {
    assert.equal(getRequestErrorMode(axiosError({ errorMode })), errorMode);
    assert.equal(shouldShowGlobalRequestError(axiosError({ errorMode })), false);
  }
});

test('request messages use the localized envelope and never Axios technical text', () => {
  assert.equal(getRequestErrorMessage(axiosError({ message: '数据已被更新' }), '操作失败'), '数据已被更新');
  assert.equal(getRequestErrorMessage(axiosError(), '操作失败'), '操作失败');
  assert.equal(getStandaloneErrorMessage(axiosError(), '操作失败'), '操作失败');
});

test('standalone local errors may retain an explicitly constructed user-facing message', () => {
  assert.equal(getStandaloneErrorMessage(new Error('授权请求无效或已过期'), '操作失败'), '授权请求无效或已过期');
});
