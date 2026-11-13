<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { $t } from '@/locales';
import { toolResultDeferred, toolResultFailed, type ToolOperation } from './tool-activity';

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
const pageCaptureRunning = computed(
  () => props.active && regularOperations.value.some(op => op.toolName === 'builtin__page_capture' && !op.hasResult)
);

const statusCounts = computed(() => {
  let success = 0;
  let deferred = 0;
  let failed = 0;
  for (const op of regularOperations.value) {
    if (!op.hasResult) continue;
    if (toolResultDeferred(op.result)) deferred += 1;
    else if (toolResultFailed(op.result)) failed += 1;
    else success += 1;
  }
  return { success, deferred, failed };
});

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
        :aria-expanded="navigationExpanded"
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
        <span class="tool-activity__live-dot" aria-hidden="true" />
        <SvgIcon icon="mdi:map-search-outline" class="tool-activity__live-icon" />
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
      :aria-expanded="expanded"
      @click="expanded = !expanded"
    >
      <SvgIcon icon="mdi:format-list-checks" class="tool-activity__summary-icon" />
      <span class="tool-activity__summary-label">
        {{ $t('chat.toolProcessed', { count: regularOperations.length }) }}
      </span>
      <span v-if="statusCounts.success || statusCounts.deferred || statusCounts.failed" class="tool-activity__badges">
        <span
          v-if="statusCounts.success > 0"
          class="tool-activity__badge tool-activity__badge--success"
          :title="$t('chat.toolSuccess')"
        >
          <i aria-hidden="true" />
          {{ statusCounts.success }}
        </span>
        <span
          v-if="statusCounts.deferred > 0"
          class="tool-activity__badge tool-activity__badge--deferred"
          :title="$t('chat.toolDeferred')"
        >
          <i aria-hidden="true" />
          {{ statusCounts.deferred }}
        </span>
        <span
          v-if="statusCounts.failed > 0"
          class="tool-activity__badge tool-activity__badge--failed"
          :title="$t('chat.toolFailed')"
        >
          <i aria-hidden="true" />
          {{ statusCounts.failed }}
        </span>
      </span>
      <SvgIcon
        icon="mdi:chevron-right"
        class="tool-activity__summary-chevron"
        :class="{ 'tool-activity__summary-chevron--open': expanded }"
      />
    </button>

    <template v-else>
      <div
        v-if="regularOperations.length > 0"
        class="tool-activity__live"
        :class="{ 'tool-activity__live--settled': allSettled }"
      >
        <span class="tool-activity__live-dot" aria-hidden="true" />
        <SvgIcon
          :icon="pageCaptureRunning ? 'mdi:monitor-screenshot' : 'mdi:lightning-bolt-outline'"
          class="tool-activity__live-icon"
        />
        <span>
          {{
            pageCaptureRunning
              ? $t('chat.pageCapturing')
              : allSettled
                ? $t('chat.toolProcessed', { count: regularOperations.length })
                : $t('chat.toolCall')
          }}
        </span>
      </div>
    </template>

    <Transition name="tool-activity-expand">
      <div v-if="visible && regularOperations.length > 0" class="tool-activity__items">
        <ToolCallCard
          v-for="operation in regularOperations"
          :key="operation.toolCallId || operation.toolName"
          :tool-name="operation.toolName"
          :presentation="operation.presentation"
          :tool-call-id="operation.toolCallId"
          :request="operation.request"
          :result="operation.result"
          :has-result="operation.hasResult"
          :retried-after-deferral="operation.retriedAfterDeferral"
          :pending="active"
        />
      </div>
    </Transition>
  </div>
</template>

<style scoped>
.tool-activity {
  margin: 8px 0;
}

.tool-activity__navigation + .tool-activity__summary,
.tool-activity__navigation + .tool-activity__live {
  margin-top: 8px;
}

.tool-activity__summary {
  max-width: 100%;
  display: inline-flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  border: 1px solid rgba(127, 127, 127, 0.18);
  border-radius: 999px;
  background: rgba(127, 127, 127, 0.06);
  padding: 5px 11px;
  color: rgba(var(--base-text-color), 0.78);
  font-size: 12px;
  font-variant-numeric: tabular-nums;
  cursor: pointer;
  transition:
    border-color 0.2s ease,
    background 0.2s ease,
    color 0.2s ease;
}

.tool-activity__summary:hover,
.tool-activity__summary:focus-visible {
  border-color: rgba(var(--primary-color), 0.42);
  background: rgba(var(--primary-color), 0.08);
  color: rgb(var(--primary-color));
  outline: none;
}

.tool-activity__summary-icon {
  flex: none;
  font-size: 14px;
  color: rgba(var(--primary-color), 0.88);
}

.tool-activity__summary-label {
  font-weight: 500;
}

.tool-activity__badges {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding-left: 4px;
  border-left: 1px solid rgba(127, 127, 127, 0.2);
}

.tool-activity__badge {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  font-size: 11px;
  font-weight: 600;
  line-height: 1;
}

.tool-activity__badge i {
  width: 6px;
  height: 6px;
  border-radius: 50%;
}

.tool-activity__badge--success {
  color: #10b981;
}

.tool-activity__badge--success i {
  background: #10b981;
}

.tool-activity__badge--deferred {
  color: #d97706;
}

:where(.dark) .tool-activity__badge--deferred {
  color: #fbbf24;
}

.tool-activity__badge--deferred i {
  background: #f59e0b;
}

.tool-activity__badge--failed {
  color: #ef4444;
}

.tool-activity__badge--failed i {
  background: #ef4444;
}

.tool-activity__summary-chevron {
  flex: none;
  font-size: 15px;
  opacity: 0.78;
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
  border: 1px solid rgba(var(--primary-color), 0.2);
  border-radius: 999px;
  background: rgba(var(--primary-color), 0.07);
  padding: 4px 10px;
  color: rgba(var(--base-text-color), 0.86);
  font-size: 12px;
  font-weight: 500;
  font-variant-numeric: tabular-nums;
}

.tool-activity__live--settled {
  border-color: rgba(127, 127, 127, 0.18);
  background: rgba(127, 127, 127, 0.06);
}

.tool-activity__live-dot {
  width: 6px;
  height: 6px;
  flex: none;
  border-radius: 50%;
  background: rgb(var(--primary-color));
  box-shadow: 0 0 6px rgba(var(--primary-color), 0.6);
  animation: tool-rail-pulse 1.5s ease-in-out infinite;
}

.tool-activity__live--settled .tool-activity__live-dot {
  background: #10b981;
  box-shadow: none;
  animation: none;
}

.tool-activity__live-icon {
  flex: none;
  font-size: 14px;
  color: rgb(var(--primary-color));
}

/* Chain-of-Action vertical neural rail connecting step cards */
.tool-activity__items {
  position: relative;
  margin-top: 8px;
  padding-left: 18px;
  display: flex;
  flex-direction: column;
  gap: 7px;
}

.tool-activity__items::before {
  content: '';
  position: absolute;
  top: 10px;
  bottom: 10px;
  left: 6px;
  width: 1.5px;
  border-radius: 999px;
  background: linear-gradient(
    180deg,
    rgba(var(--primary-color), 0.5) 0%,
    rgba(8, 145, 178, 0.35) 55%,
    rgba(127, 127, 127, 0.16) 100%
  );
  pointer-events: none;
}

@keyframes tool-rail-pulse {
  0%,
  100% {
    opacity: 1;
    transform: scale(1);
  }
  50% {
    opacity: 0.45;
    transform: scale(0.85);
  }
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
  .tool-activity__summary,
  .tool-activity__summary-chevron,
  .tool-activity-expand-enter-active,
  .tool-activity-expand-leave-active {
    transition: none;
  }

  .tool-activity__live-dot {
    animation: none;
  }
}
</style>
