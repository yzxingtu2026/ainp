<script setup lang="ts">
import { defineAsyncComponent } from 'vue';
import type { ProfileExtensionRegistration } from '@/composition/contract';
import { loadProfileExtensionComponent } from '@/composition/profile-extension';
import ProfileExtensionLoadError from './profile-extension-load-error.vue';

const props = defineProps<{ registration: Readonly<ProfileExtensionRegistration> }>();

const extensionComponent = defineAsyncComponent({
  loader: () => loadProfileExtensionComponent(props.registration),
  errorComponent: ProfileExtensionLoadError,
  delay: 0,
  timeout: 15_000
});
</script>

<template>
  <component :is="extensionComponent" />
</template>
