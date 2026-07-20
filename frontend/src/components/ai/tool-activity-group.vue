<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { $t } from '@/locales';
import type { ToolOperation } from './tool-activity';

defineOptions({ name: 'ToolActivityGroup' });

const props = defineProps<{
  operations: ToolOperation[];
  /** 所属运行是否仍在进行：进行中逐条实时展示，结束后折叠为汇总行 */
  active: boolean;
}>();

const expanded = ref(false);

const visible = computed(() => props.active || expanded.value);

const allSettled = computed(() => props.operations.length > 0 && props.operations.every(op => op.hasResult));

// 运行结束后自动收起为汇总行
watch(
  () => props.active,
  value => {
    if (!value) expanded.value = false;
  }
);
</script>

<template>
  <div v-if="operations.length > 0" class="tool-activity">
    <button
      v-if="!active"
      type="button"
      class="tool-activity__summary"
      @click="expanded = !expanded"
    >
      <SvgIcon icon="mdi:format-list-checks" class="tool-activity__summary-icon" />
      <span>{{ $t('chat.toolProcessed', { count: operations.length }) }}</span>
      <SvgIcon
        icon="mdi:chevron-right"
        class="tool-activity__summary-chevron"
        :class="{ 'tool-activity__summary-chevron--open': expanded }"
      />
    </button>

    <template v-else>
      <div class="tool-activity__live">
        <SvgIcon icon="mdi:lightning-bolt-outline" />
        <span>{{ allSettled ? $t('chat.toolProcessed', { count: operations.length }) : $t('chat.toolCall') }}</span>
      </div>
    </template>

    <Transition name="tool-activity-expand">
      <div v-if="visible" class="tool-activity__items">
        <ToolCallCard
          v-for="operation in operations"
          :key="operation.toolCallId || operation.toolName"
          :tool-name="operation.toolName"
          :request="operation.request"
          :result="operation.result"
          :has-result="operation.hasResult"
          :pending="active"
        />
      </div>
    </Transition>
  </div>
</template>

<style scoped>
.tool-activity {
  margin-top: 8px;
}

.tool-activity__summary {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  border: 0;
  border-radius: 999px;
  background: rgba(127, 127, 127, 0.1);
  padding: 6px 12px;
  color: rgba(127, 127, 127, 1);
  font-size: 12px;
  cursor: pointer;
  transition: background 0.2s ease, color 0.2s ease;
}

.tool-activity__summary:hover {
  background: rgba(var(--primary-color), 0.1);
  color: rgb(var(--primary-color));
}

.tool-activity__summary-icon {
  font-size: 15px;
}

.tool-activity__summary-chevron {
  font-size: 16px;
  transition: transform 0.2s ease;
}

.tool-activity__summary-chevron--open {
  transform: rotate(90deg);
}

.tool-activity__live {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 6px;
  color: rgba(127, 127, 127, 0.95);
  font-size: 12px;
}

.tool-activity__items {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.tool-activity-expand-enter-active,
.tool-activity-expand-leave-active {
  overflow: hidden;
  transition:
    opacity 0.22s ease,
    transform 0.22s ease;
}

.tool-activity-expand-enter-from,
.tool-activity-expand-leave-to {
  opacity: 0;
  transform: translateY(-4px);
}

@media (prefers-reduced-motion: reduce) {
  .tool-activity-expand-enter-active,
  .tool-activity-expand-leave-active {
    transition: none;
  }
}
</style>
