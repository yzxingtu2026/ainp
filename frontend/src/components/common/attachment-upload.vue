<script setup lang="ts">
import { computed, ref } from 'vue';
import { useAttachmentUploadQueue } from '@/hooks/business/attachment-upload-queue';
import { formatFileSize } from '@/hooks/business/attachment';
import { $t } from '@/locales';

defineOptions({ name: 'AttachmentUpload' });

const props = withDefaults(
  defineProps<{
    multiple?: boolean;
    disabled?: boolean;
    maxCount?: number;
    maxSizeBytes?: number;
    allowedExt?: string[];
    allowedMime?: string[];
  }>(),
  {
    multiple: true,
    disabled: false
  }
);

const emit = defineEmits<{
  success: [attachment: Api.Attachment.Item];
  failed: [message: string];
}>();

const inputRef = ref<HTMLInputElement | null>(null);
const dragging = ref(false);

const {
  items: queueItems,
  enqueue,
  cancel,
  remove,
  retry,
  maxSizeBytes,
  allowedExt
} = useAttachmentUploadQueue({
  maxCount: props.maxCount,
  maxSizeBytes: props.maxSizeBytes,
  allowedExt: props.allowedExt,
  allowedMime: props.allowedMime,
  onSuccess: async item => {
    if (item.attachment) {
      emit('success', item.attachment);
    }
  }
});

const accept = computed(() => allowedExt.join(','));
const hint = computed(() =>
  $t('page.attachment.uploadHint', {
    size: formatFileSize(maxSizeBytes),
    ext: allowedExt.join(', ')
  })
);

function openPicker() {
  if (props.disabled) return;
  inputRef.value?.click();
}

function onFileChange(event: Event) {
  const target = event.target as HTMLInputElement;
  if (target.files?.length) {
    enqueue(target.files);
  }
  target.value = '';
}

function onDrop(event: DragEvent) {
  event.preventDefault();
  dragging.value = false;
  if (props.disabled) return;
  if (event.dataTransfer?.files?.length) {
    enqueue(event.dataTransfer.files);
  }
}

function statusLabel(status: string) {
  const map: Record<string, string> = {
    queued: $t('page.attachment.status.queued'),
    preparing: $t('page.attachment.status.preparing'),
    uploading: $t('page.attachment.status.uploading'),
    completing: $t('page.attachment.status.completing'),
    success: $t('page.attachment.status.success'),
    failed: $t('page.attachment.status.failed'),
    cancelled: $t('page.attachment.status.cancelled')
  };
  return map[status] || status;
}

function errorLabel(code: string) {
  const map: Record<string, string> = {
    size: $t('page.attachment.precheck.size'),
    ext: $t('page.attachment.precheck.ext'),
    mime: $t('page.attachment.precheck.mime'),
    empty: $t('page.attachment.precheck.empty')
  };
  return map[code] || code;
}
</script>

<template>
  <div class="flex-col gap-12px">
    <div
      class="border border-dashed rounded-8px px-16px py-20px transition-colors"
      :class="[
        dragging ? 'border-primary bg-primary/5' : 'border-gray-300 dark:border-gray-600',
        disabled ? 'opacity-50 pointer-events-none' : 'cursor-pointer'
      ]"
      @click="openPicker"
      @dragenter.prevent="dragging = true"
      @dragover.prevent="dragging = true"
      @dragleave.prevent="dragging = false"
      @drop="onDrop"
    >
      <div class="flex-col-center gap-8px text-center">
        <SvgIcon icon="mdi:cloud-upload-outline" class="text-28px text-primary" />
        <div class="text-14px">{{ $t('page.attachment.dropOrClick') }}</div>
        <div class="text-12px text-gray-500">{{ hint }}</div>
        <NButton size="small" type="primary" secondary :disabled="disabled" @click.stop="openPicker">
          {{ $t('page.attachment.selectFiles') }}
        </NButton>
      </div>
      <input
        ref="inputRef"
        class="hidden"
        type="file"
        :accept="accept"
        :multiple="multiple"
        :disabled="disabled"
        @change="onFileChange"
      />
    </div>

    <div v-if="queueItems.length" class="flex-col gap-8px">
      <div
        v-for="item in queueItems"
        :key="item.localId"
        class="flex items-center gap-12px rounded-8px bg-gray-50 px-12px py-10px dark:bg-gray-800"
      >
        <SvgIcon icon="mdi:file-outline" class="text-20px shrink-0" />
        <div class="min-w-0 flex-1">
          <div class="truncate text-13px font-medium">{{ item.file.name }}</div>
          <div class="text-12px text-gray-500">
            {{ formatFileSize(item.file.size) }} · {{ statusLabel(item.status) }}
            <span v-if="item.error"> · {{ errorLabel(item.error) }}</span>
          </div>
          <NProgress
            v-if="item.status === 'uploading' || item.status === 'preparing' || item.status === 'completing'"
            class="mt-6px"
            type="line"
            :percentage="item.progress"
            :show-indicator="false"
            processing
          />
        </div>
        <div class="flex shrink-0 gap-4px">
          <ButtonIcon
            v-if="item.status === 'queued' || item.status === 'preparing' || item.status === 'uploading'"
            icon="mdi:close-circle-outline"
            :tooltip-content="$t('page.attachment.cancel')"
            :aria-label="$t('page.attachment.cancel')"
            @click="cancel(item.localId)"
          />
          <ButtonIcon
            v-if="item.status === 'failed' || item.status === 'cancelled'"
            icon="mdi:refresh"
            :tooltip-content="$t('page.attachment.retry')"
            :aria-label="$t('page.attachment.retry')"
            @click="retry(item.localId)"
          />
          <ButtonIcon
            icon="mdi:trash-can-outline"
            :tooltip-content="$t('common.delete')"
            :aria-label="$t('common.delete')"
            @click="remove(item.localId)"
          />
        </div>
      </div>
    </div>
  </div>
</template>
