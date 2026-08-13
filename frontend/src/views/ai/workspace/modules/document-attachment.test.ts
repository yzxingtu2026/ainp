import assert from 'node:assert/strict';
import test from 'node:test';
import { isWorkspaceAttachment, isWorkspaceDocument } from './document-attachment';

function attachment(name: string, contentType: string): Api.Attachment.Item {
  return {
    id: name,
    original_name: name,
    content_type: contentType,
    size: 1,
    status: 'ready',
    uploader_id: 'user',
    created_at: '',
    updated_at: ''
  };
}

test('accepts the five document formats and existing image formats', () => {
  const documents = [
    attachment('report.pdf', 'application/pdf'),
    attachment('data.xlsx', 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet'),
    attachment('memo.docx', 'application/vnd.openxmlformats-officedocument.wordprocessingml.document'),
    attachment('notes.txt', 'text/plain'),
    attachment('guide.md', 'text/markdown')
  ];
  for (const item of documents) {
    assert.equal(isWorkspaceDocument(item), true);
    assert.equal(isWorkspaceAttachment(item), true);
  }
  assert.equal(isWorkspaceAttachment(attachment('photo.webp', 'image/webp')), true);
});

test('requires matching MIME and extension and rejects out-of-scope formats', () => {
  assert.equal(isWorkspaceDocument(attachment('spoofed.txt', 'application/pdf')), false);
  assert.equal(isWorkspaceAttachment(attachment('spoofed.pdf', 'text/plain')), false);
  assert.equal(isWorkspaceDocument(attachment('slides.pptx', 'application/vnd.openxmlformats-officedocument.presentationml.presentation')), false);
  assert.equal(isWorkspaceAttachment(attachment('table.csv', 'text/csv')), false);
});
