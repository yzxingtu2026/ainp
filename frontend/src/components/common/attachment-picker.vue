<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { Icon } from '@iconify/vue';
import { fetchAttachmentList, fetchAttachmentPolicy } from '@/service/api';
import {
  attachmentTypeIcon,
  formatFileSize,
  isImageContentType,
  useAttachmentGridThumbs
} from '@/hooks/business/attachment';
import { DEFAULT_ATTACHMENT_UPLOAD_POLICY } from '@/constants/attachment';
import { $t } from '@/locales';

defineOptions({ name: 'AttachmentPicker' });

const props = withDefaults(
  defineProps<{
    show: boolean;
    multiple?: boolean;
    maxCount?: number;
    selectedIds?: string[];
  }>(),
  {
    multiple: true,
    maxCount: 10,
    selectedIds: () => []
  }
);

const emit = defineEmits<{
  'update:show': [value: boolean];
  confirm: [items: Api.Attachment.Item[]];
}>();

const tab = ref<'mine' | 'upload'>('mine');
const loading = ref(false);
const keyword = ref('');
const records = ref<Api.Attachment.Item[]>([]);
const checked = ref<string[]>([]);
const policy = ref<Api.Attachment.UploadPolicy | null>(null);
const gridEnabled = computed(() => props.show && tab.value === 'mine');

const limit = computed(() => props.maxCount || policy.value?.max_uploads_once || DEFAULT_ATTACHMENT_UPLOAD_POLICY.maxUploadsOnce);

const thumbs = useAttachmentGridThumbs(records, { concurrency: 3, enabled: gridEnabled });

watch(
  () => props.show,
  async visible => {
    if (!visible) {
      thumbs.clear();
      return;
    }
    checked.value = [...props.selectedIds];
    tab.value = 'mine';
    await Promise.all([loadPolicy(), loadMine()]);
  }
);

async function loadPolicy() {
  const { data } = await fetchAttachmentPolicy();
  if (data) policy.value = data;
}

async function loadMine() {
  loading.value = true;
  try {
    const { data, error } = await fetchAttachmentList({
      current: 1,
      size: 50,
      scope: 'mine',
      keyword: keyword.value.trim() || undefined,
      status: 'ready'
    });
    if (error) {
      window.$message?.error(error.message || $t('page.attachment.loadFailed'));
      return;
    }
    records.value = data?.records || [];
  } finally {
    loading.value = false;
  }
}

function toggle(id: string) {
  if (props.multiple) {
    if (checked.value.includes(id)) {
      checked.value = checked.value.filter(item => item !== id);
      return;
    }
    if (checked.value.length >= limit.value) {
      window.$message?.warning($t('page.attachment.maxCountReached'));
      return;
    }
    checked.value = [...checked.value, id];
    return;
  }
  checked.value = [id];
}

function onUploaded(attachment: Api.Attachment.Item) {
  if (attachment.status === 'ready' && !records.value.some(item => item.id === attachment.id)) {
    records.value = [attachment, ...records.value];
  }
  toggle(attachment.id);
  tab.value = 'mine';
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
      <NTabPane name="mine" :tab="$t('page.attachment.pickerMine')">
        <NSpace vertical>
          <NInput
            v-model:value="keyword"
            clearable
            :placeholder="$t('page.attachment.searchName')"
            @keyup.enter="loadMine"
          >
            <template #suffix>
              <ButtonIcon icon="mdi:magnify" :tooltip-content="$t('common.search')" @click="loadMine" />
            </template>
          </NInput>
          <NSpin :show="loading">
            <NEmpty v-if="!records.length" :description="$t('page.attachment.empty')" />
            <div v-else class="max-h-480px overflow-auto grid grid-cols-4 gap-10px lt-sm:grid-cols-2">
              <button
                v-for="item in records"
                :key="item.id"
                type="button"
                class="relative flex-col items-stretch overflow-hidden rounded-8px border text-left transition-colors"
                :class="
                  checked.includes(item.id)
                    ? 'border-primary bg-primary/5 ring-1 ring-primary'
                    : 'border-gray-200 hover:border-primary/40 dark:border-gray-700'
                "
                @click="toggle(item.id)"
              >
                <div class="relative h-96px w-full bg-gray-50 dark:bg-gray-800">
                  <img
                    v-if="thumbs.urls[item.id]"
                    :src="thumbs.urls[item.id]"
                    :alt="item.original_name"
                    class="h-full w-full object-cover"
                  />
                  <div
                    v-else
                    class="h-full w-full flex-col-center gap-6px text-gray-400"
                  >
                    <NSpin v-if="thumbs.loadingIds[item.id]" :size="18" />
                    <template v-else>
                      <Icon :icon="attachmentTypeIcon(item.content_type)" class="text-28px" />
                      <NButton
                        v-if="thumbs.failedIds[item.id] && isImageContentType(item.content_type)"
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
                    <NCheckbox :checked="checked.includes(item.id)" @update:checked="() => toggle(item.id)" />
                  </div>
                </div>
                <div class="min-w-0 px-10px py-8px">
                  <div class="truncate text-12px font-medium" :title="item.original_name">{{ item.original_name }}</div>
                  <div class="mt-2px text-11px text-gray-500">{{ formatFileSize(item.size) }}</div>
                </div>
              </button>
            </div>
          </NSpin>
        </NSpace>
      </NTabPane>
      <NTabPane name="upload" :tab="$t('page.attachment.pickerUpload')">
        <AttachmentUpload
          :multiple="multiple"
          :max-count="limit"
          :max-size-bytes="policy?.max_size_bytes"
          :allowed-ext="policy?.allowed_ext"
          :allowed-mime="policy?.allowed_mime"
          @success="onUploaded"
        />
      </NTabPane>
    </NTabs>
    <template #footer>
      <NSpace justify="end">
        <NButton @click="close">{{ $t('common.cancel') }}</NButton>
        <NButton type="primary" :disabled="!checked.length" @click="confirm">
          {{ $t('page.attachment.pickerConfirm') }} ({{ checked.length }})
        </NButton>
      </NSpace>
    </template>
  </NModal>
</template>
