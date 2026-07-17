<script setup lang="ts">
import { computed } from 'vue';
import { $t } from '@/locales';

defineOptions({ name: 'PreviewComposer' });

const props = defineProps<{
  disabled?: boolean;
  canSend?: boolean;
  active?: boolean;
}>();

const model = defineModel<string>({ default: '' });

const emit = defineEmits<{
  send: [];
  stop: [];
  clear: [];
}>();

const showStop = computed(() => Boolean(props.active));

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
</script>

<template>
  <div class="preview-composer flex items-center gap-10px">
    <button
      type="button"
      class="preview-composer__clear"
      :title="$t('page.agentBuilder.clearPreview')"
      :aria-label="$t('page.agentBuilder.clearPreview')"
      @click="emit('clear')"
    >
      <SvgIcon icon="mdi:broom" class="text-18px" />
    </button>

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
        <button type="button" class="preview-composer__plus" tabindex="-1" disabled aria-hidden="true">
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
  cursor: default;
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
