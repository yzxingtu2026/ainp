<script setup lang="tsx">
import { computed, h, onMounted, reactive, ref, watch } from 'vue';
import type { DataTableColumns } from 'naive-ui';
import { NButton, NSpace } from 'naive-ui';
import CodeTag from '@/components/common/code-tag.vue';
import { CODESET_KEYS } from '@/constants/codeset';
import { SYSTEM_ACTIONS } from '@/constants/system';
import { $t } from '@/locales';
import { formatDateTime } from '@/utils/datetime';
import {
  downloadAttachmentById,
  formatFileSize,
  isImageContentType,
  isVideoContentType,
  triggerBrowserDownload,
  useAttachmentPreview
} from '@/hooks/business/attachment';
import { deleteAttachment, fetchAttachment, fetchAttachmentList } from '@/service/api';
import { useAuthStore } from '@/store/modules/auth';
import { pageAfterDelete } from '@/hooks/common/table';

defineOptions({ name: 'SystemAttachments' });

const loading = ref(false);
const authStore = useAuthStore();
const records = ref<Api.Attachment.Item[]>([]);
const total = ref(0);
const query = reactive<{
  page: number;
  page_size: number;
  keyword: string;
  status: string | null;
  scope: 'all';
}>({
  page: 1,
  page_size: 20,
  keyword: '',
  status: null,
  scope: 'all'
});
const detailVisible = ref(false);
const detail = ref<Api.Attachment.Detail | null>(null);

const {
  objectUrl: previewUrl,
  previewType,
  loading: previewLoading,
  error: previewError,
  open: openPreview,
  close: closePreview
} = useAttachmentPreview();

const canPreviewMedia = computed(() => {
  if (!detail.value || detail.value.status !== 'ready') return false;
  return isImageContentType(detail.value.content_type) || isVideoContentType(detail.value.content_type);
});

watch(detailVisible, visible => {
  if (!visible) {
    closePreview();
    detail.value = null;
  }
});

function can(permission: string) {
  return authStore.userInfo.buttons.includes(permission) || authStore.userInfo.buttons.includes('*');
}

function uploaderLabel(row: Pick<Api.Attachment.Item, 'uploader_id' | 'uploader_name'>) {
  return row.uploader_name || row.uploader_id || '-';
}

const columns = computed<DataTableColumns<Api.Attachment.Item>>(() => [
  { title: $t('page.attachment.fileName'), key: 'original_name', minWidth: 180, ellipsis: { tooltip: true } },
  { title: $t('page.attachment.contentType'), key: 'content_type', width: 140 },
  {
    title: $t('page.attachment.size'),
    key: 'size',
    width: 100,
    render: row => formatFileSize(row.size)
  },
  {
    title: $t('page.attachment.statusLabel'),
    key: 'status',
    width: 110,
    render: row => h(CodeTag, { codeSet: CODESET_KEYS.attachmentStatus, value: row.status })
  },
  {
    title: $t('page.attachment.uploader'),
    key: 'uploader_name',
    width: 160,
    ellipsis: { tooltip: true },
    render: row => uploaderLabel(row)
  },
  {
    title: $t('page.attachment.createdAt'),
    key: 'created_at',
    width: 170,
    render: row => formatDateTime(row.created_at)
  },
  {
    title: $t('common.action'),
    key: 'actions',
    width: 220,
    fixed: 'right',
    render: row =>
      h(NSpace, null, {
        default: () =>
          [
            h(NButton, { size: 'small', onClick: () => openDetail(row.id) }, { default: () => $t('common.detail') }),
            row.status === 'ready'
              ? h(
                  NButton,
                  { size: 'small', onClick: () => handleDownload(row) },
                  { default: () => $t('page.attachment.download') }
                )
              : null,
            can(SYSTEM_ACTIONS.attachmentDelete)
              ? h(
                  NButton,
                  { size: 'small', type: 'error', secondary: true, onClick: () => handleDelete(row) },
                  { default: () => $t('common.delete') }
                )
              : null
          ].filter(Boolean)
      })
  }
]);

async function loadData() {
  loading.value = true;
  try {
    const { data, error } = await fetchAttachmentList({
      page: query.page,
      page_size: query.page_size,
      keyword: query.keyword?.trim() || undefined,
      status: query.status || undefined,
      scope: 'all'
    });
    if (error) {
      window.$message?.error(error.message || $t('page.attachment.loadFailed'));
      records.value = [];
      total.value = 0;
      return;
    }
    records.value = Array.isArray(data?.items) ? data.items : [];
    total.value = typeof data?.total === 'number' ? data.total : 0;
  } finally {
    loading.value = false;
  }
}

async function openDetail(id: string) {
  closePreview();
  const { data, error } = await fetchAttachment(id);
  if (error || !data) {
    window.$message?.error(error?.message || $t('page.attachment.loadFailed'));
    return;
  }
  detail.value = data;
  detailVisible.value = true;
  if (
    data.status === 'ready' &&
    (isImageContentType(data.content_type) || isVideoContentType(data.content_type))
  ) {
    await openPreview(data.id);
  }
}

async function handleDownload(row: Api.Attachment.Item) {
  try {
    const result = await downloadAttachmentById(row.id);
    triggerBrowserDownload(result.blob, result.name);
  } catch (err) {
    window.$message?.error(err instanceof Error ? err.message : $t('page.attachment.downloadFailed'));
  }
}

function handleDelete(row: Api.Attachment.Item) {
  window.$dialog?.warning({
    title: $t('common.warning'),
    content: $t('page.attachment.confirmDelete'),
    positiveText: $t('common.confirm'),
    negativeText: $t('common.cancel'),
    onPositiveClick: async () => {
      const { error } = await deleteAttachment(row.id);
      if (error) {
        window.$message?.error(error.message || $t('page.attachment.deleteFailed'));
        return;
      }
      query.page = pageAfterDelete(query.page, records.value.length);
      await loadData();
    }
  });
}

onMounted(loadData);
</script>

<template>
  <div class="min-h-500px flex-col-stretch gap-16px overflow-hidden lt-sm:overflow-auto">
    <NCard :title="$t('page.attachment.centerTitle')" :bordered="false" class="card-wrapper">
      <NSpace class="mb-16px" align="center">
        <NInput
          v-model:value="query.keyword"
          clearable
          class="w-220px"
          :placeholder="$t('page.attachment.searchName')"
          @keyup.enter="loadData"
        />
        <CodeSelect
          v-model:value="query.status"
          class="w-160px"
          :code-set="CODESET_KEYS.attachmentStatus"
          :placeholder="$t('page.attachment.statusAll')"
        />
        <NButton type="primary" @click="query.page = 1; loadData()">{{ $t('common.search') }}</NButton>
      </NSpace>
      <NDataTable
        :columns="columns"
        :data="records"
        :loading="loading"
        :row-key="(row: Api.Attachment.Item) => row.id"
        :scroll-x="1100"
      />
      <div class="mt-16px flex justify-end">
        <NPagination
          v-model:page="query.page"
          v-model:page-size="query.page_size"
          :item-count="total"
          show-size-picker
          :page-sizes="[10, 20, 50]"
          @update:page="loadData"
          @update:page-size="
            () => {
              query.page = 1;
              loadData();
            }
          "
        />
      </div>
    </NCard>

    <NDrawer v-model:show="detailVisible" :width="520">
      <NDrawerContent :title="$t('page.attachment.detailTitle')" closable>
        <template v-if="detail">
          <div v-if="canPreviewMedia" class="mb-16px">
            <div class="mb-8px text-14px font-medium">{{ $t('page.attachment.preview') }}</div>
            <div
              class="min-h-180px flex-col-center overflow-hidden rounded-8px border border-gray-200 bg-gray-50 dark:border-gray-700 dark:bg-gray-800"
            >
              <NSpin :show="previewLoading" class="w-full">
                <div class="w-full flex-col-center px-12px py-16px">
                  <img
                    v-if="previewUrl && isImageContentType(previewType || detail.content_type)"
                    :src="previewUrl"
                    :alt="detail.original_name"
                    class="max-h-360px max-w-full object-contain"
                  />
                  <video
                    v-else-if="previewUrl && isVideoContentType(previewType || detail.content_type)"
                    :src="previewUrl"
                    class="max-h-360px max-w-full"
                    controls
                    preload="metadata"
                  />
                  <NEmpty
                    v-else-if="previewError"
                    class="py-24px"
                    :description="$t('page.attachment.previewFailed')"
                  />
                  <NEmpty
                    v-else-if="!previewLoading"
                    class="py-24px"
                    :description="$t('page.attachment.previewUnavailable')"
                  />
                </div>
              </NSpin>
            </div>
          </div>

          <NDescriptions label-placement="left" :column="1" bordered size="small">
            <NDescriptionsItem :label="$t('page.attachment.fileName')">{{ detail.original_name }}</NDescriptionsItem>
            <NDescriptionsItem :label="$t('page.attachment.contentType')">{{ detail.content_type }}</NDescriptionsItem>
            <NDescriptionsItem :label="$t('page.attachment.size')">{{ formatFileSize(detail.size) }}</NDescriptionsItem>
            <NDescriptionsItem :label="$t('page.attachment.statusLabel')">
              <CodeTag :code-set="CODESET_KEYS.attachmentStatus" :value="detail.status" />
            </NDescriptionsItem>
            <NDescriptionsItem :label="$t('page.attachment.uploader')">{{ uploaderLabel(detail) }}</NDescriptionsItem>
            <NDescriptionsItem :label="$t('page.attachment.createdAt')">{{ formatDateTime(detail.created_at) }}</NDescriptionsItem>
          </NDescriptions>
          <div class="mt-16px text-14px font-medium">{{ $t('page.attachment.links') }}</div>
          <NEmpty v-if="!detail.links?.length" class="mt-8px" :description="$t('page.attachment.noLinks')" />
          <div v-else class="mt-8px flex-col gap-8px">
            <div v-for="link in detail.links" :key="link.id" class="rounded-8px border border-gray-200 px-12px py-8px text-12px dark:border-gray-700">
              {{ link.resource_type }} / {{ link.resource_id }}
              <span v-if="link.field"> · {{ link.field }}</span>
            </div>
          </div>
        </template>
      </NDrawerContent>
    </NDrawer>
  </div>
</template>
