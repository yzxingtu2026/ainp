<script setup lang="ts">
import { computed, ref } from 'vue';
import { $t } from '@/locales';
import { isImageContentType, useAttachmentGridThumbs } from '@/hooks/business/attachment';
import AttachmentPicker from '@/components/common/attachment-picker.vue';

defineOptions({ name: 'PreviewComposer' });

const AI_IMAGE_MIMES = new Set(['image/jpeg', 'image/png', 'image/gif', 'image/webp']);
const AI_IMAGE_MAX_BYTES = 10 * 1024 * 1024;
const AI_IMAGE_TOTAL_MAX_BYTES = 20 * 1024 * 1024;

const props = defineProps<{
  disabled?: boolean;
  canSend?: boolean;
  active?: boolean;
  supportsVision?: boolean;
}>();

const model = defineModel<string>({ default: '' });
const images = defineModel<Api.Attachment.Item[]>('images', { default: () => [] });

const emit = defineEmits<{
  send: [];
  stop: [];
  clear: [];
}>();

const showStop = computed(() => Boolean(props.active));
const pickerVisible = ref(false);
const thumbs = useAttachmentGridThumbs(images, {
  enabled: computed(() => images.value.length > 0)
});
const attachDisabled = computed(() => Boolean(props.disabled || showStop.value || images.value.length >= 4));
const attachTitle = computed(() =>
  props.supportsVision ? $t('page.agentBuilder.chooseImages') : $t('page.agentBuilder.previewVisionUnsupported')
);

function onKeydown(event: KeyboardEvent) {
  if (event.key !== 'Enter' || event.shiftKey || event.isComposing) return;
  event.preventDefault();
  if (showStop.value) return;
  if (!props.canSend || props.disabled) return;
  emit('send');
}

function onPrimaryAction() {
  if (showStop.value) {
    emit('stop');
    return;
  }
  if (!props.canSend || props.disabled) return;
  emit('send');
}

function openPicker() {
  if (props.disabled || showStop.value) return;
  if (!props.supportsVision) {
    window.$message?.warning($t('page.agentBuilder.previewVisionUnsupported'));
    return;
  }
  pickerVisible.value = true;
}

function selectImages(items: Api.Attachment.Item[]) {
  const accepted: Api.Attachment.Item[] = [];
  let totalBytes = 0;
  for (const item of items) {
    const mime = item.content_type.toLowerCase().split(';')[0].trim();
    if (
      accepted.length >= 4 ||
      item.status !== 'ready' ||
      !item.sha256 ||
      !isImageContentType(mime) ||
      !AI_IMAGE_MIMES.has(mime) ||
      item.size <= 0 ||
      item.size > AI_IMAGE_MAX_BYTES ||
      totalBytes + item.size > AI_IMAGE_TOTAL_MAX_BYTES
    ) {
      continue;
    }
    accepted.push(item);
    totalBytes += item.size;
  }
  if (accepted.length !== items.length) {
    window.$message?.warning($t('page.agentBuilder.previewImageLimits'));
  }
  images.value = accepted;
}

function removeImage(id: string) {
  if (showStop.value) return;
  images.value = images.value.filter(item => item.id !== id);
}
</script>

<template>
  <div>
    <div class="preview-composer flex items-end gap-10px">
      <button
        type="button"
        class="preview-composer__clear"
        :title="$t('page.agentBuilder.clearPreview')"
        :aria-label="$t('page.agentBuilder.clearPreview')"
        @click="emit('clear')"
      >
        <SvgIcon icon="mdi:broom" class="text-18px" />
      </button>

      <div class="min-w-0 flex flex-1 flex-col gap-8px">
        <div v-if="images.length" class="flex flex-wrap gap-8px">
          <div
            v-for="item in images"
            :key="item.id"
            class="preview-composer__image relative overflow-hidden rounded-8px border"
          >
            <img
              v-if="thumbs.urls[item.id]"
              :src="thumbs.urls[item.id]"
              :alt="item.original_name"
              class="h-full w-full object-cover"
            />
            <div v-else class="h-full w-full flex-col-center gap-2px bg-gray-100 text-gray-400 dark:bg-gray-800">
              <SvgIcon icon="mdi:image-outline" class="text-22px" />
              <span class="max-w-60px truncate text-10px">{{ item.original_name }}</span>
            </div>
            <button
              type="button"
              class="preview-composer__image-remove"
              :disabled="showStop"
              :title="$t('page.agentBuilder.removeImage')"
              :aria-label="$t('page.agentBuilder.removeImage')"
              @click="removeImage(item.id)"
            >
              <SvgIcon icon="mdi:close" class="text-14px" />
            </button>
          </div>
        </div>
        <div v-if="images.length && !supportsVision" class="preview-composer__warning text-12px">
          {{ $t('page.agentBuilder.previewVisionUnsupported') }}
        </div>

        <div class="preview-composer__capsule" :class="{ 'preview-composer__capsule--disabled': disabled }">
          <input
            v-model="model"
            type="text"
            class="preview-composer__input"
            :placeholder="$t('page.agentBuilder.previewPlaceholder')"
            :disabled="disabled || showStop"
            @keydown="onKeydown"
          />
          <div class="preview-composer__actions">
            <button
              type="button"
              class="preview-composer__plus"
              :disabled="attachDisabled"
              :title="attachTitle"
              :aria-label="attachTitle"
              @click="openPicker"
            >
              <SvgIcon icon="mdi:plus" class="text-16px" />
            </button>
            <span class="preview-composer__divider" />
            <button
              type="button"
              class="preview-composer__primary"
              :class="{ 'preview-composer__primary--stop': showStop }"
              :disabled="showStop ? false : !canSend || disabled"
              :title="showStop ? $t('page.agentBuilder.stopPreview') : $t('page.agentBuilder.runPreview')"
              :aria-label="showStop ? $t('page.agentBuilder.stopPreview') : $t('page.agentBuilder.runPreview')"
              @click="onPrimaryAction"
            >
              <SvgIcon v-if="showStop" icon="mdi:stop" class="text-18px" />
              <SvgIcon v-else icon="mdi:send" class="text-18px" />
            </button>
          </div>
        </div>
      </div>
    </div>

    <AttachmentPicker
      v-model:show="pickerVisible"
      :selected-ids="images.map(item => item.id)"
      :max-count="4"
      @confirm="selectImages"
    />
  </div>
</template>

<style scoped>
.preview-composer__clear {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 36px;
  flex-shrink: 0;
  border: 1px solid rgb(0 0 0 / 12%);
  border-radius: 9999px;
  background: transparent;
  color: rgb(0 0 0 / 45%);
  cursor: pointer;
  transition:
    color 0.15s ease,
    border-color 0.15s ease,
    background-color 0.15s ease;
}

.preview-composer__clear:hover {
  color: rgb(var(--primary-color));
  border-color: rgb(var(--primary-color) / 45%);
  background: rgb(var(--primary-color) / 8%);
}

.preview-composer__capsule {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  flex: 1;
  height: 42px;
  padding: 0 8px 0 16px;
  border: 1px solid rgb(var(--primary-color));
  border-radius: 9999px;
  background: #fff;
}

.preview-composer__capsule--disabled {
  opacity: 0.7;
}

.preview-composer__input {
  min-width: 0;
  flex: 1;
  height: 100%;
  border: none;
  outline: none;
  background: transparent;
  color: inherit;
  font-size: 14px;
  line-height: 1.4;
}

.preview-composer__input::placeholder {
  color: rgb(0 0 0 / 35%);
}

.preview-composer__actions {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  flex-shrink: 0;
}

.preview-composer__plus {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 26px;
  border: 1px solid rgb(0 0 0 / 12%);
  border-radius: 9999px;
  background: transparent;
  color: rgb(0 0 0 / 40%);
  cursor: pointer;
}

.preview-composer__plus:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}

.preview-composer__image {
  width: 72px;
  height: 72px;
}

.preview-composer__image-remove {
  position: absolute;
  top: 3px;
  right: 3px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  border: none;
  border-radius: 9999px;
  background: rgb(0 0 0 / 62%);
  color: #fff;
  cursor: pointer;
}

.preview-composer__image-remove:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.preview-composer__warning {
  color: #d97706;
}

.preview-composer__divider {
  width: 1px;
  height: 18px;
  background: rgb(0 0 0 / 12%);
}

.preview-composer__primary {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border: none;
  border-radius: 9999px;
  background: transparent;
  color: rgb(var(--primary-color));
  cursor: pointer;
  transition:
    opacity 0.15s ease,
    background-color 0.15s ease;
}

.preview-composer__primary:disabled {
  opacity: 0.35;
  cursor: not-allowed;
  color: rgb(0 0 0 / 35%);
}

.preview-composer__primary:not(:disabled):hover {
  background: rgb(var(--primary-color) / 10%);
}

.preview-composer__primary--stop {
  color: rgb(var(--primary-color));
}

html.dark .preview-composer__capsule,
.dark .preview-composer__capsule {
  background: #18181c;
}

html.dark .preview-composer__clear,
.dark .preview-composer__clear {
  border-color: rgb(255 255 255 / 18%);
  color: rgb(255 255 255 / 55%);
}

html.dark .preview-composer__plus,
.dark .preview-composer__plus {
  border-color: rgb(255 255 255 / 18%);
  color: rgb(255 255 255 / 45%);
}

html.dark .preview-composer__divider,
.dark .preview-composer__divider {
  background: rgb(255 255 255 / 18%);
}

html.dark .preview-composer__input::placeholder,
.dark .preview-composer__input::placeholder {
  color: rgb(255 255 255 / 35%);
}

html.dark .preview-composer__primary:disabled,
.dark .preview-composer__primary:disabled {
  color: rgb(255 255 255 / 35%);
}
</style>
