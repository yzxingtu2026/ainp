import { request } from '../request';

const ATTACHMENT_API = {
  uploadPrepare: '/api/v1/system/attachments/upload/prepare',
  downloadPrepare: '/api/v1/system/attachments/download/prepare',
  bind: '/api/v1/system/attachments/bind',
  unbind: '/api/v1/system/attachments/unbind',
  listByResource: '/api/v1/system/attachments/list_by_resource',
  delete: '/api/v1/system/attachments/delete'
} as const;

export function prepareAttachmentUpload(payload: {
  original_name: string;
  content_type: string;
  size: number;
}) {
  return request<Api.Attachment.TransferPrepare>({
    url: ATTACHMENT_API.uploadPrepare,
    method: 'post',
    data: payload
  });
}

export function prepareAttachmentDownload(id: string) {
  return request<Api.Attachment.TransferPrepare>({
    url: ATTACHMENT_API.downloadPrepare,
    method: 'post',
    data: { id }
  });
}

export function bindAttachment(payload: {
  attachment_id: string;
  resource_type: string;
  resource_id: string;
  field?: string;
}) {
  return request<Api.Attachment.BindResult>({
    url: ATTACHMENT_API.bind,
    method: 'post',
    data: {
      attachment_id: payload.attachment_id,
      resource_type: payload.resource_type,
      resource_id: payload.resource_id,
      field: payload.field || ''
    }
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

export function listAttachmentsByResource(payload: {
  resource_type: string;
  resource_id: string;
  field?: string;
}) {
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
