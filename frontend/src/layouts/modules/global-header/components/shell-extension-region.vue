<script setup lang="ts">
import type { ShellExtensionRegistration } from '@/composition/contract';
import ShellExtensionHost from './shell-extension-host.vue';

defineProps<{
  extensions: readonly Readonly<ShellExtensionRegistration>[];
  compact: boolean;
}>();
</script>

<template>
  <NPopover v-if="compact" trigger="click" placement="bottom-end" :show-arrow="false">
    <template #trigger>
      <NButton
        quaternary
        circle
        :title="$t('common.businessContext')"
        :aria-label="$t('common.businessContext')"
        data-testid="shell-extension-trigger"
      >
        <template #icon><SvgIcon icon="mdi:briefcase-outline" /></template>
      </NButton>
    </template>
    <section class="shell-extension-popover" :aria-label="$t('common.businessContext')">
      <NText depth="3" class="shell-extension-title">{{ $t('common.businessContext') }}</NText>
      <div class="shell-extension-stack">
        <ShellExtensionHost v-for="extension in extensions" :key="extension.id" :registration="extension" />
      </div>
    </section>
  </NPopover>
  <section v-else class="shell-extension-inline" :aria-label="$t('common.businessContext')">
    <ShellExtensionHost v-for="extension in extensions" :key="extension.id" :registration="extension" />
  </section>
</template>

<style scoped>
.shell-extension-inline {
  display: flex;
  flex: 0 1 auto;
  align-items: center;
  gap: 8px;
  min-width: 0;
  max-width: min(42vw, 720px);
  padding-inline: 8px;
  overflow-x: auto;
  scrollbar-width: none;
}

.shell-extension-inline::-webkit-scrollbar {
  display: none;
}

.shell-extension-popover {
  width: min(360px, calc(100vw - 24px));
  max-width: 100%;
}

.shell-extension-title {
  display: block;
  margin-bottom: 8px;
  font-size: 12px;
}

.shell-extension-stack {
  display: flex;
  flex-direction: column;
  gap: 8px;
  max-height: min(60vh, 480px);
  overflow: auto;
}
</style>
