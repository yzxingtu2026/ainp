import { computed, onBeforeUnmount, ref } from 'vue';
import { prepareAttachmentDownload } from '@/service/api';
import { getAttachmentTransferBlob } from '@/service/attachment/transfer';

/** 下载准备 → 短时 URL 拉流；不缓存 token。 */
export async function downloadAttachmentById(id: string, signal?: AbortSignal): Promise<{ blob: Blob; name: string; contentType: string }> {
  const { data, error } = await prepareAttachmentDownload(id);
  if (error || !data) {
    throw new Error(error?.message || 'download_prepare_failed');
  }
  const transferToken = data.token;
  const downloadUrl = data.download_url || `/api/v1/attachments/transfer/${data.attachment.id}`;
  const blob = await getAttachmentTransferBlob({ downloadUrl, transferToken, signal });
  return {
    blob,
    name: data.attachment.original_name,
    contentType: data.attachment.content_type
  };
}

export function triggerBrowserDownload(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = filename || 'download';
  anchor.click();
  URL.revokeObjectURL(url);
}

export function useAttachmentPreview() {
  const objectUrl = ref<string | null>(null);
  const previewName = ref('');
  const previewType = ref('');
  const loading = ref(false);
  const error = ref('');
  let abort: AbortController | null = null;

  const visible = computed(() => Boolean(objectUrl.value));

  async function open(attachmentId: string) {
    close();
    loading.value = true;
    error.value = '';
    abort = new AbortController();
    try {
      const result = await downloadAttachmentById(attachmentId, abort.signal);
      objectUrl.value = URL.createObjectURL(result.blob);
      previewName.value = result.name;
      previewType.value = result.contentType;
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'preview_failed';
    } finally {
      loading.value = false;
      abort = null;
    }
  }

  function close() {
    abort?.abort();
    abort = null;
    if (objectUrl.value) {
      URL.revokeObjectURL(objectUrl.value);
      objectUrl.value = null;
    }
    previewName.value = '';
    previewType.value = '';
    error.value = '';
  }

  onBeforeUnmount(close);

  return { objectUrl, previewName, previewType, loading, error, visible, open, close };
}

export function isImageContentType(contentType: string): boolean {
  return contentType.toLowerCase().startsWith('image/');
}

export function formatFileSize(size: number): string {
  if (size < 1024) return `${size} B`;
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`;
  return `${(size / (1024 * 1024)).toFixed(1)} MB`;
}
