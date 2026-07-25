import assert from 'node:assert/strict';
import test from 'node:test';
import { createLatestRequestGate, loadAllCursorPages } from './message-sync';

test('loads every cursor page in sequence', async () => {
  const cursors: number[] = [];
  const items = await loadAllCursorPages(async afterSequence => {
    cursors.push(afterSequence);
    if (afterSequence === 0) return { items: ['a', 'b'], next_sequence: 2, has_more: true };
    return { items: ['c'], next_sequence: 3, has_more: false };
  });

  assert.deepEqual(cursors, [0, 2]);
  assert.deepEqual(items, ['a', 'b', 'c']);
});

test('rejects a non-advancing cursor', async () => {
  await assert.rejects(
    loadAllCursorPages(async () => ({ items: ['a'], next_sequence: 0, has_more: true })),
    /cursor did not advance/
  );
});

test('only the latest request remains current', () => {
  const gate = createLatestRequestGate();
  const older = gate.begin();
  const latest = gate.begin();

  assert.equal(gate.isCurrent(older), false);
  assert.equal(gate.isCurrent(latest), true);
  gate.invalidate();
  assert.equal(gate.isCurrent(latest), false);
});
