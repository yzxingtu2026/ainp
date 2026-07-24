import { computed, onBeforeUnmount, watch, type Ref } from 'vue';
import { fetchMyAttachmentList } from '@/service/api';
import { mergePendingThumbnailStatuses, pendingThumbnailIds } from './attachment-thumb-status';

const DEFAULT_INTERVAL_MS = 2500;
const PAGE_SIZE = 100;

export function useAttachmentThumbStatusPolling(
  items: Ref<Api.Attachment.Item[]>,
  options: {
    enabled: Ref<boolean>;
    keyword?: Ref<string>;
    maxItems?: number;
    intervalMs?: number;
  }
) {
  const pendingCount = computed(() => pendingThumbnailIds(items.value).size);
  let timer: number | null = null;
  let inFlight = false;
  let disposed = false;

  function stop() {
    if (timer !== null) window.clearInterval(timer);
    timer = null;
  }

  async function refresh() {
    if (disposed || !options.enabled.value || inFlight) return;
    const pending = pendingThumbnailIds(items.value);
    if (pending.size === 0) {
      stop();
      return;
    }

    inFlight = true;
    try {
      const maxItems = Math.max(1, options.maxItems ?? items.value.length);
      const pageCount = Math.max(1, Math.ceil(Math.min(items.value.length, maxItems) / PAGE_SIZE));
      const responses = await Promise.all(
        Array.from({ length: pageCount }, (_, index) =>
          fetchMyAttachmentList({
            page: index + 1,
            page_size: PAGE_SIZE,
            keyword: options.keyword?.value.trim() || undefined,
            status: 'ready'
          })
        )
      );
      if (disposed || !options.enabled.value) return;
      const refreshed = responses.flatMap(response => response.data?.items || []).filter(item => pending.has(item.id));
      items.value = mergePendingThumbnailStatuses(items.value, refreshed);
    } catch {
      // Polling is best effort; the next interval retries without replacing current UI state.
    } finally {
      inFlight = false;
    }
  }

  function sync() {
    stop();
    if (disposed || !options.enabled.value || pendingCount.value === 0) return;
    void refresh();
    timer = window.setInterval(() => void refresh(), options.intervalMs ?? DEFAULT_INTERVAL_MS);
  }

  watch([options.enabled, pendingCount, () => options.keyword?.value], sync, { immediate: true });
  onBeforeUnmount(() => {
    disposed = true;
    stop();
  });

  return { pendingCount, refresh, stop };
}
