<script setup lang="ts">
import { APP_BRAND_NAME } from '@/constants/brand';
import { buildInfo } from '@/utils/build-info';

defineOptions({
  name: 'GlobalFooter'
});
</script>

<template>
  <DarkModeContainer class="h-full flex-center">
    <div class="flex items-center gap-12px">
      <a href="https://github.com/yzxingtu2026/ainp" target="_blank" rel="noopener noreferrer">
        Copyright © 2026 {{ APP_BRAND_NAME }}
      </a>
      <span>v{{ buildInfo.version.replace(/^v/, '') }}</span>
    </div>
  </DarkModeContainer>
</template>

<style scoped></style>
