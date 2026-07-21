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
const navigationExpanded = ref(false);

const visible = computed(() => props.active || expanded.value);
const navigationOperations = computed(() =>
  props.operations.filter(operation => operation.toolName === 'builtin__navigation_search')
);
const regularOperations = computed(() =>
  props.operations.filter(operation => operation.toolName !== 'builtin__navigation_search')
);

function resultHasNavigationItem(result: unknown) {
  if (!result || typeof result !== 'object' || Array.isArray(result)) return false;
  const data = (result as Record<string, unknown>).data;
  if (!data || typeof data !== 'object' || Array.isArray(data)) return false;
  const items = (data as Record<string, unknown>).items;
  return (
    Array.isArray(items) &&
    items.some(item => {
      if (!item || typeof item !== 'object' || Array.isArray(item)) return false;
      const value = item as Record<string, unknown>;
      return typeof value.menu_id === 'string' && typeof value.path === 'string';
    })
  );
}

const navigationHasResults = computed(() =>
  navigationOperations.value.some(operation => resultHasNavigationItem(operation.result))
);
const navigationVisible = computed(() => props.active || navigationHasResults.value || navigationExpanded.value);

const allSettled = computed(
  () => regularOperations.value.length > 0 && regularOperations.value.every(op => op.hasResult)
);

// 运行结束后自动收起为汇总行
watch(
  () => props.active,
  value => {
    if (!value) {
      expanded.value = false;
      navigationExpanded.value = false;
    }
  }
);
</script>

<template>
  <div v-if="operations.length > 0" class="tool-activity">
    <div v-if="navigationOperations.length > 0" class="tool-activity__navigation">
      <button
        v-if="!active && !navigationHasResults"
        type="button"
        class="tool-activity__summary"
        @click="navigationExpanded = !navigationExpanded"
      >
        <SvgIcon icon="mdi:map-search-outline" class="tool-activity__summary-icon" />
        <span>{{ $t('chat.navigationProcessed', { count: navigationOperations.length }) }}</span>
        <SvgIcon
          icon="mdi:chevron-right"
          class="tool-activity__summary-chevron"
          :class="{ 'tool-activity__summary-chevron--open': navigationExpanded }"
        />
      </button>
      <div v-else-if="active" class="tool-activity__live">
        <SvgIcon icon="mdi:map-search-outline" />
        <span>{{ $t('chat.navigationSearching') }}</span>
      </div>
      <Transition name="tool-activity-expand">
        <NavigationResultCard
          v-if="navigationVisible"
          :result="navigationOperations.map(operation => operation.result)"
          :pending="active && navigationOperations.some(operation => !operation.hasResult)"
        />
      </Transition>
    </div>
    <button
      v-if="!active && regularOperations.length > 0"
      type="button"
      class="tool-activity__summary"
      @click="expanded = !expanded"
    >
      <SvgIcon icon="mdi:format-list-checks" class="tool-activity__summary-icon" />
      <span>{{ $t('chat.toolProcessed', { count: regularOperations.length }) }}</span>
      <SvgIcon
        icon="mdi:chevron-right"
        class="tool-activity__summary-chevron"
        :class="{ 'tool-activity__summary-chevron--open': expanded }"
      />
    </button>

    <template v-else>
      <div v-if="regularOperations.length > 0" class="tool-activity__live">
        <SvgIcon icon="mdi:lightning-bolt-outline" />
        <span>
          {{ allSettled ? $t('chat.toolProcessed', { count: regularOperations.length }) : $t('chat.toolCall') }}
        </span>
      </div>
    </template>

    <Transition name="tool-activity-expand">
      <div v-if="visible && regularOperations.length > 0" class="tool-activity__items">
        <ToolCallCard
          v-for="operation in regularOperations"
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

.tool-activity__navigation + .tool-activity__summary,
.tool-activity__navigation + .tool-activity__live {
  margin-top: 6px;
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
  transition:
    background 0.2s ease,
    color 0.2s ease;
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
