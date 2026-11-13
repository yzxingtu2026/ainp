import assert from 'node:assert/strict';
import test from 'node:test';
import {
  PAGE_CAPTURE_EVENT_REQUESTED,
  computeDownscaledDimensions,
  parsePageCaptureRequestEvent
} from './page-capture';

test('parses valid page capture SSE request events and rejects forged or unknown payloads', () => {
  const valid = parsePageCaptureRequestEvent({
    id: 'ev-1',
    run_id: 'run-1',
    sequence: 4,
    type: PAGE_CAPTURE_EVENT_REQUESTED,
    payload: {
      capture_request_id: 'cap-req-1',
      kind: 'page.capture',
      tool_call_id: 'tc-1',
      expires_at: '2026-11-13T08:00:00Z'
    },
    created_at: '2026-11-13T07:59:30Z'
  });

  assert.deepEqual(valid, {
    captureRequestId: 'cap-req-1',
    kind: 'page.capture',
    toolCallId: 'tc-1',
    expiresAt: '2026-11-13T08:00:00Z'
  });

  assert.equal(
    parsePageCaptureRequestEvent({
      id: 'ev-2',
      run_id: 'run-1',
      sequence: 5,
      type: PAGE_CAPTURE_EVENT_REQUESTED,
      payload: { capture_request_id: 'cap-req-2', kind: 'unknown.kind' },
      created_at: '2026-11-13T07:59:30Z'
    }),
    null
  );

  assert.equal(
    parsePageCaptureRequestEvent({
      id: 'ev-3',
      run_id: 'run-1',
      sequence: 6,
      type: 'run.delta',
      payload: { capture_request_id: 'cap-req-3', kind: 'page.capture' },
      created_at: '2026-11-13T07:59:30Z'
    }),
    null
  );
});

test('downscales oversized dimensions to respect max edge and max pixel budget', () => {
  const normal = computeDownscaledDimensions(1280, 720);
  assert.equal(normal.width, 1280);
  assert.equal(normal.height, 720);
  assert.equal(normal.scale, 1);

  const wide = computeDownscaledDimensions(3200, 1800);
  assert.ok(wide.width <= 1600);
  assert.ok(wide.height <= 1600);
  assert.ok(wide.width * wide.height <= 2_000_000);

  const square = computeDownscaledDimensions(2400, 2400);
  assert.ok(square.width <= 1600);
  assert.ok(square.height <= 1600);
  assert.ok(square.width * square.height <= 2_000_000);
});
