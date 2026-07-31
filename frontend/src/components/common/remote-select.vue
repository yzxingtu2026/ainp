<script setup lang="ts">
import { ref } from 'vue';
import { request } from '@/service/request';

defineOptions({ name: 'RemoteSelect' });

const props = defineProps<{
  source: Api.Setting.RemoteOptionSource;
}>();

const modelValue = defineModel<string | null>('value');
const loading = ref(false);
const loaded = ref(false);
const options = ref<Array<{ label: string; value: string }>>([]);

function valueAtPath(value: unknown, path?: string): unknown {
  if (!path) return value;
  return path.split('.').reduce<unknown>((current, key) => {
    if (!current || typeof current !== 'object') return undefined;
    return (current as Record<string, unknown>)[key];
  }, value);
}

async function loadOptions() {
  if (loading.value || loaded.value) return;
  loading.value = true;
  try {
    const { data, error } = await request<unknown>({
      url: props.source.endpoint,
      method: 'post',
      data: props.source.params || {}
    });
    if (error) return;
    const rows = valueAtPath(data, props.source.response_path);
    options.value = Array.isArray(rows)
      ? rows.flatMap(row => {
          if (!row || typeof row !== 'object') return [];
          const item = row as Record<string, unknown>;
          const value = item[props.source.value_field];
          const label = item[props.source.label_field];
          return typeof value === 'string' && typeof label === 'string' ? [{ value, label }] : [];
        })
      : [];
    loaded.value = true;
  } finally {
    loading.value = false;
  }
}
</script>

<template>
  <AppSelect
    v-model:value="modelValue"
    filterable
    clearable
    :loading="loading"
    :options="options"
    @focus="loadOptions"
  />
</template>
