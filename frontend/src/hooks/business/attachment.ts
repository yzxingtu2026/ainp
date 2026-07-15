import { computed, onBeforeUnmount, reactive, ref, watch, type Ref } from 'vue';
import { prepareAttachmentDownload } from '@/service/api';
import { getAttachmentTransferBlob } from '@/service/attachment/transfer';

export type AttachmentDownloadVariant = Api.Attachment.DownloadVariant;

/** 下载准备 → 短时 URL 拉流；不缓存 token。 */
export async function downloadAttachmentById(
  id: string,
  options?: { variant?: AttachmentDownloadVariant; signal?: AbortSignal }
): Promise<{ blob: Blob; name: string; contentType: string; variant: AttachmentDownloadVariant }> {
  const variant = options?.variant || 'original';
  const { data, error } = await prepareAttachmentDownload(id, { variant });
  if (error || !data) {
    throw new Error(error?.message || 'download_prepare_failed');
  }
  const transferToken = data.token;
  const downloadUrl = data.download_url || `/api/v1/attachments/transfer/${data.attachment.id}`;
  const blob = await getAttachmentTransferBlob({ downloadUrl, transferToken, signal: options?.signal });
  return {
    blob,
    name: data.attachment.original_name,
    contentType: data.attachment.content_type,
    variant
  };
}

/**
 * 宫格缩略图加载：仅在 thumb_status === 'ready' 时请求 variant=thumb。
 * 未就绪/失败/跳过时不拉原图（避免宫格撑爆带宽）；调用方显示类型图标占位。
 * 业务大图预览请用 downloadAttachmentById({ variant: 'original' })。
 */
export async function loadAttachmentPreviewBlob(
  id: string,
  options?: {
    preferThumb?: boolean;
    thumbStatus?: Api.Attachment.Item['thumb_status'];
    signal?: AbortSignal;
  }
): Promise<{ blob: Blob; name: string; contentType: string; variant: AttachmentDownloadVariant } | null> {
  const preferThumb = options?.preferThumb !== false;
  const canTryThumb = preferThumb && options?.thumbStatus === 'ready';

  if (!canTryThumb) {
    return null;
  }

  return downloadAttachmentById(id, { variant: 'thumb', signal: options?.signal });
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
      // 详情预览用原图；宫格另走 useAttachmentGridThumbs
      const result = await downloadAttachmentById(attachmentId, {
        variant: 'original',
        signal: abort.signal
      });
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

/** 宫格缩略图缓存：并发限制 + Object URL 回收。 */
export function useAttachmentGridThumbs(
  items: Ref<Api.Attachment.Item[]>,
  options?: { concurrency?: number; enabled?: Ref<boolean> }
) {
  const concurrency = options?.concurrency ?? 3;
  const urls = reactive<Record<string, string>>({});
  const loadingIds = reactive<Record<string, boolean>>({});
  const failedIds = reactive<Record<string, boolean>>({});
  const aborts = new Map<string, AbortController>();
  let queue: string[] = [];
  let active = 0;
  let disposed = false;

  function revokeAll() {
    for (const [, ctrl] of aborts) ctrl.abort();
    aborts.clear();
    queue = [];
    active = 0;
    Object.keys(urls).forEach(id => {
      URL.revokeObjectURL(urls[id]);
      delete urls[id];
    });
    Object.keys(loadingIds).forEach(id => delete loadingIds[id]);
    Object.keys(failedIds).forEach(id => delete failedIds[id]);
  }

  function pump() {
    if (disposed) return;
    while (active < concurrency && queue.length) {
      const id = queue.shift()!;
      const item = items.value.find(row => row.id === id);
      if (!item || urls[id] || failedIds[id]) continue;
      if (!isImageContentType(item.content_type) && !isVideoContentType(item.content_type)) continue;
      // 仅 thumb ready 时加载；pending/failed/skipped/缺省走占位
      if (item.thumb_status !== 'ready') continue;

      active += 1;
      loadingIds[id] = true;
      const ctrl = new AbortController();
      aborts.set(id, ctrl);

      loadAttachmentPreviewBlob(id, {
        preferThumb: true,
        thumbStatus: item.thumb_status,
        signal: ctrl.signal
      })
        .then(result => {
          if (disposed || ctrl.signal.aborted || !result) {
            return;
          }
          if (urls[id]) URL.revokeObjectURL(urls[id]);
          urls[id] = URL.createObjectURL(result.blob);
          delete failedIds[id];
        })
        .catch(() => {
          if (!ctrl.signal.aborted) failedIds[id] = true;
        })
        .finally(() => {
          aborts.delete(id);
          delete loadingIds[id];
          active -= 1;
          pump();
        });
    }
  }

  function enqueueVisible() {
    if (disposed) return;
    if (options?.enabled && !options.enabled.value) return;
    const wanted = new Set(
      items.value
        .filter(
          item =>
            (isImageContentType(item.content_type) || isVideoContentType(item.content_type)) &&
            item.thumb_status === 'ready'
        )
        .map(item => item.id)
    );
    // 回收已不在列表中的 URL
    Object.keys(urls).forEach(id => {
      if (!wanted.has(id)) {
        URL.revokeObjectURL(urls[id]);
        delete urls[id];
      }
    });
    for (const id of wanted) {
      if (urls[id] || loadingIds[id] || failedIds[id] || queue.includes(id) || aborts.has(id)) continue;
      queue.push(id);
    }
    pump();
  }

  function retry(id: string) {
    delete failedIds[id];
    if (urls[id]) {
      URL.revokeObjectURL(urls[id]);
      delete urls[id];
    }
    enqueueVisible();
  }

  watch(
    [items, () => options?.enabled?.value],
    () => enqueueVisible(),
    { immediate: true, deep: true }
  );

  onBeforeUnmount(() => {
    disposed = true;
    revokeAll();
  });

  return { urls, loadingIds, failedIds, retry, clear: revokeAll, refresh: enqueueVisible };
}

export function isImageContentType(contentType: string): boolean {
  return contentType.toLowerCase().startsWith('image/');
}

export function isVideoContentType(contentType: string): boolean {
  return contentType.toLowerCase().startsWith('video/');
}

export function attachmentTypeIcon(contentType: string): string {
  if (isImageContentType(contentType)) return 'mdi:image-outline';
  if (isVideoContentType(contentType)) return 'mdi:video-outline';
  if (contentType.toLowerCase().includes('pdf')) return 'mdi:file-pdf-box';
  if (contentType.toLowerCase().includes('zip')) return 'mdi:folder-zip-outline';
  return 'mdi:file-outline';
}

export function formatFileSize(size: number): string {
  if (size < 1024) return `${size} B`;
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`;
  return `${(size / (1024 * 1024)).toFixed(1)} MB`;
}
