<script setup lang="ts">
import { AINP_LOGO_PATH } from '@/constants/brand';

defineOptions({
  name: 'SystemLogo'
});
</script>

<template>
  <img :src="AINP_LOGO_PATH" alt="AINP" class="app-logo" />
</template>

<style scoped>
.app-logo {
  display: block;
  object-fit: contain;
}
</style>
