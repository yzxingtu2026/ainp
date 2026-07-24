import assert from 'node:assert/strict';
import test from 'node:test';
import { normalizeClipboardImage } from './clipboard-image';

test('normalizes supported clipboard images to stable upload filenames', () => {
  const source = new File(['png'], '', { type: 'image/png' });
  const normalized = normalizeClipboardImage(source, 1, 1234);

  assert.equal(normalized?.name, 'clipboard-1234-2.png');
  assert.equal(normalized?.type, 'image/png');
  assert.equal(normalized?.lastModified, 1234);
});

test('rejects unsupported clipboard image formats', () => {
  const source = new File(['svg'], 'image.svg', { type: 'image/svg+xml' });

  assert.equal(normalizeClipboardImage(source, 0, 1234), null);
});
