<script setup lang="ts">
import { computed } from 'vue';
import { formatDateTime, type DateTimeDisplayFormat } from '@/utils/datetime';

defineOptions({ name: 'DateTimeText' });

const props = withDefaults(
  defineProps<{
    value?: string | number | Date | null;
    format?: DateTimeDisplayFormat;
  }>(),
  {
    format: 'datetime'
  }
);

const text = computed(() => formatDateTime(props.value, props.format));
</script>

<template>
  <span class="whitespace-nowrap">{{ text }}</span>
</template>
