<script setup lang="ts">
import { ref } from 'vue';

const value = ref('primary');
const options = [
  { label: '主要业务空间', value: 'primary' },
  { label: '协作业务空间', value: 'collaboration' }
];
</script>

<template>
  <div class="flex items-center gap-8px">
    <NText depth="3" class="whitespace-nowrap text-12px">业务上下文</NText>
    <NSelect v-model:value="value" class="w-180px" size="small" :options="options" aria-label="选择业务上下文" />
  </div>
</template>
