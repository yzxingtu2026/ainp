<script setup lang="ts">
import { computed, onMounted, watch } from 'vue';
import { Icon } from '@iconify/vue';
import type { TagProps } from 'naive-ui';
import { useCodeSetStore } from '@/store/modules/codeset';

defineOptions({ name: 'CodeTag' });

const props = defineProps<{ codeSet: string; value: string }>();
const store = useCodeSetStore();

const item = computed(() => store.get(props.codeSet)?.items.find(candidate => candidate.value === props.value));
const tagType = computed<TagProps['type']>(() => {
  const mapping: Record<Api.CodeSet.Semantic, TagProps['type']> = {
    default: 'default', info: 'info', success: 'success', warning: 'warning', danger: 'error', muted: 'default'
  };
  return item.value ? mapping[item.value.semantic] : 'default';
});

async function ensureLoaded() { await store.load([props.codeSet]); }
onMounted(ensureLoaded);
watch(() => props.codeSet, ensureLoaded);
</script>

<template>
  <NTag :type="tagType" size="small" :bordered="false" :class="{ 'opacity-60': !item || item.deprecated }">
    <template v-if="item?.icon" #icon><Icon :icon="item.icon" /></template>
    {{ item?.label || value }}
  </NTag>
</template>
