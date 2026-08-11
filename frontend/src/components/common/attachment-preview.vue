<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import {
  ATTACHMENT_PREVIEW_MAX_BLOB_SIZE_BYTES,
  downloadAttachmentById,
  formatFileSize,
  triggerBrowserDownload,
  useAttachmentPreview
} from '@/hooks/business/attachment';
import { $t } from '@/locales';
import { getStandaloneErrorMessage } from '@/service/request';

defineOptions({ name: 'AttachmentPreview' });

const props = withDefaults(
  defineProps<{
    show: boolean;
    attachment?: string | Api.Attachment.Item | null;
  }>(),
  {
    attachment: null
  }
);

const emit = defineEmits<{
  'update:show': [show: boolean];
}>();

const preview = useAttachmentPreview();
const downloadLoading = ref(false);
const attachmentKey = computed(() => {
  if (!props.attachment) return '';
  if (typeof props.attachment === 'string') return props.attachment;
  const item = props.attachment;
  return [item.id, item.status, item.content_type, item.size, item.updated_at].join(':');
});
const attachmentId = computed(() =>
  typeof props.attachment === 'string' ? props.attachment : props.attachment?.id || ''
);

watch(
  [() => props.show, attachmentKey],
  ([show]) => {
    if (show && props.attachment) {
      void preview.open(props.attachment);
      return;
    }
    preview.close();
  },
  { immediate: true }
);

function close() {
  preview.close();
  emit('update:show', false);
}

function openInNewWindow() {
  if (!preview.objectUrl.value) return;
  const opened = window.open(preview.objectUrl.value, '_blank', 'noopener,noreferrer');
  if (opened) opened.opener = null;
}

async function download() {
  if (!attachmentId.value || downloadLoading.value) return;
  downloadLoading.value = true;
  try {
    if (preview.previewBlob.value) {
      triggerBrowserDownload(preview.previewBlob.value, preview.previewName.value);
      return;
    }
    const result = await downloadAttachmentById(attachmentId.value);
    triggerBrowserDownload(result.blob, result.name);
  } catch (err) {
    window.$message?.error(getStandaloneErrorMessage(err, $t('page.attachment.downloadFailed')));
  } finally {
    downloadLoading.value = false;
  }
}
</script>

<template>
  <NModal
    :show="show"
    preset="card"
    :title="preview.previewName.value || $t('page.attachment.preview')"
    class="w-960px max-w-[calc(100vw-24px)]"
    :content-style="{ padding: '0' }"
    @update:show="(value: boolean) => !value && close()"
  >
    <template #header-extra>
      <div class="flex items-center gap-4px">
        <ButtonIcon
          v-if="preview.objectUrl.value"
          icon="mdi:open-in-new"
          :tooltip-content="$t('page.attachment.openInNewWindow')"
          :aria-label="$t('page.attachment.openInNewWindow')"
          @click="openInNewWindow"
        />
        <ButtonIcon
          icon="mdi:download-outline"
          :tooltip-content="$t('page.attachment.download')"
          :aria-label="$t('page.attachment.download')"
          :loading="downloadLoading"
          :disabled="!attachmentId"
          @click="download"
        />
        <ButtonIcon
          icon="mdi:close"
          :tooltip-content="$t('common.close')"
          :aria-label="$t('common.close')"
          @click="close"
        />
      </div>
    </template>

    <div class="min-h-320px flex-col-center bg-gray-50 p-12px dark:bg-gray-900 sm:min-h-480px">
      <NSpin v-if="preview.state.value === 'loading'" size="large" />

      <img
        v-else-if="preview.state.value === 'ready' && preview.previewKind.value === 'image'"
        :src="preview.objectUrl.value || ''"
        :alt="preview.previewName.value"
        class="max-h-[75vh] max-w-full object-contain"
      />
      <iframe
        v-else-if="preview.state.value === 'ready' && preview.previewKind.value === 'pdf'"
        :src="preview.objectUrl.value || ''"
        :title="preview.previewName.value"
        class="h-[75vh] min-h-480px w-full border-0"
        sandbox=""
        referrerpolicy="no-referrer"
      />
      <video
        v-else-if="preview.state.value === 'ready' && preview.previewKind.value === 'video'"
        :src="preview.objectUrl.value || ''"
        class="max-h-[75vh] max-w-full"
        controls
        preload="metadata"
      />

      <NAlert v-else-if="preview.state.value === 'too-large'" type="warning" class="max-w-560px">
        {{
          $t('page.attachment.previewTooLarge', {
            size: formatFileSize(preview.previewSize.value),
            limit: formatFileSize(ATTACHMENT_PREVIEW_MAX_BLOB_SIZE_BYTES)
          })
        }}
      </NAlert>
      <NEmpty
        v-else-if="preview.state.value === 'unsupported'"
        :description="$t('page.attachment.previewUnsupported')"
      />
      <NEmpty v-else-if="preview.state.value === 'not-ready'" :description="$t('page.attachment.previewNotReady')" />
      <NEmpty
        v-else-if="preview.state.value === 'error'"
        :description="preview.error.value || $t('page.attachment.previewFailed')"
      />
      <NEmpty v-else :description="$t('page.attachment.previewUnavailable')" />
    </div>
  </NModal>
</template>
