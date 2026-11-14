export const workspaceAttachmentExts = [
  '.jpg',
  '.jpeg',
  '.png',
  '.gif',
  '.webp',
  '.pdf',
  '.xlsx',
  '.docx',
  '.txt',
  '.md'
] as const;
export const workspaceAttachmentMimes = [
  'image/jpeg',
  'image/png',
  'image/gif',
  'image/webp',
  'application/pdf',
  'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
  'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
  'text/plain',
  'text/markdown'
] as const;

const mimeExtensions: Record<string, readonly string[]> = {
  'image/jpeg': ['.jpg', '.jpeg'],
  'image/png': ['.png'],
  'image/gif': ['.gif'],
  'image/webp': ['.webp'],
  'application/pdf': ['.pdf'],
  'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet': ['.xlsx'],
  'application/vnd.openxmlformats-officedocument.wordprocessingml.document': ['.docx'],
  'text/plain': ['.txt'],
  'text/markdown': ['.md']
};

function matchesWorkspaceType(item: Pick<Api.Attachment.Item, 'content_type' | 'original_name'>) {
  const extensions = mimeExtensions[item.content_type.toLowerCase()];
  const name = item.original_name.toLowerCase();
  return Boolean(extensions?.some(extension => name.endsWith(extension)));
}

export function isWorkspaceDocument(item: Pick<Api.Attachment.Item, 'content_type' | 'original_name'>) {
  return !item.content_type.toLowerCase().startsWith('image/') && matchesWorkspaceType(item);
}

export function isWorkspaceAttachment(item: Pick<Api.Attachment.Item, 'content_type' | 'original_name'>) {
  return matchesWorkspaceType(item);
}

export function attachmentExtBadge(name: string, contentType = ''): string {
  const trimmed = name.trim();
  const dotIndex = trimmed.lastIndexOf('.');
  if (dotIndex > 0 && dotIndex < trimmed.length - 1) {
    const ext = trimmed.slice(dotIndex + 1).replace(/[^a-zA-Z0-9]/g, '');
    if (ext) return ext.slice(0, 6).toUpperCase();
  }
  const normalized = contentType.toLowerCase();
  if (normalized === 'application/pdf') return 'PDF';
  if (normalized.includes('spreadsheetml')) return 'XLSX';
  if (normalized.includes('wordprocessingml')) return 'DOCX';
  if (normalized === 'text/markdown') return 'MD';
  if (normalized.startsWith('text/')) return 'TXT';
  if (normalized.startsWith('image/')) return normalized.slice(6).split('+')[0].toUpperCase().slice(0, 5) || 'IMG';
  return 'FILE';
}

export function workspaceDocumentIcon(contentType: string, name = ''): string {
  const normalized = contentType.toLowerCase();
  const lowerName = name.toLowerCase();
  if (normalized === 'application/pdf' || lowerName.endsWith('.pdf')) return 'mdi:file-pdf-box';
  if (normalized.includes('spreadsheetml') || lowerName.endsWith('.xlsx')) return 'mdi:file-excel-box';
  if (normalized.includes('wordprocessingml') || lowerName.endsWith('.docx')) return 'mdi:file-word-box';
  if (normalized === 'text/markdown' || lowerName.endsWith('.md')) return 'mdi:language-markdown-outline';
  if (normalized.startsWith('text/') || lowerName.endsWith('.txt')) return 'mdi:file-document-outline';
  return 'mdi:file-outline';
}
