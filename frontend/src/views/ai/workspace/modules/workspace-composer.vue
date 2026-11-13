<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { useTextareaAutosize } from '@vueuse/core';
import {
  attachmentTypeIcon,
  canPreviewAttachment,
  formatFileSize,
  isImageContentType,
  useAttachmentGridThumbs
} from '@/hooks/business/attachment';
import { useAttachmentUploadQueue } from '@/hooks/business/attachment-upload-queue';
import { useAttachmentThumbStatusPolling } from '@/hooks/business/attachment-thumb-poll';
import AttachmentPicker from '@/components/common/attachment-picker.vue';
import AttachmentPreview from '@/components/common/attachment-preview.vue';
import { $t } from '@/locales';
import { normalizeClipboardImage } from './clipboard-image';
import {
  attachmentExtBadge,
  isWorkspaceAttachment,
  workspaceAttachmentExts,
  workspaceAttachmentMimes,
  workspaceDocumentIcon
} from './document-attachment';

defineOptions({ name: 'WorkspaceComposer' });

const props = defineProps<{
  disabled?: boolean;
  offline?: boolean;
  failed?: boolean;
  canSend?: boolean;
  active?: boolean;
  draftKey?: string;
  density?: 'workspace' | 'sidebar';
}>();
const text = defineModel<string>({ default: '' });
const attachments = defineModel<Api.Attachment.Item[]>('attachments', { default: () => [] });
const emit = defineEmits<{ send: []; stop: [] }>();

const pickerVisible = ref(false);
const previewVisible = ref(false);
const previewTarget = ref<Api.Attachment.Item | null>(null);
const textareaElement = ref<HTMLTextAreaElement>();
const { triggerResize } = useTextareaAutosize({ element: textareaElement, input: text });
const composerStyle = computed<Record<string, string>>(() => ({
  '--workspace-composer-textarea-max-height': props.density === 'sidebar' ? 'min(194px, 32dvh)' : 'min(238px, 38dvh)'
}));
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

// Visual state only: interaction guards remain owned by the existing controller/queue.
const composerClasses = computed(() => ({
  [`workspace-composer--${props.density || 'workspace'}`]: true,
  'workspace-composer--has-content': Boolean(text.value.trim() || attachments.value.length),
  'workspace-composer--active': props.active,
  'workspace-composer--uploading': uploadQueue.busy.value,
  'workspace-composer--failed': props.failed || uploadQueue.items.value.some(item => item.status === 'failed'),
  'workspace-composer--disabled': props.disabled,
  'workspace-composer--offline': props.offline
}));

const sendReady = computed(() => Boolean(!props.active && props.canSend && !props.disabled && !uploadQueue.busy.value));

watch(
  () => props.draftKey,
  () => uploadQueue.reset()
);
watch(() => props.density, triggerResize, { flush: 'post' });

function openPreview(item: Api.Attachment.Item) {
  if (!canPreviewAttachment(item)) return;
  previewTarget.value = item;
  previewVisible.value = true;
}

function itemIcon(item: Api.Attachment.Item) {
  if (isImageContentType(item.content_type)) return 'mdi:image-outline';
  return workspaceDocumentIcon(item.content_type, item.original_name) || attachmentTypeIcon(item.content_type);
}

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
  attachments.value = selected.slice(0, 4);
}
</script>

<template>
  <div class="workspace-composer" :class="composerClasses" :style="composerStyle">
    <div v-if="attachments.length || uploadQueue.items.value.length" class="workspace-composer__attachments">
      <div
        v-for="item in attachments"
        :key="item.id"
        class="workspace-composer__attachment"
        :class="{
          'workspace-composer__attachment--image': isImageContentType(item.content_type),
          'workspace-composer__attachment--doc': !isImageContentType(item.content_type)
        }"
      >
        <button
          v-if="canPreviewAttachment(item)"
          type="button"
          class="workspace-composer__attachment-preview"
          :title="item.original_name"
          :aria-label="`${$t('page.aiWorkspace.previewAttachment')}：${item.original_name}`"
          @click="openPreview(item)"
        >
          <img
            v-if="isImageContentType(item.content_type) && thumbs.urls[item.id]"
            :src="thumbs.urls[item.id]"
            :alt="item.original_name"
          />
          <span
            v-else-if="isImageContentType(item.content_type) && item.thumb_status === 'pending'"
            class="workspace-composer__thumb-pending"
          >
            <NSpin :size="15" />
          </span>
          <span v-else-if="isImageContentType(item.content_type)" class="workspace-composer__thumb-pending">
            <SvgIcon icon="mdi:image-outline" class="text-18px" />
          </span>
          <span v-else class="workspace-composer__doc-chip">
            <span class="workspace-composer__doc-icon">
              <SvgIcon :icon="itemIcon(item)" class="text-17px" />
            </span>
            <span class="workspace-composer__doc-meta">
              <span class="workspace-composer__doc-name">{{ item.original_name }}</span>
              <span class="workspace-composer__doc-sub">
                {{ attachmentExtBadge(item.original_name, item.content_type) }} · {{ formatFileSize(item.size) }}
              </span>
            </span>
          </span>
        </button>

        <span v-else class="workspace-composer__doc-chip" :title="item.original_name">
          <span class="workspace-composer__doc-icon">
            <SvgIcon :icon="itemIcon(item)" class="text-17px" />
          </span>
          <span class="workspace-composer__doc-meta">
            <span class="workspace-composer__doc-name">{{ item.original_name }}</span>
            <span class="workspace-composer__doc-sub">
              {{ attachmentExtBadge(item.original_name, item.content_type) }} · {{ formatFileSize(item.size) }}
            </span>
          </span>
        </span>

        <button
          type="button"
          class="workspace-composer__remove"
          :title="$t('page.aiWorkspace.removeAttachment')"
          :aria-label="$t('page.aiWorkspace.removeAttachment')"
          @click="attachments = attachments.filter(attachment => attachment.id !== item.id)"
        >
          <SvgIcon icon="mdi:close" />
        </button>
      </div>

      <div
        v-for="item in uploadQueue.items.value"
        :key="item.localId"
        class="workspace-composer__attachment workspace-composer__attachment--upload"
        :class="{ 'workspace-composer__attachment--failed': item.status === 'failed' }"
        :title="item.error || $t('page.aiWorkspace.imageUploading')"
      >
        <span class="workspace-composer__upload-icon">
          <NSpin v-if="item.status !== 'failed'" :size="15" />
          <SvgIcon v-else icon="mdi:image-remove-outline" class="text-17px text-error" />
        </span>
        <span class="workspace-composer__doc-meta">
          <span class="workspace-composer__doc-name">{{ item.file.name }}</span>
          <span class="workspace-composer__doc-sub">
            {{
              item.status === 'uploading' ? `${item.progress}%` : item.error || $t('page.aiWorkspace.imageUploading')
            }}
          </span>
        </span>
        <span class="workspace-composer__upload-actions">
          <button
            v-if="item.status === 'failed'"
            type="button"
            class="workspace-composer__upload-btn"
            :title="$t('page.aiWorkspace.retryImageUpload')"
            @click="uploadQueue.retry(item.localId)"
          >
            <SvgIcon icon="mdi:refresh" />
          </button>
          <button
            type="button"
            class="workspace-composer__upload-btn"
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
        :aria-label="$t('page.aiWorkspace.addAttachment')"
        :disabled="disabled || active || uploadQueue.busy.value || attachments.length >= 4"
        @click="pickerVisible = true"
      >
        <SvgIcon icon="mdi:plus" class="text-22px" />
      </button>
      <textarea
        ref="textareaElement"
        v-model="text"
        rows="2"
        :aria-label="$t('page.aiWorkspace.promptInput')"
        :placeholder="$t('page.aiWorkspace.placeholder')"
        :disabled="disabled || active"
        @keydown="onKeydown"
        @paste="onPaste"
      />
      <button
        type="button"
        class="workspace-composer__primary"
        :class="{
          'workspace-composer__primary--stop': active,
          'workspace-composer__primary--ready': sendReady
        }"
        :title="active ? $t('page.aiWorkspace.stop') : $t('page.aiWorkspace.send')"
        :aria-label="active ? $t('page.aiWorkspace.stop') : $t('page.aiWorkspace.send')"
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

    <AttachmentPreview v-if="previewVisible" v-model:show="previewVisible" :attachment="previewTarget" />
  </div>
</template>

<style scoped>
.workspace-composer {
  --workspace-composer-textarea-max-height: min(238px, 38dvh);
  --aurora-indigo: 100, 108, 255;
  --aurora-cyan: 8, 145, 178;
  --aurora-mint: 52, 211, 153;
  --aurora-glow: 0.12;
  --aurora-blur: 7px;
  --aurora-outset: -3px;
  --aurora-speed: 12s;
  --aurora-border-opacity: 0.65;
  width: min(820px, 100%);
  min-width: 0;
  isolation: isolate;
  margin: 0 auto;
}

.workspace-composer__attachments {
  display: flex;
  align-items: center;
  gap: 8px;
  overflow-x: auto;
  padding: 0 12px 9px;
}

.workspace-composer__attachment {
  position: relative;
  flex: none;
  overflow: hidden;
  border: 1px solid rgba(127, 127, 127, 0.2);
  border-radius: 10px;
  background: rgb(var(--container-bg-color));
  transition: border-color 0.16s ease;
}

.workspace-composer__attachment:hover {
  border-color: rgba(var(--aurora-indigo), 0.45);
}

.workspace-composer__attachment--image {
  width: 60px;
  height: 46px;
}

.workspace-composer__attachment--doc {
  height: 46px;
  max-width: 214px;
  display: inline-flex;
  align-items: center;
  padding-right: 26px;
}

.workspace-composer--sidebar .workspace-composer__attachment--doc {
  max-width: 178px;
}

.workspace-composer__attachment-preview {
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  border: 0;
  background: transparent;
  padding: 0;
  color: inherit;
  font: inherit;
  text-align: left;
  cursor: pointer;
}

.workspace-composer__attachment-preview img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.workspace-composer__thumb-pending {
  width: 100%;
  height: 100%;
  display: grid;
  place-items: center;
  color: rgba(127, 127, 127, 0.88);
}

.workspace-composer__doc-chip {
  min-width: 0;
  height: 100%;
  display: inline-flex;
  align-items: center;
  gap: 7px;
  padding: 0 8px;
}

.workspace-composer__doc-icon {
  width: 26px;
  height: 26px;
  flex: none;
  display: grid;
  place-items: center;
  border-radius: 6px;
  background: rgba(var(--aurora-indigo), 0.1);
  color: rgb(var(--aurora-indigo));
}

.workspace-composer__doc-meta {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 1px;
}

.workspace-composer__doc-name {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 11.5px;
  font-weight: 560;
  line-height: 1.25;
}

.workspace-composer__doc-sub {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: rgba(127, 127, 127, 0.86);
  font-size: 10px;
  font-variant-numeric: tabular-nums;
  line-height: 1.2;
}

.workspace-composer__remove {
  position: absolute;
  top: 4px;
  right: 4px;
  width: 18px;
  height: 18px;
  display: grid;
  place-items: center;
  border: 0;
  border-radius: 50%;
  background: rgba(15, 23, 42, 0.62);
  color: #fff;
  font-size: 12px;
  cursor: pointer;
}

.workspace-composer__attachment--doc .workspace-composer__remove {
  top: 50%;
  right: 5px;
  transform: translateY(-50%);
  background: rgba(127, 127, 127, 0.16);
  color: rgba(var(--base-text-color), 0.78);
}

.workspace-composer__attachment--doc .workspace-composer__remove:hover {
  background: rgba(220, 64, 80, 0.16);
  color: #dc4050;
}

.workspace-composer__attachment--upload {
  height: 46px;
  max-width: 214px;
  display: inline-flex;
  align-items: center;
  gap: 7px;
  padding: 0 6px 0 8px;
  color: rgba(127, 127, 127, 0.95);
}

.workspace-composer__upload-icon {
  width: 24px;
  height: 24px;
  flex: none;
  display: grid;
  place-items: center;
}

.workspace-composer__upload-actions {
  flex: none;
  display: inline-flex;
  align-items: center;
  gap: 2px;
}

.workspace-composer__upload-btn {
  width: 20px;
  height: 20px;
  display: grid;
  place-items: center;
  border: 0;
  border-radius: 50%;
  background: rgba(127, 127, 127, 0.14);
  color: inherit;
  font-size: 12px;
  cursor: pointer;
}

.workspace-composer__upload-btn:hover {
  background: rgba(127, 127, 127, 0.24);
}

.workspace-composer--sidebar {
  --aurora-glow: 0.07;
  --aurora-blur: 4px;
  --aurora-outset: -2px;
  --aurora-border-opacity: 0.5;
}

:where(.dark) .workspace-composer {
  --aurora-cyan: 34, 211, 238;
}

.workspace-composer__capsule {
  position: relative;
  isolation: isolate;
  min-height: 56px;
  display: flex;
  align-items: flex-end;
  gap: 8px;
  border: 1px solid rgba(var(--aurora-indigo), 0.3);
  border-radius: 28px;
  background: rgb(var(--container-bg-color));
  padding: 7px;
  box-shadow: 0 8px 28px rgba(25, 31, 46, 0.08);
  transition:
    border-color 180ms ease,
    box-shadow 180ms ease;
}

/* Both decorative layers are hollow, so text and controls never sit under a gradient. */
.workspace-composer__capsule::before,
.workspace-composer__capsule::after {
  content: '';
  position: absolute;
  z-index: -1;
  pointer-events: none;
  border-radius: inherit;
  background: linear-gradient(
    110deg,
    rgb(var(--aurora-indigo)) 0%,
    rgb(var(--aurora-cyan)) 30%,
    rgb(var(--aurora-mint)) 42%,
    rgb(var(--aurora-indigo)) 60%,
    rgb(var(--aurora-cyan)) 80%,
    rgb(var(--aurora-indigo)) 100%
  );
  background-size: 250% 100%;
  mask:
    linear-gradient(#fff 0 0) content-box,
    linear-gradient(#fff 0 0);
  mask-composite: exclude;
}

.workspace-composer__capsule::before {
  inset: var(--aurora-outset);
  padding: 4px;
  filter: blur(var(--aurora-blur));
  opacity: var(--aurora-glow);
  animation: aurora-breathe 6s ease-in-out infinite;
}

.workspace-composer__capsule::after {
  inset: -1px;
  padding: 1px;
  opacity: var(--aurora-border-opacity);
  animation: aurora-flow var(--aurora-speed) ease-in-out infinite alternate;
}

.workspace-composer--has-content {
  --aurora-speed: 8s;
}

.workspace-composer__capsule:focus-within {
  --aurora-glow: 0.28;
  --aurora-border-opacity: 1;
  border-color: rgba(var(--aurora-indigo), 0.65);
  box-shadow:
    0 0 0 3px rgba(var(--aurora-indigo), 0.12),
    0 8px 28px rgba(25, 31, 46, 0.08);
}

.workspace-composer--sidebar .workspace-composer__capsule:focus-within {
  --aurora-glow: 0.16;
}

.workspace-composer--uploading {
  --aurora-indigo: var(--aurora-cyan);
  --aurora-speed: 4s;
}

.workspace-composer--active .workspace-composer__capsule::after {
  background-image: linear-gradient(
    90deg,
    rgba(var(--aurora-indigo), 0.35) 0%,
    rgb(var(--aurora-indigo)) 30%,
    rgb(var(--aurora-cyan)) 45%,
    rgb(var(--aurora-mint)) 50%,
    rgb(var(--aurora-indigo)) 65%,
    rgba(var(--aurora-indigo), 0.35) 100%
  );
  animation: aurora-flow 3s linear infinite;
}

/* Terminal/unavailable states take precedence over the decorative activity states. */
.workspace-composer--failed {
  --aurora-indigo: 220, 64, 80;
  --aurora-cyan: 220, 64, 80;
  --aurora-mint: 220, 64, 80;
}

.workspace-composer--disabled {
  --aurora-indigo: 127, 127, 127;
  --aurora-cyan: 127, 127, 127;
  --aurora-mint: 127, 127, 127;
}

.workspace-composer--offline {
  --aurora-indigo: 180, 119, 28;
  --aurora-cyan: 180, 119, 28;
  --aurora-mint: 180, 119, 28;
}

.workspace-composer--failed .workspace-composer__capsule::before,
.workspace-composer--disabled .workspace-composer__capsule::before,
.workspace-composer--offline .workspace-composer__capsule::before {
  animation: none;
  opacity: 0;
}

.workspace-composer--failed .workspace-composer__capsule::after,
.workspace-composer--disabled .workspace-composer__capsule::after,
.workspace-composer--offline .workspace-composer__capsule::after {
  animation: none;
}

.workspace-composer__attachment--failed {
  border-color: #dc4050;
}

.workspace-composer textarea {
  min-width: 0;
  min-height: 62px;
  max-height: var(--workspace-composer-textarea-max-height);
  flex: 1;
  resize: none;
  overflow-x: hidden;
  overflow-y: auto;
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
  transition:
    transform 160ms ease,
    box-shadow 180ms ease,
    background 180ms ease;
}

.workspace-composer__icon {
  background: transparent;
  color: rgba(127, 127, 127, 0.95);
}

.workspace-composer__icon:hover:not(:disabled) {
  background: rgba(127, 127, 127, 0.1);
  color: rgb(var(--aurora-indigo));
}

.workspace-composer__primary {
  background: linear-gradient(135deg, #646cff 0%, #4f46e5 100%);
  color: white;
}

.workspace-composer__primary--ready {
  background: linear-gradient(135deg, #646cff 0%, #0891b2 100%);
  box-shadow:
    0 4px 14px rgba(100, 108, 255, 0.36),
    inset 0 1px 0 rgba(255, 255, 255, 0.28);
}

.workspace-composer__primary--ready:hover:not(:disabled) {
  transform: translateY(-1px);
  box-shadow:
    0 6px 18px rgba(100, 108, 255, 0.45),
    inset 0 1px 0 rgba(255, 255, 255, 0.35);
}

.workspace-composer__primary--stop {
  background: #dc3545;
  box-shadow: 0 4px 12px rgba(220, 53, 69, 0.32);
}

.workspace-composer button:disabled {
  cursor: not-allowed;
  opacity: 0.42;
}

.workspace-composer button:focus-visible {
  outline: 2px solid #646cff;
  outline-offset: 3px;
}

.workspace-composer textarea::placeholder {
  color: rgba(127, 127, 127, 0.95);
}

@keyframes aurora-breathe {
  0%,
  100% {
    opacity: var(--aurora-glow);
  }
  50% {
    opacity: calc(var(--aurora-glow) * 0.5);
  }
}

@keyframes aurora-flow {
  from {
    background-position: 0% 50%;
  }
  to {
    background-position: 100% 50%;
  }
}

@media (prefers-reduced-motion: reduce) {
  .workspace-composer__capsule,
  .workspace-composer__attachment,
  .workspace-composer__icon,
  .workspace-composer__primary {
    transition: none;
  }

  .workspace-composer__primary--ready:hover:not(:disabled) {
    transform: none;
  }

  .workspace-composer .workspace-composer__capsule::before,
  .workspace-composer .workspace-composer__capsule::after {
    animation: none;
  }

  .workspace-composer :deep(.n-base-loading .n-base-loading__container) {
    animation: none;
  }
}

@media (max-width: 640px) {
  .workspace-composer__capsule {
    border-radius: 20px;
  }
}
</style>
