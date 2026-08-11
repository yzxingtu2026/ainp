import assert from 'node:assert/strict';
import test from 'node:test';
import { findRequestErrorBoundaryViolations } from './check-request-error-boundary.mjs';

test('rejects a destructured request error message in a toast', () => {
  const source = `
    const { data, error } = await fetchRows();
    if (error) window.$message?.error(error.message);
  `;
  assert.deepEqual(
    findRequestErrorBoundaryViolations(source).map(item => item.binding),
    ['error']
  );
});

test('rejects aliased and result-object request error messages', () => {
  const source = `
    const { error: saveError } = await saveRow();
    inlineError.value = saveError?.message || 'failed';
    const result = await deleteRow();
    if (result.error) message.error(result.error.message);
  `;
  assert.deepEqual(
    findRequestErrorBoundaryViolations(source).map(item => item.binding),
    ['saveError', 'result']
  );
});

test('allows default global handling and explicit safe inline handling', () => {
  const source = `
    const { data, error } = await fetchRows();
    if (error) return;
    const result = await saveRow({ errorMode: 'inline' });
    if (result.error) inlineError.value = getRequestErrorMessage(result.error, 'failed');
  `;
  assert.deepEqual(findRequestErrorBoundaryViolations(source), []);
});

test('allows local non-request exceptions', () => {
  const source = `
    try { await fetch('/download'); }
    catch (error) { window.$message?.error(error instanceof Error ? error.message : 'failed'); }
  `;
  assert.deepEqual(findRequestErrorBoundaryViolations(source), []);
});
