<script setup lang="ts">
import { computed, ref } from 'vue';
import { isImageContentType, useAttachmentGridThumbs } from '@/hooks/business/attachment';
import AttachmentPicker from '@/components/common/attachment-picker.vue';
import { $t } from '@/locales';

defineOptions({ name: 'WorkspaceComposer' });

const props = defineProps<{
  disabled?: boolean;
  canSend?: boolean;
  active?: boolean;
}>();
const text = defineModel<string>({ default: '' });
const images = defineModel<Api.Attachment.Item[]>('images', { default: () => [] });
const emit = defineEmits<{ send: []; stop: [] }>();

const pickerVisible = ref(false);
const thumbs = useAttachmentGridThumbs(images, { enabled: computed(() => images.value.length > 0) });

function onKeydown(event: KeyboardEvent) {
  if (event.key !== 'Enter' || event.shiftKey || event.isComposing) return;
  event.preventDefault();
  if (!props.active && props.canSend && !props.disabled) emit('send');
}

function selectImages(items: Api.Attachment.Item[]) {
  images.value = items
    .filter(item => item.status === 'ready' && item.sha256 && isImageContentType(item.content_type))
    .slice(0, 4);
}
</script>

<template>
  <div class="workspace-composer">
    <div v-if="images.length" class="workspace-composer__attachments">
      <div v-for="item in images" :key="item.id" class="workspace-composer__attachment">
        <img v-if="thumbs.urls[item.id]" :src="thumbs.urls[item.id]" :alt="item.original_name" />
        <span v-else>{{ item.original_name }}</span>
        <button
          type="button"
          :title="$t('page.aiWorkspace.removeImage')"
          @click="images = images.filter(image => image.id !== item.id)"
        >
          <SvgIcon icon="mdi:close" />
        </button>
      </div>
    </div>
    <div class="workspace-composer__capsule">
      <button
        type="button"
        class="workspace-composer__icon"
        :title="$t('page.aiWorkspace.addImage')"
        :disabled="disabled || active || images.length >= 4"
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
      />
      <button
        type="button"
        class="workspace-composer__primary"
        :class="{ 'workspace-composer__primary--stop': active }"
        :title="active ? $t('page.aiWorkspace.stop') : $t('page.aiWorkspace.send')"
        :disabled="active ? false : !canSend || disabled"
        @click="active ? emit('stop') : emit('send')"
      >
        <SvgIcon :icon="active ? 'mdi:stop' : 'mdi:arrow-up'" class="text-20px" />
      </button>
    </div>
    <AttachmentPicker v-model:show="pickerVisible" selection-mode="multiple" :max="4" @confirm="selectImages" />
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
  background: rgb(var(--container-color));
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
