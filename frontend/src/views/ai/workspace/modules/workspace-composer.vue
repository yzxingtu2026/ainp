<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { attachmentTypeIcon, isImageContentType, useAttachmentGridThumbs } from '@/hooks/business/attachment';
import { useAttachmentUploadQueue } from '@/hooks/business/attachment-upload-queue';
import { useAttachmentThumbStatusPolling } from '@/hooks/business/attachment-thumb-poll';
import AttachmentPicker from '@/components/common/attachment-picker.vue';
import { $t } from '@/locales';
import { normalizeClipboardImage } from './clipboard-image';
import {
  isWorkspaceAttachment,
  workspaceAttachmentExts,
  workspaceAttachmentMimes
} from './document-attachment';

defineOptions({ name: 'WorkspaceComposer' });

const props = defineProps<{
  disabled?: boolean;
  canSend?: boolean;
  active?: boolean;
  draftKey?: string;
}>();
const text = defineModel<string>({ default: '' });
const attachments = defineModel<Api.Attachment.Item[]>('attachments', { default: () => [] });
const emit = defineEmits<{ send: []; stop: [] }>();

const pickerVisible = ref(false);
const imageAttachments = computed(() => attachments.value.filter(item => isImageContentType(item.content_type)));
const thumbsEnabled = computed(() => imageAttachments.value.length > 0);
const thumbs = useAttachmentGridThumbs(imageAttachments, { enabled: thumbsEnabled });
useAttachmentThumbStatusPolling(attachments, { enabled: thumbsEnabled, maxItems: 4 });
const uploadQueue = useAttachmentUploadQueue({
  maxCount: () => Math.max(0, 4 - attachments.value.length),
  maxSizeBytes: 10 * 1024 * 1024,
  allowedExt: workspaceAttachmentExts,
  allowedMime: workspaceAttachmentMimes,
  accept: file =>
    isWorkspaceAttachment({ original_name: file.name, content_type: file.type || 'application/octet-stream' }),
  onSuccess(item) {
    if (item.attachment && attachments.value.length < 4) attachments.value = [...attachments.value, item.attachment];
    uploadQueue.remove(item.localId);
  }
});

watch(
  () => props.draftKey,
  () => uploadQueue.reset()
);

function onKeydown(event: KeyboardEvent) {
  if (event.key !== 'Enter' || event.shiftKey || event.isComposing) return;
  event.preventDefault();
  if (!props.active && props.canSend && !props.disabled && !uploadQueue.busy.value) emit('send');
}

function onPaste(event: ClipboardEvent) {
  if (props.disabled || props.active) return;
  const imageFiles = Array.from(event.clipboardData?.items || [])
    .filter(item => item.kind === 'file' && item.type.startsWith('image/'))
    .map(item => item.getAsFile())
    .filter((file): file is File => Boolean(file));
  if (imageFiles.length === 0) return;

  event.preventDefault();
  const remaining = Math.max(0, 4 - attachments.value.length - uploadQueue.items.value.length);
  if (remaining === 0) {
    window.$message?.warning($t('page.aiWorkspace.attachmentLimit'));
    return;
  }
  const timestamp = Date.now();
  const normalized = imageFiles
    .slice(0, remaining)
    .map((file, index) => normalizeClipboardImage(file, index, timestamp))
    .filter((file): file is File => Boolean(file));
  if (normalized.length === 0) {
    window.$message?.warning($t('page.aiWorkspace.clipboardImageUnsupported'));
    return;
  }
  if (normalized.length < Math.min(imageFiles.length, remaining)) {
    window.$message?.warning($t('page.aiWorkspace.clipboardImageUnsupported'));
  }
  if (imageFiles.length > remaining) window.$message?.warning($t('page.aiWorkspace.attachmentLimit'));
  uploadQueue.enqueue(normalized);
}

function selectAttachments(items: Api.Attachment.Item[]) {
  const selected = items.filter(
    item => item.status === 'ready' && item.sha256 && item.size <= 10 * 1024 * 1024 && isWorkspaceAttachment(item)
  );
  if (selected.length !== items.length) window.$message?.warning($t('page.aiWorkspace.attachmentUnsupported'));
  attachments.value = selected
    .slice(0, 4);
}
</script>

<template>
  <div class="workspace-composer">
    <div v-if="attachments.length || uploadQueue.items.value.length" class="workspace-composer__attachments">
      <div v-for="item in attachments" :key="item.id" class="workspace-composer__attachment">
        <img v-if="thumbs.urls[item.id]" :src="thumbs.urls[item.id]" :alt="item.original_name" />
        <span v-else class="workspace-composer__file">
          <SvgIcon :icon="attachmentTypeIcon(item.content_type)" class="text-20px" />
          <span :title="item.original_name">{{ item.original_name }}</span>
        </span>
        <button
          type="button"
          :title="$t('page.aiWorkspace.removeAttachment')"
          @click="attachments = attachments.filter(attachment => attachment.id !== item.id)"
        >
          <SvgIcon icon="mdi:close" />
        </button>
      </div>
      <div
        v-for="item in uploadQueue.items.value"
        :key="item.localId"
        class="workspace-composer__attachment workspace-composer__attachment--upload"
        :title="item.error || $t('page.aiWorkspace.imageUploading')"
      >
        <NSpin v-if="item.status !== 'failed'" :size="18" />
        <SvgIcon v-else icon="mdi:image-remove-outline" class="text-20px text-error" />
        <span>{{ item.status === 'uploading' ? `${item.progress}%` : item.file.name }}</span>
        <span class="workspace-composer__upload-actions">
          <button
            v-if="item.status === 'failed'"
            type="button"
            :title="$t('page.aiWorkspace.retryImageUpload')"
            @click="uploadQueue.retry(item.localId)"
          >
            <SvgIcon icon="mdi:refresh" />
          </button>
          <button
            type="button"
            :title="$t('page.aiWorkspace.removeImage')"
            @click="uploadQueue.remove(item.localId)"
          >
            <SvgIcon icon="mdi:close" />
          </button>
        </span>
      </div>
    </div>
    <div class="workspace-composer__capsule">
      <button
        type="button"
        class="workspace-composer__icon"
        :title="$t('page.aiWorkspace.addAttachment')"
        :disabled="disabled || active || uploadQueue.busy.value || attachments.length >= 4"
        @click="pickerVisible = true"
      >
        <SvgIcon icon="mdi:plus" class="text-22px" />
      </button>
      <textarea
        v-model="text"
        rows="1"
        :placeholder="$t('page.aiWorkspace.placeholder')"
        :disabled="disabled || active"
        @keydown="onKeydown"
        @paste="onPaste"
      />
      <button
        type="button"
        class="workspace-composer__primary"
        :class="{ 'workspace-composer__primary--stop': active }"
        :title="active ? $t('page.aiWorkspace.stop') : $t('page.aiWorkspace.send')"
        :disabled="active ? false : !canSend || disabled || uploadQueue.busy.value"
        @click="active ? emit('stop') : emit('send')"
      >
        <SvgIcon :icon="active ? 'mdi:stop' : 'mdi:arrow-up'" class="text-20px" />
      </button>
    </div>
    <AttachmentPicker
      v-model:show="pickerVisible"
      :multiple="true"
      :max-count="4"
      :selected-ids="attachments.map(item => item.id)"
      :allowed-ext="workspaceAttachmentExts"
      :allowed-mime="workspaceAttachmentMimes"
      :max-size-bytes="10 * 1024 * 1024"
      :accept="isWorkspaceAttachment"
      @confirm="selectAttachments"
    />
  </div>
</template>

<style scoped>
.workspace-composer {
  width: min(820px, 100%);
  margin: 0 auto;
}

.workspace-composer__attachments {
  display: flex;
  gap: 8px;
  overflow-x: auto;
  padding: 0 12px 8px;
}

.workspace-composer__attachment {
  position: relative;
  width: 64px;
  height: 48px;
  flex: none;
  overflow: hidden;
  border: 1px solid rgba(127, 127, 127, 0.2);
  border-radius: 6px;
  background: var(--n-color, white);
  font-size: 10px;
}

.workspace-composer__attachment img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.workspace-composer__file {
  width: 100%;
  height: 100%;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 2px;
  padding: 4px;
}

.workspace-composer__file span {
  width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  text-align: center;
}

.workspace-composer__attachment--upload {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 3px;
  padding: 4px;
  color: rgba(127, 127, 127, 0.95);
}

.workspace-composer__attachment--upload span {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.workspace-composer__attachment--upload .workspace-composer__upload-actions {
  position: absolute;
  top: 2px;
  right: 2px;
  display: flex;
  overflow: visible;
}

.workspace-composer__attachment--upload .workspace-composer__upload-actions button {
  position: static;
}

.workspace-composer__attachment button {
  position: absolute;
  top: 2px;
  right: 2px;
  width: 20px;
  height: 20px;
  display: grid;
  place-items: center;
  border: 0;
  border-radius: 50%;
  background: rgba(0, 0, 0, 0.62);
  color: white;
}

.workspace-composer__capsule {
  min-height: 56px;
  display: flex;
  align-items: flex-end;
  gap: 8px;
  border: 1px solid rgba(127, 127, 127, 0.22);
  border-radius: 28px;
  background: rgb(var(--container-bg-color));
  padding: 7px;
  box-shadow: 0 8px 28px rgba(25, 31, 46, 0.1);
}

.workspace-composer__capsule:focus-within {
  border-color: rgba(var(--primary-color), 0.55);
  box-shadow:
    0 0 0 3px rgba(var(--primary-color), 0.1),
    0 8px 28px rgba(25, 31, 46, 0.1);
}

.workspace-composer textarea {
  min-height: 40px;
  max-height: 160px;
  flex: 1;
  resize: none;
  border: 0;
  outline: none;
  background: transparent;
  padding: 9px 2px;
  color: inherit;
  line-height: 22px;
}

.workspace-composer__icon,
.workspace-composer__primary {
  width: 42px;
  height: 42px;
  flex: none;
  display: grid;
  place-items: center;
  border: 0;
  border-radius: 50%;
  cursor: pointer;
}

.workspace-composer__icon {
  background: transparent;
  color: rgba(127, 127, 127, 0.95);
}

.workspace-composer__icon:hover {
  background: rgba(127, 127, 127, 0.1);
}
.workspace-composer__primary {
  background: rgb(var(--primary-color));
  color: white;
}
.workspace-composer__primary--stop {
  background: #ef4444;
}
.workspace-composer button:disabled {
  cursor: not-allowed;
  opacity: 0.42;
}

@media (max-width: 640px) {
  .workspace-composer__capsule {
    border-radius: 20px;
  }
}
</style>
