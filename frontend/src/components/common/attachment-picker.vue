<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue';
import { Icon } from '@iconify/vue';
import { fetchAttachmentPolicy, fetchMyAttachmentList } from '@/service/api';
import {
  attachmentTypeIcon,
  formatFileSize,
  isImageContentType,
  isVideoContentType,
  useAttachmentGridThumbs
} from '@/hooks/business/attachment';
import { DEFAULT_ATTACHMENT_UPLOAD_POLICY } from '@/constants/attachment';
import { $t } from '@/locales';
import { useAttachmentThumbStatusPolling } from '@/hooks/business/attachment-thumb-poll';

defineOptions({ name: 'AttachmentPicker' });

/** 首屏约 6 行 × 4 列；触底追加 */
const PAGE_SIZE = 24;
/** Picker 软顶：再多请去附件中心或加强搜索 */
const MAX_LOADED = 200;
const SCROLL_THRESHOLD_PX = 80;

const props = withDefaults(
  defineProps<{
    show: boolean;
    multiple?: boolean;
    maxCount?: number;
    mode?: 'replace' | 'append';
    selectedIds?: string[];
    lockedSelectedIds?: string[];
  }>(),
  {
    multiple: true,
    maxCount: 10,
    mode: 'replace',
    selectedIds: () => [],
    lockedSelectedIds: () => []
  }
);

const emit = defineEmits<{
  'update:show': [value: boolean];
  confirm: [items: Api.Attachment.Item[]];
}>();

const tab = ref<'mine' | 'upload'>('mine');
const loading = ref(false);
const loadingMore = ref(false);
const keyword = ref('');
const records = ref<Api.Attachment.Item[]>([]);
const checked = ref<string[]>([]);
const policy = ref<Api.Attachment.UploadPolicy | null>(null);
const policyLoading = ref(false);
const policyLoadFailed = ref(false);
const page = ref(1);
const total = ref(0);
const scrollEl = ref<HTMLElement | null>(null);
/** 本批上传成功的 id，整批 idle 后再切 tab 并刷新列表 */
const pendingSelectIds = ref<string[]>([]);

const gridEnabled = computed(() => props.show && tab.value === 'mine');
const limit = computed(() =>
  Math.min(props.maxCount, policy.value?.max_uploads_once ?? DEFAULT_ATTACHMENT_UPLOAD_POLICY.maxUploadsOnce)
);
const hasMore = computed(() => records.value.length < total.value && records.value.length < MAX_LOADED);
const reachedCap = computed(() => records.value.length >= MAX_LOADED && total.value > MAX_LOADED);
const lockedIds = computed(() => (props.mode === 'append' ? new Set(props.lockedSelectedIds) : new Set<string>()));
const lockedCount = computed(() => lockedIds.value.size);
const selectedCount = computed(() => checked.value.length);
const totalSelectedCount = computed(() => lockedCount.value + selectedCount.value);
const confirmLabel = computed(() =>
  $t(props.mode === 'append' ? 'page.attachment.pickerAssociate' : 'page.attachment.pickerConfirm')
);

const thumbs = useAttachmentGridThumbs(records, { concurrency: 3, enabled: gridEnabled });
useAttachmentThumbStatusPolling(records, { enabled: gridEnabled, keyword, maxItems: MAX_LOADED });

watch(
  () => props.show,
  async visible => {
    if (!visible) {
      thumbs.clear();
      pendingSelectIds.value = [];
      return;
    }
    checked.value = props.mode === 'append' ? [] : [...new Set(props.selectedIds)];
    tab.value = 'mine';
    keyword.value = '';
    pendingSelectIds.value = [];
    policy.value = null;
    policyLoadFailed.value = false;
    await Promise.all([loadPolicy(), resetAndLoadMine()]);
  }
);

async function loadPolicy() {
  policyLoading.value = true;
  policyLoadFailed.value = false;
  try {
    const { data, error } = await fetchAttachmentPolicy({ errorMode: 'inline' });
    if (error || !data) {
      policy.value = null;
      policyLoadFailed.value = true;
      return;
    }
    policy.value = data;
  } catch {
    policy.value = null;
    policyLoadFailed.value = true;
  } finally {
    policyLoading.value = false;
  }
}

async function resetAndLoadMine() {
  page.value = 1;
  total.value = 0;
  records.value = [];
  await loadMine({ reset: true });
  await nextTick();
  if (scrollEl.value) scrollEl.value.scrollTop = 0;
}

async function loadMine(options?: { reset?: boolean }) {
  const reset = Boolean(options?.reset);
  if (reset) {
    if (loading.value) return;
    loading.value = true;
  } else {
    if (loadingMore.value || loading.value || !hasMore.value) return;
    loadingMore.value = true;
  }

  const requestPage = reset ? 1 : page.value + 1;

  try {
    const { data, error } = await fetchMyAttachmentList({
      page: requestPage,
      page_size: PAGE_SIZE,
      keyword: keyword.value.trim() || undefined,
      status: 'ready'
    });
    if (error) {
      return;
    }
    const next = data?.items || [];
    total.value = typeof data?.total === 'number' ? data.total : 0;
    page.value = requestPage;
    if (reset) {
      records.value = next;
    } else {
      const seen = new Set(records.value.map(item => item.id));
      records.value = [...records.value, ...next.filter(item => !seen.has(item.id))];
    }
  } finally {
    loading.value = false;
    loadingMore.value = false;
  }
}

function onGridScroll(event: Event) {
  const el = event.target as HTMLElement | null;
  if (!el || loadingMore.value || loading.value || !hasMore.value) return;
  if (el.scrollHeight - el.scrollTop - el.clientHeight <= SCROLL_THRESHOLD_PX) {
    void loadMine({ reset: false });
  }
}

function toggle(id: string) {
  if (lockedIds.value.has(id)) return;
  if (props.multiple) {
    if (checked.value.includes(id)) {
      checked.value = checked.value.filter(item => item !== id);
      return;
    }
    if (totalSelectedCount.value >= limit.value) {
      window.$message?.warning($t('page.attachment.maxCountReached'));
      return;
    }
    checked.value = [...checked.value, id];
    return;
  }
  if (lockedCount.value >= limit.value) {
    window.$message?.warning($t('page.attachment.maxCountReached'));
    return;
  }
  checked.value = [id];
}

function isSelected(id: string) {
  return lockedIds.value.has(id) || checked.value.includes(id);
}

function onUploaded(attachment: Api.Attachment.Item) {
  // 单文件成功时先记入待选；整批 idle 后再切「我的上传」并刷新，避免销毁上传面板中断其余任务
  if (attachment.status === 'ready' && !pendingSelectIds.value.includes(attachment.id)) {
    pendingSelectIds.value = [...pendingSelectIds.value, attachment.id];
  }
}

async function onUploadIdle() {
  if (!pendingSelectIds.value.length) return;
  const ids = [...pendingSelectIds.value];
  pendingSelectIds.value = [];
  tab.value = 'mine';
  await resetAndLoadMine();
  for (const id of ids) {
    if (!checked.value.includes(id)) {
      if (!props.multiple) {
        checked.value = [id];
        break;
      }
      if (totalSelectedCount.value >= limit.value) {
        window.$message?.warning($t('page.attachment.maxCountReached'));
        break;
      }
      checked.value = [...checked.value, id];
    }
  }
}

function close() {
  emit('update:show', false);
}

function confirm() {
  const selected = records.value.filter(item => checked.value.includes(item.id));
  emit('confirm', selected);
  close();
}
</script>

<template>
  <NModal
    :show="show"
    preset="card"
    :title="$t('page.attachment.pickerTitle')"
    class="w-920px max-w-95vw"
    :mask-closable="false"
    @update:show="(v: boolean) => emit('update:show', v)"
  >
    <NTabs v-model:value="tab" type="line">
      <NTabPane name="mine" :tab="$t('page.attachment.pickerMine')" display-directive="show">
        <NSpace vertical>
          <NInput
            v-model:value="keyword"
            clearable
            :placeholder="$t('page.attachment.searchName')"
            @keyup.enter="resetAndLoadMine"
            @clear="resetAndLoadMine"
          >
            <template #suffix>
              <ButtonIcon icon="mdi:magnify" :tooltip-content="$t('common.search')" @click="resetAndLoadMine" />
            </template>
          </NInput>
          <NSpin :show="loading">
            <NEmpty v-if="!loading && !records.length" :description="$t('page.attachment.empty')" />
            <div
              v-else-if="records.length"
              ref="scrollEl"
              class="max-h-480px overflow-auto"
              @scroll.passive="onGridScroll"
            >
              <div class="grid grid-cols-4 gap-10px lt-sm:grid-cols-2">
                <button
                  v-for="item in records"
                  :key="item.id"
                  type="button"
                  class="relative flex-col items-stretch overflow-hidden rounded-8px border text-left transition-colors"
                  :class="[
                    lockedIds.has(item.id)
                      ? 'cursor-not-allowed border-gray-300 bg-gray-50 dark:border-gray-600 dark:bg-gray-800'
                      : checked.includes(item.id)
                        ? 'border-primary bg-primary/5 ring-1 ring-primary'
                        : 'border-gray-200 hover:border-primary/40 dark:border-gray-700'
                  ]"
                  :aria-disabled="lockedIds.has(item.id)"
                  @click="toggle(item.id)"
                >
                  <div class="relative h-96px w-full bg-gray-50 dark:bg-gray-800">
                    <img
                      v-if="thumbs.urls[item.id]"
                      :src="thumbs.urls[item.id]"
                      :alt="item.original_name"
                      class="h-full w-full object-cover"
                    />
                    <div v-else class="h-full w-full flex-col-center gap-6px text-gray-400">
                      <NSpin v-if="thumbs.loadingIds[item.id] || item.thumb_status === 'pending'" :size="18" />
                      <template v-else>
                        <Icon :icon="attachmentTypeIcon(item.content_type)" class="text-28px" />
                        <NButton
                          v-if="
                            thumbs.failedIds[item.id] &&
                              item.thumb_status === 'ready' &&
                              (isImageContentType(item.content_type) || isVideoContentType(item.content_type))
                          "
                          size="tiny"
                          quaternary
                          @click.stop="thumbs.retry(item.id)"
                        >
                          {{ $t('page.attachment.retry') }}
                        </NButton>
                      </template>
                    </div>
                    <div
                      class="absolute left-8px top-8px rounded-4px bg-white/90 px-4px py-2px dark:bg-gray-900/80"
                      @click.stop
                    >
                      <NCheckbox
                        :checked="isSelected(item.id)"
                        :disabled="lockedIds.has(item.id)"
                        @update:checked="() => toggle(item.id)"
                      />
                    </div>
                    <NTag v-if="lockedIds.has(item.id)" class="absolute right-8px top-8px" size="small" type="success">
                      {{ $t('page.attachment.alreadyAssociated') }}
                    </NTag>
                  </div>
                  <div class="min-w-0 bg-white px-10px py-8px dark:bg-gray-900">
                    <div
                      class="truncate text-12px text-gray-900 font-medium dark:text-gray-100"
                      :title="item.original_name"
                    >
                      {{ item.original_name }}
                    </div>
                    <div class="mt-2px text-11px text-gray-600 dark:text-gray-400">
                      {{ formatFileSize(item.size) }}
                    </div>
                  </div>
                </button>
              </div>
              <div class="py-12px text-center text-12px text-gray-500">
                <template v-if="loadingMore">{{ $t('page.attachment.loadingMore') }}</template>
                <template v-else-if="reachedCap">{{ $t('page.attachment.loadMoreCap') }}</template>
                <template v-else-if="!hasMore">{{ $t('page.attachment.noMore') }}</template>
              </div>
            </div>
          </NSpin>
        </NSpace>
      </NTabPane>
      <NTabPane name="upload" :tab="$t('page.attachment.pickerUpload')" display-directive="show">
        <NSpin :show="policyLoading">
          <AttachmentUpload
            v-if="policy"
            :multiple="multiple"
            :max-count="limit"
            :max-size-bytes="policy.max_size_bytes"
            :allowed-ext="policy.allowed_ext"
            :allowed-mime="policy.allowed_mime"
            @success="onUploaded"
            @idle="onUploadIdle"
          />
          <NEmpty v-else-if="policyLoadFailed" :description="$t('page.attachment.loadFailed')">
            <template #extra>
              <NButton size="small" @click="loadPolicy">{{ $t('page.attachment.retry') }}</NButton>
            </template>
          </NEmpty>
          <div v-else class="h-120px" />
        </NSpin>
      </NTabPane>
    </NTabs>
    <template #footer>
      <NSpace justify="end">
        <NButton @click="close">{{ $t('common.cancel') }}</NButton>
        <NButton type="primary" :disabled="selectedCount === 0" @click="confirm">
          {{ confirmLabel }} ({{ selectedCount }})
        </NButton>
      </NSpace>
    </template>
  </NModal>
</template>
