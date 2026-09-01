<script setup lang="ts">
import { defineAsyncComponent, onErrorCaptured, ref } from 'vue';
import type { ShellExtensionRegistration } from '@/composition/contract';
import { loadShellExtensionComponent } from '@/composition/shell-extension';
import ShellExtensionError from './shell-extension-error.vue';
import ShellExtensionLoading from './shell-extension-loading.vue';

const props = defineProps<{ registration: Readonly<ShellExtensionRegistration> }>();
const renderFailed = ref(false);

const extensionComponent = defineAsyncComponent({
  loader: () => loadShellExtensionComponent(props.registration),
  loadingComponent: ShellExtensionLoading,
  errorComponent: ShellExtensionError,
  delay: 0,
  timeout: 15_000
});

onErrorCaptured(() => {
  renderFailed.value = true;
  return false;
});
</script>

<template>
  <div class="shell-extension-host" :data-shell-extension-id="registration.id">
    <ShellExtensionError v-if="renderFailed" />
    <component :is="extensionComponent" v-else />
  </div>
</template>

<style scoped>
.shell-extension-host {
  min-width: 0;
}
</style>
