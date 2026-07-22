import { onBeforeUnmount, readonly, ref, watch, type Ref } from 'vue';
import { downloadAttachmentById } from './attachment';

type AvatarStatus = 'idle' | 'loading' | 'ready' | 'error';

interface AvatarEntry {
  url: Ref<string | null>;
  status: Ref<AvatarStatus>;
  refs: number;
  abort?: AbortController;
}

const entries = new Map<string, AvatarEntry>();
const queue: string[] = [];
const concurrency = 4;
let active = 0;

function release(id: string) {
  const entry = entries.get(id);
  if (!entry) return;
  entry.refs -= 1;
  if (entry.refs > 0) return;
  entry.abort?.abort();
  if (entry.url.value) URL.revokeObjectURL(entry.url.value);
  entries.delete(id);
}

function pump() {
  while (active < concurrency && queue.length) {
    const id = queue.shift()!;
    const entry = entries.get(id);
    if (!entry || entry.status.value !== 'idle') continue;
    active += 1;
    entry.status.value = 'loading';
    entry.abort = new AbortController();
    downloadAttachmentById(id, { variant: 'preview', signal: entry.abort.signal })
      .then(result => {
        if (!entries.has(id) || entry.abort?.signal.aborted) return;
        entry.url.value = URL.createObjectURL(result.blob);
        entry.status.value = 'ready';
      })
      .catch(() => {
        if (entries.has(id) && !entry.abort?.signal.aborted) entry.status.value = 'error';
      })
      .finally(() => {
        entry.abort = undefined;
        active -= 1;
        pump();
      });
  }
}

function acquire(id: string) {
  let entry = entries.get(id);
  if (!entry) {
    entry = { url: ref(null), status: ref('idle'), refs: 0 };
    entries.set(id, entry);
    queue.push(id);
    pump();
  }
  entry.refs += 1;
  return entry;
}

/** Shared Agent avatar loader: deduplicates IDs, limits concurrency and revokes URLs after the last consumer leaves. */
export function useAgentAvatar(attachmentId: Ref<string | null | undefined>) {
  const url = ref<string | null>(null);
  const status = ref<AvatarStatus>('idle');
  let currentId = '';
  let stopMirroring: (() => void) | undefined;

  function detach() {
    stopMirroring?.();
    stopMirroring = undefined;
    if (currentId) release(currentId);
    currentId = '';
    url.value = null;
    status.value = 'idle';
  }

  watch(
    attachmentId,
    id => {
      detach();
      if (!id) return;
      currentId = id;
      const entry = acquire(id);
      stopMirroring = watch(
        [entry.url, entry.status],
        ([nextUrl, nextStatus]) => {
          url.value = nextUrl;
          status.value = nextStatus;
        },
        { immediate: true }
      );
    },
    { immediate: true }
  );

  onBeforeUnmount(detach);
  return { url: readonly(url), status: readonly(status) };
}
