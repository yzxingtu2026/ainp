import { create, type AxiosProgressEvent } from 'axios';
import { getServiceBaseURL } from '@/utils/service';
import { localStg } from '@/utils/storage';

type TransferEnvelope<T = unknown> = {
  code: number | string;
  message?: string;
  message_key?: string;
  data: T;
};

function transferBaseURL(): string {
  const isHttpProxy = import.meta.env.DEV && import.meta.env.VITE_HTTP_PROXY === 'Y';
  return getServiceBaseURL(import.meta.env, isHttpProxy).baseURL;
}

function transferHeaders(transferToken: string): Record<string, string> {
  return {
    Authorization: `Bearer ${transferToken}`,
    'Accept-Language': localStg.get('lang') || 'zh-CN'
  };
}

function envelopeError(payload?: TransferEnvelope | null): Error {
  return new Error(payload?.message || payload?.message_key || 'transfer failed');
}

/** 流式上传字节；使用短时 transfer token，禁止把 token 写入日志/存储。 */
export async function putAttachmentTransfer(options: {
  uploadUrl: string;
  transferToken: string;
  file: Blob;
  contentType?: string;
  signal?: AbortSignal;
  onProgress?: (loaded: number, total: number) => void;
}): Promise<Api.Attachment.Item> {
  const client = create({ baseURL: transferBaseURL() });
  const response = await client.put<TransferEnvelope<{ attachment: Api.Attachment.Item }>>(
    options.uploadUrl,
    options.file,
    {
      headers: {
        ...transferHeaders(options.transferToken),
        'Content-Type': options.contentType || options.file.type || 'application/octet-stream'
      },
      signal: options.signal,
      onUploadProgress(event: AxiosProgressEvent) {
        if (!options.onProgress) return;
        const total = event.total || options.file.size || 0;
        options.onProgress(event.loaded, total);
      }
    }
  );
  if (String(response.data.code) !== String(import.meta.env.VITE_SERVICE_SUCCESS_CODE)) {
    throw envelopeError(response.data);
  }
  return response.data.data.attachment;
}

/** 使用短时 download token 拉取二进制；调用方负责用后释放 Object URL。 */
export async function getAttachmentTransferBlob(options: {
  downloadUrl: string;
  transferToken: string;
  signal?: AbortSignal;
}): Promise<Blob> {
  const client = create({ baseURL: transferBaseURL() });
  const response = await client.get(options.downloadUrl, {
    headers: transferHeaders(options.transferToken),
    responseType: 'blob',
    signal: options.signal
  });

  const contentType = String(response.headers['content-type'] || '');
  if (contentType.includes('application/json')) {
    const text = await (response.data as Blob).text();
    let payload: TransferEnvelope | null = null;
    try {
      payload = JSON.parse(text) as TransferEnvelope;
    } catch {
      payload = null;
    }
    throw envelopeError(payload);
  }
  return response.data as Blob;
}
