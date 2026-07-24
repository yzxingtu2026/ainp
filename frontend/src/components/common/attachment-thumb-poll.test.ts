import assert from 'node:assert/strict';
import test from 'node:test';
import { mergePendingThumbnailStatuses, pendingThumbnailIds } from './attachment-thumb-poll';

function attachment(id: string, thumbStatus: Api.Attachment.Item['thumb_status'], contentType = 'image/png') {
  return {
    id,
    original_name: `${id}.png`,
    content_type: contentType,
    size: 10,
    status: 'ready',
    uploader_id: 'user-1',
    thumb_status: thumbStatus,
    created_at: '2026-07-24T00:00:00Z',
    updated_at: '2026-07-24T00:00:00Z'
  } as Api.Attachment.Item;
}

test('polls only pending image and video thumbnails', () => {
  const ids = pendingThumbnailIds([
    attachment('image', 'pending'),
    attachment('video', 'pending', 'video/mp4'),
    attachment('pdf', 'pending', 'application/pdf'),
    attachment('ready', 'ready')
  ]);

  assert.deepEqual([...ids], ['image', 'video']);
});

test('merges refreshed status only into currently pending records', () => {
  const current = [attachment('pending', 'pending'), attachment('ready', 'ready')];
  const merged = mergePendingThumbnailStatuses(current, [
    { ...attachment('pending', 'ready'), updated_at: '2026-07-24T00:00:03Z' },
    attachment('ready', 'failed')
  ]);

  assert.equal(merged[0].thumb_status, 'ready');
  assert.equal(merged[0].updated_at, '2026-07-24T00:00:03Z');
  assert.equal(merged[1].thumb_status, 'ready');
});
