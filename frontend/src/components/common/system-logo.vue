<script setup lang="ts">
import { useBrandStore } from '@/store/modules/brand';

defineOptions({
  name: 'SystemLogo'
});

const brandStore = useBrandStore();
</script>

<template>
  <img :src="brandStore.logo" :alt="brandStore.name" class="app-logo" @error="brandStore.fallbackLogo" />
</template>

<style scoped>
.app-logo {
  display: block;
  object-fit: contain;
}
</style>
