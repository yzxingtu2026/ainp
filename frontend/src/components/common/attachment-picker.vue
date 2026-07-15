<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue';
import { fetchAttachmentList, fetchAttachmentPolicy } from '@/service/api';
import { formatFileSize } from '@/hooks/business/attachment';
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

const limit = computed(() => props.maxCount || policy.value?.max_uploads_once || DEFAULT_ATTACHMENT_UPLOAD_POLICY.maxUploadsOnce);

watch(
  () => props.show,
  async visible => {
    if (!visible) return;
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
  // 刚上传但列表尚未含 ready 的，用 checked 对应 records；不足时仅返回已匹配项
  emit('confirm', selected);
  close();
}
</script>

<template>
  <NModal
    :show="show"
    preset="card"
    :title="$t('page.attachment.pickerTitle')"
    class="w-720px"
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
            <div v-else class="max-h-360px overflow-auto flex-col gap-8px">
              <div
                v-for="item in records"
                :key="item.id"
                class="flex items-center gap-12px rounded-8px border px-12px py-10px cursor-pointer"
                :class="checked.includes(item.id) ? 'border-primary bg-primary/5' : 'border-gray-200 dark:border-gray-700'"
                @click="toggle(item.id)"
              >
                <NCheckbox :checked="checked.includes(item.id)" @update:checked="() => toggle(item.id)" @click.stop />
                <div class="min-w-0 flex-1">
                  <div class="truncate text-13px font-medium">{{ item.original_name }}</div>
                  <div class="text-12px text-gray-500">{{ formatFileSize(item.size) }} · {{ item.content_type }}</div>
                </div>
              </div>
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
