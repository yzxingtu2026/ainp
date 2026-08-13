export const workspaceAttachmentExts = ['.jpg', '.jpeg', '.png', '.gif', '.webp', '.pdf', '.xlsx', '.docx', '.txt', '.md'] as const;
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
