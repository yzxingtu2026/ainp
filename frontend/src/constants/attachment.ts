/** 后端已注册的附件 resource_type；前端禁止自由拼接任意字符串。 */
export const ATTACHMENT_RESOURCE_TYPES = {
  demoTask: 'demo.task',
  agentAvatar: 'ai.agent',
  accountProfile: 'system.account_profile'
} as const;

export type RegisteredAttachmentResourceType =
  (typeof ATTACHMENT_RESOURCE_TYPES)[keyof typeof ATTACHMENT_RESOURCE_TYPES];

export const ATTACHMENT_ACTIONS = {
  uploadPrepare: 'system.attachment_upload_prepare',
  downloadPrepare: 'system.attachment_download_prepare',
  bind: 'system.attachment_bind',
  unbind: 'system.attachment_unbind',
  listByResource: 'system.attachment_list_by_resource',
  delete: 'system.attachment_delete',
  list: 'system.attachment_list',
  get: 'system.attachment_get',
  policyGet: 'system.attachment_policy_get'
} as const;

/** 与后端 DefaultUploadPolicy 对齐的前端预校验默认值（非唯一安全边界）。 */
export const DEFAULT_ATTACHMENT_UPLOAD_POLICY = {
  maxSizeBytes: 20 * 1024 * 1024,
  allowedMime: [
    'image/png',
    'image/jpeg',
    'image/gif',
    'image/webp',
    'video/mp4',
    'video/webm',
    'video/quicktime',
    'application/pdf',
    'text/plain',
    'text/markdown',
    'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
    'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
    'application/vnd.openxmlformats-officedocument.presentationml.presentation',
    'application/zip',
    'application/octet-stream'
  ],
  allowedExt: [
    '.png',
    '.jpg',
    '.jpeg',
    '.gif',
    '.webp',
    '.mp4',
    '.webm',
    '.mov',
    '.pdf',
    '.txt',
    '.md',
    '.docx',
    '.xlsx',
    '.pptx',
    '.zip'
  ],
  maxUploadsOnce: 10,
  concurrency: 2
} as const;

export type AttachmentUploadItemStatus =
  | 'queued'
  | 'preparing'
  | 'uploading'
  | 'completing'
  | 'success'
  | 'failed'
  | 'cancelled';
