import assert from 'node:assert/strict';
import test from 'node:test';
import { collapseRetriedToolOperations, toolResultDeferred, toolResultFailed } from './tool-activity';

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

test('retry updates the deferred business operation without increasing count', () => {
  const operations = collapseRetriedToolOperations([
    {
      toolCallId: 'call-a',
      toolName: 'action__demo_write',
      request: { id: 'a', options: { enabled: true, count: 1 } },
      result: { status: 'failed', error_category: 'deferred_for_confirmation' },
      hasResult: true
    },
    {
      toolCallId: 'retry-a',
      retryOfToolCallId: 'call-a',
      toolName: 'action__demo_write',
      request: { options: { count: 1, enabled: true }, id: 'a' },
      result: { status: 'completed', call_id: 'retry-a' },
      hasResult: true
    }
  ]);
  assert.equal(operations.length, 1);
  assert.equal(operations[0].toolCallId, 'call-a');
  assert.equal(operations[0].retriedAfterDeferral, true);
  assert.deepEqual(operations[0].result, { status: 'completed', call_id: 'retry-a' });
});

test('pending retry keeps the original card in waiting state', () => {
  const operations = collapseRetriedToolOperations([
    {
      toolCallId: 'call-a',
      toolName: 'action__demo_write',
      request: {},
      result: { status: 'failed', error_category: 'deferred_for_confirmation' },
      hasResult: true
    },
    {
      toolCallId: 'retry-a',
      retryOfToolCallId: 'call-a',
      toolName: 'action__demo_write',
      request: {},
      result: undefined,
      hasResult: false
    }
  ]);
  assert.equal(operations.length, 1);
  assert.equal(operations[0].toolCallId, 'call-a');
  assert.equal(operations[0].hasResult, false);
});

test('non-deferred and ambiguous retry relations fail closed', () => {
  const nonDeferred = collapseRetriedToolOperations([
    { toolCallId: 'call-a', toolName: 'write', request: {}, result: { status: 'completed' }, hasResult: true },
    {
      toolCallId: 'retry-a',
      retryOfToolCallId: 'call-a',
      toolName: 'write',
      request: {},
      result: { status: 'completed' },
      hasResult: true
    }
  ]);
  assert.equal(nonDeferred.length, 2);

  const ambiguous = collapseRetriedToolOperations([
    {
      toolCallId: 'call-b',
      toolName: 'write',
      request: {},
      result: { status: 'failed', error_category: 'deferred_for_confirmation' },
      hasResult: true
    },
    {
      toolCallId: 'retry-b-1',
      retryOfToolCallId: 'call-b',
      toolName: 'write',
      request: {},
      result: { status: 'completed' },
      hasResult: true
    },
    {
      toolCallId: 'retry-b-2',
      retryOfToolCallId: 'call-b',
      toolName: 'write',
      request: {},
      result: { status: 'completed' },
      hasResult: true
    }
  ]);
  assert.equal(ambiguous.length, 3);

  const mismatched = collapseRetriedToolOperations([
    {
      toolCallId: 'call-c',
      toolName: 'write',
      request: { id: 'original' },
      result: { status: 'failed', error_category: 'deferred_for_confirmation' },
      hasResult: true
    },
    {
      toolCallId: 'retry-c',
      retryOfToolCallId: 'call-c',
      toolName: 'write',
      request: { id: 'different' },
      result: { status: 'completed' },
      hasResult: true
    }
  ]);
  assert.equal(mismatched.length, 2);
});
