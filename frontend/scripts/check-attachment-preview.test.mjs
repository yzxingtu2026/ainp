import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import test from 'node:test';
import { findAttachmentPreviewViolations } from './check-attachment-preview.mjs';

function writeFixture(root, relativePath, content) {
  const target = path.join(root, relativePath);
  fs.mkdirSync(path.dirname(target), { recursive: true });
  fs.writeFileSync(target, content);
}

test('detects consumer bypass and preview support regressions', t => {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), 'attachment-preview-check-'));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  writeFixture(root, 'src/components/common/attachment-preview.vue', 'useAttachmentPreview <img noopener,noreferrer');
  writeFixture(root, 'src/hooks/business/attachment.ts', "normalized === 'application/pdf'");
  writeFixture(root, 'src/components/common/attachment-list.vue', 'useAttachmentPreview <img');
  writeFixture(root, 'src/views/system/attachments/index.vue', 'AttachmentPreview <video');

  const violations = findAttachmentPreviewViolations(root);

  assert.ok(violations.some(item => item.includes('pdf 渲染分支')));
  assert.ok(violations.some(item => item.includes('video 渲染分支')));
  assert.ok(violations.some(item => item.includes('Blob 预览必须声明大小上限')));
  assert.ok(violations.some(item => item.includes('attachment-list.vue 未复用')));
  assert.ok(violations.some(item => item.includes('直接使用 Hook')));
  assert.ok(violations.some(item => item.includes('局部 <video')));
});
