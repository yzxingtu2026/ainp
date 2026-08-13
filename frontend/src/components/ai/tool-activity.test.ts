import assert from 'node:assert/strict';
import test from 'node:test';
import { toolResultDeferred, toolResultFailed } from './tool-activity';

test('deferred confirmation result is a distinct settled state', () => {
  const result = {
    status: 'failed',
    error_category: 'deferred_for_confirmation',
    summary: 'not executed'
  };

  assert.equal(toolResultDeferred(result), true);
  assert.equal(toolResultFailed(result), true);
  assert.equal(toolResultDeferred({ status: 'failed', error_category: 'execution_failed' }), false);
});
