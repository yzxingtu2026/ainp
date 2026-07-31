<script setup lang="ts">
import { computed, ref, useAttrs } from 'vue';
import type { SelectInst, SelectOption } from 'naive-ui';
import { NSelect } from 'naive-ui';
import type { AppSelectOption, EmptyStringPolicy, SelectModelValue } from './select-value';
import { normalizeSelectValue } from './select-value';

defineOptions({ name: 'AppSelect', inheritAttrs: false });

const props = withDefaults(
  defineProps<{
    value?: SelectModelValue;
    options?: AppSelectOption[];
    valueField?: string;
    childrenField?: string;
    emptyStringPolicy?: EmptyStringPolicy;
  }>(),
  {
    value: undefined,
    options: () => [],
    valueField: 'value',
    childrenField: 'children',
    emptyStringPolicy: 'auto'
  }
);

const emit = defineEmits<{
  'update:value': [value: Exclude<SelectModelValue, undefined>, option: SelectOption | null | SelectOption[]];
}>();

defineSlots<{
  default?: () => unknown;
  header?: () => unknown;
  action?: () => unknown;
  empty?: () => unknown;
  arrow?: () => unknown;
}>();

const attrs = useAttrs();
const selectRef = ref<SelectInst | null>(null);
const naiveOptions = computed(() => props.options as SelectOption[]);
const viewValue = computed(() =>
  normalizeSelectValue(props.value, props.options, {
    emptyStringPolicy: props.emptyStringPolicy,
    valueField: props.valueField,
    childrenField: props.childrenField
  })
);

function handleUpdate(value: Exclude<SelectModelValue, undefined>, option: SelectOption | null | SelectOption[]) {
  emit('update:value', value, option);
}

defineExpose({
  focus: () => selectRef.value?.focus(),
  blur: () => selectRef.value?.blur(),
  focusInput: () => selectRef.value?.focusInput(),
  blurInput: () => selectRef.value?.blurInput()
});
</script>

<template>
  <NSelect
    ref="selectRef"
    v-bind="attrs"
    :value="viewValue"
    :options="naiveOptions"
    :value-field="valueField"
    :children-field="childrenField"
    @update:value="handleUpdate"
  >
    <template v-if="$slots.default" #default><slot /></template>
    <template v-if="$slots.header" #header><slot name="header" /></template>
    <template v-if="$slots.action" #action><slot name="action" /></template>
    <template v-if="$slots.empty" #empty><slot name="empty" /></template>
    <template v-if="$slots.arrow" #arrow><slot name="arrow" /></template>
  </NSelect>
</template>
