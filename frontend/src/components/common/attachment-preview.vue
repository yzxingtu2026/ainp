<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, reactive, ref, watch } from 'vue';
import { useResizeObserver } from '@vueuse/core';
import {
  clampAttachmentPreviewImageOffset,
  nextAttachmentPreviewImageZoom
} from '@/components/common/attachment-preview-image';
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
const imageStageRef = ref<HTMLElement | null>(null);
const imageRef = ref<HTMLImageElement | null>(null);
const imageZoom = ref(1);
const imageOffset = reactive({ x: 0, y: 0 });
const imageDragging = ref(false);
let dragPointerId: number | null = null;
let dragLastX = 0;
let dragLastY = 0;
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
    resetImageTransform();
    if (show && props.attachment) {
      void preview.open(props.attachment);
      return;
    }
    preview.close();
  },
  { immediate: true }
);

useResizeObserver(imageStageRef, clampImageOffset);
onBeforeUnmount(resetImageTransform);

function resetImageTransform() {
  if (dragPointerId !== null && imageRef.value?.hasPointerCapture(dragPointerId)) {
    imageRef.value.releasePointerCapture(dragPointerId);
  }
  imageZoom.value = 1;
  imageOffset.x = 0;
  imageOffset.y = 0;
  imageDragging.value = false;
  dragPointerId = null;
}

function clampImageOffset() {
  const stage = imageStageRef.value;
  const image = imageRef.value;
  if (!stage || !image) return;
  const next = clampAttachmentPreviewImageOffset(
    imageOffset,
    { width: image.clientWidth, height: image.clientHeight },
    { width: stage.clientWidth, height: stage.clientHeight },
    imageZoom.value
  );
  imageOffset.x = next.x;
  imageOffset.y = next.y;
}

function zoomImage(event: WheelEvent) {
  const stage = imageStageRef.value;
  if (!stage) return;
  const previousZoom = imageZoom.value;
  const nextZoom = nextAttachmentPreviewImageZoom(previousZoom, event.deltaY);
  if (nextZoom === previousZoom) return;

  const rect = stage.getBoundingClientRect();
  const cursorX = event.clientX - rect.left - rect.width / 2;
  const cursorY = event.clientY - rect.top - rect.height / 2;
  const ratio = nextZoom / previousZoom;
  imageOffset.x = cursorX - (cursorX - imageOffset.x) * ratio;
  imageOffset.y = cursorY - (cursorY - imageOffset.y) * ratio;
  imageZoom.value = nextZoom;
  void nextTick(clampImageOffset);
}

function startImagePan(event: PointerEvent) {
  if (imageZoom.value <= 1) return;
  dragPointerId = event.pointerId;
  dragLastX = event.clientX;
  dragLastY = event.clientY;
  imageDragging.value = true;
  imageRef.value?.setPointerCapture(event.pointerId);
}

function panImage(event: PointerEvent) {
  if (!imageDragging.value || dragPointerId !== event.pointerId) return;
  imageOffset.x += event.clientX - dragLastX;
  imageOffset.y += event.clientY - dragLastY;
  dragLastX = event.clientX;
  dragLastY = event.clientY;
  clampImageOffset();
}

function stopImagePan(event: PointerEvent) {
  if (dragPointerId !== event.pointerId) return;
  if (imageRef.value?.hasPointerCapture(event.pointerId)) imageRef.value.releasePointerCapture(event.pointerId);
  imageDragging.value = false;
  dragPointerId = null;
}

function close() {
  resetImageTransform();
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
      </div>
    </template>

    <div
      ref="imageStageRef"
      class="min-h-320px flex-col-center overflow-hidden bg-gray-50 p-12px dark:bg-gray-900 sm:min-h-480px"
    >
      <NSpin v-if="preview.state.value === 'loading'" size="large" />

      <img
        v-else-if="preview.state.value === 'ready' && preview.previewKind.value === 'image'"
        ref="imageRef"
        :src="preview.objectUrl.value || ''"
        :alt="preview.previewName.value"
        class="max-h-[75vh] max-w-full touch-none select-none object-contain"
        :class="imageZoom > 1 ? (imageDragging ? 'cursor-grabbing' : 'cursor-grab') : 'cursor-zoom-in'"
        :style="{
          transform: `translate(${imageOffset.x}px, ${imageOffset.y}px) scale(${imageZoom})`,
          transformOrigin: 'center center'
        }"
        draggable="false"
        @load="clampImageOffset"
        @wheel.prevent="zoomImage"
        @pointerdown="startImagePan"
        @pointermove="panImage"
        @pointerup="stopImagePan"
        @pointercancel="stopImagePan"
        @dblclick="resetImageTransform"
      />
      <iframe
        v-else-if="preview.state.value === 'ready' && preview.previewKind.value === 'pdf'"
        :src="preview.objectUrl.value || ''"
        :title="preview.previewName.value"
        class="h-[75vh] min-h-480px w-full border-0"
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
