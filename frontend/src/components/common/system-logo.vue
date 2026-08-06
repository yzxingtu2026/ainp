<script setup lang="ts">
import { AINP_LOGO_PATH, APP_BRAND_NAME } from '@/constants/brand';

defineOptions({
  name: 'SystemLogo'
});
</script>

<template>
  <img :src="AINP_LOGO_PATH" :alt="APP_BRAND_NAME" class="app-logo" />
</template>

<style scoped>
.app-logo {
  display: block;
  object-fit: contain;
}
</style>
