import assert from 'node:assert/strict';
import test from 'node:test';
import { resolveModelContextBudget } from './model-context-budget';

test('rejects invalid enabled chat limits with field-level reasons', () => {
  assert.equal(resolveModelContextBudget('chat', null, null).contextWindowError, 'positiveInteger');
  assert.equal(resolveModelContextBudget('chat', 128000, 0).maxOutputTokensError, 'positiveInteger');
  assert.equal(resolveModelContextBudget('chat', 128000.5, 8192).contextWindowError, 'positiveInteger');
  assert.equal(resolveModelContextBudget('chat', 128000, 8192.5).maxOutputTokensError, 'positiveInteger');
  assert.equal(
    resolveModelContextBudget('chat', Number.MAX_SAFE_INTEGER + 1, 8192).contextWindowError,
    'positiveInteger'
  );
  assert.equal(resolveModelContextBudget('chat', 128000, 128000).maxOutputTokensError, 'outputWithinWindow');
  assert.equal(resolveModelContextBudget('chat', 128000, 1000000).maxOutputTokensError, 'outputWithinWindow');
  assert.equal(resolveModelContextBudget('chat', 10, 9).maxOutputTokensError, 'inputCapacity');
});

test('returns the same usable input budget as the backend formula', () => {
  assert.deepEqual(resolveModelContextBudget('chat', 128000, 8192), {
    valid: true,
    safetyMarginTokens: 12800,
    availableInputTokens: 107008
  });
});

test('does not apply chat output limits to other model types', () => {
  assert.equal(resolveModelContextBudget('embedding', null, null).valid, true);
});
