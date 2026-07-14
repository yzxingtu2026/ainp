<script setup lang="ts">
import { computed, onMounted, watch } from 'vue';
import { useCodeSetStore } from '@/store/modules/codeset';

defineOptions({ name: 'CodeSelect' });

const props = withDefaults(defineProps<{ codeSet: string; disabled?: boolean; filterable?: boolean }>(), { filterable: true });
const model = defineModel<string | null>();
const store = useCodeSetStore();
const options = computed(() => (store.get(props.codeSet)?.items || [])
  .slice()
  .sort((a, b) => a.order - b.order)
  .map(item => ({ label: item.label, value: item.value, disabled: item.deprecated })));

async function ensureLoaded() { await store.load([props.codeSet]); }
onMounted(ensureLoaded);
watch(() => props.codeSet, ensureLoaded);
</script>

<template>
  <NSelect
    v-model:value="model"
    :options="options"
    :loading="store.loading"
    :disabled="disabled"
    :filterable="filterable"
    clearable
  />
</template>
