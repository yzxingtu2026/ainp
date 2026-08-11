import { request } from '../request';
import type { RequestErrorOptions } from '../request';

const ATTACHMENT_API = {
  uploadPrepare: '/api/v1/system/attachments/upload/prepare',
  downloadPrepare: '/api/v1/system/attachments/download/prepare',
  bind: '/api/v1/system/attachments/bind',
  unbind: '/api/v1/system/attachments/unbind',
  listByResource: '/api/v1/system/attachments/list_by_resource',
  delete: '/api/v1/system/attachments/delete',
  list: '/api/v1/system/attachments/list',
  get: '/api/v1/system/attachments/get',
  policy: '/api/v1/system/attachments/policy'
} as const;

export function prepareAttachmentUpload(
  payload: {
    original_name: string;
    content_type: string;
    size: number;
    transport?: Api.Attachment.UploadTransport;
  },
  options?: RequestErrorOptions
) {
  return request<Api.Attachment.TransferPrepare>({
    url: ATTACHMENT_API.uploadPrepare,
    method: 'post',
    data: payload,
    errorMode: options?.errorMode
  });
}

export function prepareAttachmentDownload(
  id: string,
  options?: { variant?: Api.Attachment.DownloadVariant } & RequestErrorOptions
) {
  return request<Api.Attachment.TransferPrepare>({
    url: ATTACHMENT_API.downloadPrepare,
    method: 'post',
    data: {
      id,
      variant: options?.variant || 'original'
    },
    errorMode: options?.errorMode
  });
}

export function bindAttachment(
  payload: {
    attachment_id: string;
    resource_type: string;
    resource_id: string;
    field?: string;
  },
  options?: RequestErrorOptions
) {
  return request<Api.Attachment.BindResult>({
    url: ATTACHMENT_API.bind,
    method: 'post',
    data: {
      attachment_id: payload.attachment_id,
      resource_type: payload.resource_type,
      resource_id: payload.resource_id,
      field: payload.field || ''
    },
    errorMode: options?.errorMode
  });
}

export function unbindAttachment(payload: {
  attachment_id: string;
  resource_type: string;
  resource_id: string;
  field?: string;
}) {
  return request<Api.Attachment.UnbindResult>({
    url: ATTACHMENT_API.unbind,
    method: 'post',
    data: {
      attachment_id: payload.attachment_id,
      resource_type: payload.resource_type,
      resource_id: payload.resource_id,
      field: payload.field || ''
    }
  });
}

export function listAttachmentsByResource(payload: { resource_type: string; resource_id: string; field?: string }) {
  return request<Api.Attachment.ListResult>({
    url: ATTACHMENT_API.listByResource,
    method: 'post',
    data: {
      resource_type: payload.resource_type,
      resource_id: payload.resource_id,
      field: payload.field || ''
    }
  });
}

export function deleteAttachment(id: string) {
  return request<Api.Attachment.DeleteResult>({
    url: ATTACHMENT_API.delete,
    method: 'post',
    data: { id }
  });
}

export function fetchAttachmentList(params: Api.Attachment.ListParams) {
  return request<Api.Attachment.PageResult>({
    url: ATTACHMENT_API.list,
    method: 'post',
    data: params
  });
}

export function fetchMyAttachmentList(params: Omit<Api.Attachment.ListParams, 'scope' | 'uploader_id'>) {
  return request<Api.Attachment.PageResult>({
    url: '/api/v1/system/attachments/mine/list',
    method: 'post',
    data: params
  });
}

export function fetchAttachment(id: string) {
  return request<Api.Attachment.Detail>({
    url: ATTACHMENT_API.get,
    method: 'post',
    data: { id }
  });
}

export function fetchAttachmentPolicy(options?: RequestErrorOptions) {
  return request<Api.Attachment.UploadPolicy>({
    url: ATTACHMENT_API.policy,
    method: 'post',
    data: {},
    errorMode: options?.errorMode
  });
}
