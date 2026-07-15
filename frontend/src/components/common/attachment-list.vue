<script setup lang="ts">
import { ref } from 'vue';
import {
  downloadAttachmentById,
  formatFileSize,
  isImageContentType,
  triggerBrowserDownload,
  useAttachmentPreview
} from '@/hooks/business/attachment';
import { $t } from '@/locales';

defineOptions({ name: 'AttachmentList' });

withDefaults(
  defineProps<{
    items: Api.Attachment.Item[];
    loading?: boolean;
    readonly?: boolean;
    canUnbind?: boolean;
    canDelete?: boolean;
  }>(),
  {
    loading: false,
    readonly: false,
    canUnbind: true,
    canDelete: true
  }
);

const emit = defineEmits<{
  unbind: [item: Api.Attachment.Item];
  delete: [item: Api.Attachment.Item];
  refresh: [];
}>();

const preview = useAttachmentPreview();
const actionLoading = ref<string | null>(null);
const {
  objectUrl: previewUrl,
  previewName,
  error: previewError,
  visible: previewVisible,
  open: openPreview,
  close: closePreview
} = preview;

async function handleDownload(item: Api.Attachment.Item) {
  actionLoading.value = `dl-${item.id}`;
  try {
    const result = await downloadAttachmentById(item.id);
    triggerBrowserDownload(result.blob, result.name);
  } catch (err) {
    window.$message?.error(err instanceof Error ? err.message : $t('page.attachment.downloadFailed'));
  } finally {
    actionLoading.value = null;
  }
}

async function handlePreview(item: Api.Attachment.Item) {
  if (!isImageContentType(item.content_type)) {
    await handleDownload(item);
    return;
  }
  await openPreview(item.id);
  if (previewError.value) {
    window.$message?.error(previewError.value);
  }
}

function confirmUnbind(item: Api.Attachment.Item) {
  window.$dialog?.warning({
    title: $t('common.warning'),
    content: $t('page.attachment.confirmUnbind'),
    positiveText: $t('common.confirm'),
    negativeText: $t('common.cancel'),
    onPositiveClick: () => emit('unbind', item)
  });
}

function confirmDelete(item: Api.Attachment.Item) {
  window.$dialog?.warning({
    title: $t('common.warning'),
    content: $t('page.attachment.confirmDelete'),
    positiveText: $t('common.confirm'),
    negativeText: $t('common.cancel'),
    onPositiveClick: () => emit('delete', item)
  });
}
</script>

<template>
  <div class="flex-col gap-8px">
    <NSpin :show="loading">
      <NEmpty v-if="!items.length" :description="$t('page.attachment.empty')" />
      <div v-else class="flex-col gap-8px">
        <div
          v-for="item in items"
          :key="item.id"
          class="flex items-center gap-12px rounded-8px border border-gray-200 px-12px py-10px dark:border-gray-700"
        >
          <SvgIcon
            :icon="isImageContentType(item.content_type) ? 'mdi:file-image-outline' : 'mdi:file-outline'"
            class="text-22px shrink-0"
          />
          <div class="min-w-0 flex-1">
            <div class="truncate text-13px font-medium">{{ item.original_name }}</div>
            <div class="text-12px text-gray-500">
              {{ formatFileSize(item.size) }} · {{ item.content_type }} · {{ item.status }}
            </div>
          </div>
          <div class="flex shrink-0 gap-4px">
            <ButtonIcon
              v-if="isImageContentType(item.content_type)"
              icon="mdi:eye-outline"
              :tooltip-content="$t('page.attachment.preview')"
              :aria-label="$t('page.attachment.preview')"
              @click="handlePreview(item)"
            />
            <ButtonIcon
              icon="mdi:download-outline"
              :tooltip-content="$t('page.attachment.download')"
              :aria-label="$t('page.attachment.download')"
              :loading="actionLoading === `dl-${item.id}`"
              @click="handleDownload(item)"
            />
            <ButtonIcon
              v-if="!readonly && canUnbind"
              icon="mdi:link-off"
              :tooltip-content="$t('page.attachment.unbind')"
              :aria-label="$t('page.attachment.unbind')"
              @click="confirmUnbind(item)"
            />
            <ButtonIcon
              v-if="!readonly && canDelete"
              icon="mdi:trash-can-outline"
              :tooltip-content="$t('common.delete')"
              :aria-label="$t('common.delete')"
              @click="confirmDelete(item)"
            />
          </div>
        </div>
      </div>
    </NSpin>

    <NModal
      :show="previewVisible"
      preset="card"
      :title="previewName"
      class="w-640px"
      @update:show="(v: boolean) => !v && closePreview()"
    >
      <img
        v-if="previewUrl"
        :src="previewUrl"
        :alt="previewName"
        class="max-h-70vh mx-auto block"
      />
    </NModal>
  </div>
</template>
