<script setup lang="ts">
import { buildInfo } from '@/utils/build-info';

defineOptions({
  name: 'GlobalFooter'
});
</script>

<template>
  <DarkModeContainer class="h-full flex-center">
    <div class="flex items-center gap-12px">
      <a href="https://github.com/yzxingtu2026/ainp" target="_blank" rel="noopener noreferrer">
        Copyright © 2026 AINP
      </a>
      <span>v{{ buildInfo.version.replace(/^v/, '') }}</span>
    </div>
  </DarkModeContainer>
</template>

<style scoped></style>
