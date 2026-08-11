import { computed, onBeforeUnmount, reactive, ref, toValue } from 'vue';
import type { MaybeRefOrGetter } from 'vue';
import { DEFAULT_ATTACHMENT_UPLOAD_POLICY, type AttachmentUploadItemStatus } from '@/constants/attachment';
import { prepareAttachmentUpload } from '@/service/api';
import { putAttachmentTransfer } from '@/service/attachment/transfer';
import { $t } from '@/locales';
import { getRequestErrorMessage, getStandaloneErrorMessage } from '@/service/request';

export interface AttachmentQueueItem {
  localId: string;
  file: File;
  status: AttachmentUploadItemStatus;
  progress: number;
  error: string;
  attachment: Api.Attachment.Item | null;
  abortController: AbortController | null;
}

export interface UseAttachmentUploadQueueOptions {
  concurrency?: number;
  maxCount?: MaybeRefOrGetter<number | undefined>;
  maxSizeBytes?: MaybeRefOrGetter<number | undefined>;
  allowedExt?: MaybeRefOrGetter<readonly string[] | undefined>;
  allowedMime?: MaybeRefOrGetter<readonly string[] | undefined>;
  onSuccess?: (item: AttachmentQueueItem) => void | Promise<void>;
}

function extOf(name: string): string {
  const idx = name.lastIndexOf('.');
  return idx >= 0 ? name.slice(idx).toLowerCase() : '';
}

function createLocalId(): string {
  return `local-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`;
}

export function useAttachmentUploadQueue(options: UseAttachmentUploadQueueOptions = {}) {
  const items = ref<AttachmentQueueItem[]>([]);
  const running = ref(0);
  const concurrency = options.concurrency ?? DEFAULT_ATTACHMENT_UPLOAD_POLICY.concurrency;
  const maxSizeBytes = computed(() => toValue(options.maxSizeBytes) ?? DEFAULT_ATTACHMENT_UPLOAD_POLICY.maxSizeBytes);
  const allowedExt = computed(() => toValue(options.allowedExt) ?? DEFAULT_ATTACHMENT_UPLOAD_POLICY.allowedExt);
  const allowedMime = computed(() => toValue(options.allowedMime) ?? DEFAULT_ATTACHMENT_UPLOAD_POLICY.allowedMime);
  const maxCount = computed(() => toValue(options.maxCount) ?? DEFAULT_ATTACHMENT_UPLOAD_POLICY.maxUploadsOnce);

  const busy = computed(() =>
    items.value.some(item => ['queued', 'preparing', 'uploading', 'completing'].includes(item.status))
  );

  function validateFile(file: File): string | null {
    if (file.size <= 0) return 'empty';
    if (file.size > maxSizeBytes.value) return 'size';
    const ext = extOf(file.name);
    if (!allowedExt.value.map(v => v.toLowerCase()).includes(ext)) return 'ext';
    if (file.type && !allowedMime.value.map(v => v.toLowerCase()).includes(file.type.toLowerCase())) {
      // browser MIME 可能为空；有值时做预校验
      return 'mime';
    }
    return null;
  }

  function enqueue(files: FileList | File[]) {
    const list = Array.from(files);
    const remain = Math.max(0, maxCount.value - items.value.filter(i => i.status !== 'cancelled').length);
    const accepted = list.slice(0, remain);
    for (const file of accepted) {
      const reason = validateFile(file);
      const item = reactive<AttachmentQueueItem>({
        localId: createLocalId(),
        file,
        status: reason ? 'failed' : 'queued',
        progress: 0,
        error: reason || '',
        attachment: null,
        abortController: null
      });
      items.value.push(item);
    }
    void pump();
  }

  async function pump() {
    while (running.value < concurrency) {
      const next = items.value.find(item => item.status === 'queued');
      if (!next) break;
      running.value += 1;
      next.status = 'preparing';
      void runOne(next).finally(() => {
        running.value -= 1;
        void pump();
      });
    }
  }

  async function runOne(item: AttachmentQueueItem) {
    const controller = new AbortController();
    item.abortController = controller;
    try {
      const { data, error } = await prepareAttachmentUpload(
        {
          original_name: item.file.name,
          content_type: item.file.type || 'application/octet-stream',
          size: item.file.size
        },
        { errorMode: 'inline' }
      );
      if (controller.signal.aborted) {
        item.status = 'cancelled';
        return;
      }
      if (error || !data) {
        item.status = 'failed';
        item.error = getRequestErrorMessage(error, $t('request.failed'));
        return;
      }
      // 一次性 token：仅内存持有，不落盘、不打印
      const transferToken = data.token;
      const uploadUrl = data.upload_url || `/api/v1/attachments/transfer/${data.attachment.id}`;
      item.status = 'uploading';
      item.progress = 0;
      const uploaded = await putAttachmentTransfer({
        uploadUrl,
        transferToken,
        file: item.file,
        contentType: item.file.type || data.attachment.content_type,
        signal: controller.signal,
        onProgress(loaded, total) {
          item.progress = total > 0 ? Math.min(100, Math.round((loaded / total) * 100)) : 0;
        }
      });
      if (controller.signal.aborted) {
        item.status = 'cancelled';
        return;
      }
      item.status = 'completing';
      item.attachment = uploaded;
      item.progress = 100;
      item.status = 'success';
      item.error = '';
      if (options.onSuccess) {
        await options.onSuccess(item);
      }
    } catch (err) {
      if (controller.signal.aborted) {
        item.status = 'cancelled';
        return;
      }
      item.status = 'failed';
      item.error = getStandaloneErrorMessage(err, $t('request.failed'));
    } finally {
      item.abortController = null;
    }
  }

  function cancel(localId: string) {
    const item = items.value.find(i => i.localId === localId);
    if (!item) return;
    item.abortController?.abort();
    if (item.status === 'queued') {
      item.status = 'cancelled';
    }
  }

  function remove(localId: string) {
    cancel(localId);
    items.value = items.value.filter(i => i.localId !== localId);
  }

  function retry(localId: string) {
    const item = items.value.find(i => i.localId === localId);
    if (!item || (item.status !== 'failed' && item.status !== 'cancelled')) return;
    // 必须重新 prepare，禁止重放一次性 token
    const reason = validateFile(item.file);
    item.status = reason ? 'failed' : 'queued';
    item.progress = 0;
    item.error = reason || '';
    item.attachment = null;
    item.abortController = null;
    if (!reason) void pump();
  }

  function clearFinished() {
    items.value = items.value.filter(i => !['success', 'cancelled', 'failed'].includes(i.status));
  }

  function reset() {
    for (const item of items.value) {
      item.abortController?.abort();
    }
    items.value = [];
    running.value = 0;
  }

  onBeforeUnmount(() => {
    reset();
  });

  return {
    items,
    busy,
    enqueue,
    cancel,
    remove,
    retry,
    clearFinished,
    reset,
    maxSizeBytes,
    allowedExt,
    maxCount
  };
}
