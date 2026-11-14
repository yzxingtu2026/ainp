<script setup lang="ts">
import { computed, ref } from 'vue';
import { $t } from '@/locales';
import {
  formatToolPayload,
  toolResultDeferred,
  toolResultFailed,
  toolResultRehydrated,
  toolResultSummary
} from './tool-activity';

defineOptions({ name: 'ToolCallCard' });

const props = defineProps<{
  toolName: string;
  presentation?: Api.AI.ToolPresentation;
  toolCallId: string;
  /** tool_call 调用参数 */
  request: unknown;
  /** 配对 tool_result 结果，未返回时为空 */
  result: unknown;
  hasResult: boolean;
  retriedAfterDeferral?: boolean;
  /** 所属运行是否仍在进行（无结果时用于区分「执行中」与「等待中」） */
  pending: boolean;
}>();

const expanded = ref(false);

type ToolCallStatus = 'running' | 'success' | 'failed' | 'waiting' | 'rehydrated' | 'deferred';

const status = computed<ToolCallStatus>(() => {
  if (props.hasResult) {
    if (toolResultDeferred(props.result)) return 'deferred';
    if (toolResultFailed(props.result)) return 'failed';
    if (toolResultRehydrated(props.result)) return 'rehydrated';
    return 'success';
  }
  return props.pending ? 'running' : 'waiting';
});

const statusIcon = computed(() => {
  if (status.value === 'success') return 'mdi:check-circle';
  if (status.value === 'failed') return 'mdi:close-circle';
  if (status.value === 'waiting') return 'mdi:clock-outline';
  if (status.value === 'rehydrated') return 'mdi:restore';
  if (status.value === 'deferred') return 'mdi:clock-minus-outline';
  return '';
});

const statusText = computed(() => {
  if (status.value === 'running') return $t('chat.toolRunning');
  if (status.value === 'success') {
    return props.retriedAfterDeferral ? $t('chat.toolSuccessAfterDeferral') : $t('chat.toolSuccess');
  }
  if (status.value === 'failed') return $t('chat.toolFailed');
  if (status.value === 'rehydrated') return $t('chat.toolRehydrated');
  if (status.value === 'deferred') return $t('chat.toolDeferred');
  return $t('chat.toolWaiting');
});

const summary = computed(() => toolResultSummary(props.result));
const requestCode = computed(() => formatToolPayload(props.request));
const resultCode = computed(() => formatToolPayload(props.result));
const displayName = computed(() => props.presentation?.title || props.toolName || $t('page.agentBuilder.unknownTool'));
</script>

<template>
  <div class="tool-call" :class="`tool-call--${status}`">
    <span class="tool-call__node" aria-hidden="true" />
    <div class="tool-call__surface">
      <button type="button" class="tool-call__row" :aria-expanded="expanded" @click="expanded = !expanded">
        <span class="tool-call__icon-wrap" aria-hidden="true">
          <SvgIcon icon="mdi:api" class="tool-call__icon" />
        </span>
        <span class="tool-call__name" :title="displayName">{{ displayName }}</span>
        <span class="tool-call__status">
          <NSpin v-if="status === 'running'" :size="12" />
          <SvgIcon v-else :icon="statusIcon" />
          <span class="tool-call__status-text">{{ statusText }}</span>
        </span>
        <SvgIcon icon="mdi:chevron-down" class="tool-call__chevron" :class="{ 'tool-call__chevron--open': expanded }" />
      </button>

      <div v-if="expanded" class="tool-call__detail">
        <p v-if="presentation?.description" class="tool-call__description">{{ presentation.description }}</p>
        <p v-if="summary" class="tool-call__summary">{{ summary }}</p>
        <NCollapse>
          <NCollapseItem :title="$t('chat.toolTechnicalDetails')" name="technical">
            <NDescriptions :column="1" label-placement="left" size="small">
              <NDescriptionsItem :label="$t('chat.toolIdentifier')">{{ toolName || '-' }}</NDescriptionsItem>
              <NDescriptionsItem :label="$t('chat.toolCallIdentifier')">{{ toolCallId || '-' }}</NDescriptionsItem>
            </NDescriptions>
            <div class="tool-call__section">
              <div class="tool-call__label">{{ $t('chat.toolArguments') }}</div>
              <div v-if="requestCode" class="tool-call__payload">
                <NCode :code="requestCode" language="json" word-wrap />
              </div>
              <span v-else class="tool-call__empty">-</span>
            </div>
            <div class="tool-call__section">
              <div class="tool-call__label">{{ $t('chat.toolResult') }}</div>
              <div v-if="hasResult && resultCode" class="tool-call__payload">
                <NCode :code="resultCode" language="json" word-wrap />
              </div>
              <span v-else class="tool-call__empty">{{ $t('chat.toolWaiting') }}</span>
            </div>
          </NCollapseItem>
        </NCollapse>
      </div>
    </div>
  </div>
</template>

<style scoped>
.tool-call {
  position: relative;
}

.tool-call__node {
  position: absolute;
  top: 14px;
  left: -15px;
  width: 8px;
  height: 8px;
  border: 1.5px solid rgb(var(--container-bg-color));
  border-radius: 50%;
  background: rgba(127, 127, 127, 0.55);
  pointer-events: none;
}

.tool-call--running .tool-call__node {
  background: rgb(var(--primary-color));
  box-shadow: 0 0 0 3px rgb(var(--primary-color) / 0.22);
}

.tool-call--success .tool-call__node {
  background: #10b981;
}

.tool-call--failed .tool-call__node {
  background: #ef4444;
}

.tool-call--deferred .tool-call__node {
  background: #f59e0b;
}

.tool-call--rehydrated .tool-call__node {
  background: #0891b2;
}

.tool-call__surface {
  border: 1px solid rgba(127, 127, 127, 0.16);
  border-radius: 10px;
  background: rgb(var(--container-bg-color) / 86%);
  overflow: hidden;
  transition:
    border-color 0.2s ease,
    box-shadow 0.2s ease,
    background 0.2s ease;
}

.tool-call--running .tool-call__surface {
  border-color: rgb(var(--primary-color) / 0.42);
  box-shadow: 0 0 0 1px rgb(var(--primary-color) / 0.1);
  animation: tool-call-pulse 2s ease-in-out infinite;
}

.tool-call--failed .tool-call__surface {
  border-color: rgba(239, 68, 68, 0.36);
}

@keyframes tool-call-pulse {
  0%,
  100% {
    border-color: rgb(var(--primary-color) / 0.45);
    box-shadow: 0 0 0 1px rgb(var(--primary-color) / 0.12);
  }
  50% {
    border-color: rgba(8, 145, 178, 0.42);
    box-shadow: 0 0 12px rgb(var(--primary-color) / 0.14);
  }
}

.tool-call__row {
  width: 100%;
  display: flex;
  align-items: center;
  gap: 8px;
  border: 0;
  background: transparent;
  padding: 7px 10px;
  color: inherit;
  font-size: 12.5px;
  text-align: left;
  cursor: pointer;
}

.tool-call__row:hover,
.tool-call__row:focus-visible {
  background: rgba(127, 127, 127, 0.06);
  outline: none;
}

.tool-call__icon-wrap {
  width: 22px;
  height: 22px;
  flex: none;
  display: grid;
  place-items: center;
  border-radius: 6px;
  background: rgb(var(--primary-color) / 0.1);
}

.tool-call__icon {
  color: rgb(var(--primary-color));
  font-size: 13px;
}

.tool-call__name {
  min-width: 0;
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-weight: 600;
}

.tool-call__status {
  flex: none;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  border-radius: 999px;
  background: rgba(127, 127, 127, 0.08);
  padding: 2px 8px;
  font-size: 11px;
  font-weight: 550;
  font-variant-numeric: tabular-nums;
}

.tool-call--running .tool-call__status {
  background: rgb(var(--primary-color) / 0.11);
  color: rgb(var(--primary-color));
}

.tool-call--success .tool-call__status {
  background: rgba(16, 185, 129, 0.12);
  color: #10b981;
}

.tool-call--failed .tool-call__status {
  background: rgba(239, 68, 68, 0.12);
  color: #ef4444;
}

.tool-call--waiting .tool-call__status {
  color: rgba(127, 127, 127, 0.9);
}

.tool-call--rehydrated .tool-call__status {
  background: rgba(8, 145, 178, 0.12);
  color: #0891b2;
}

:where(.dark) .tool-call--rehydrated .tool-call__status {
  color: #22d3ee;
}

.tool-call--deferred .tool-call__status {
  background: rgba(245, 158, 11, 0.12);
  color: #d97706;
}

:where(.dark) .tool-call--deferred .tool-call__status {
  color: #fbbf24;
}

.tool-call__status-text {
  color: inherit;
}

.tool-call__chevron {
  flex: none;
  color: rgba(127, 127, 127, 0.7);
  font-size: 17px;
  transition: transform 0.2s ease;
}

.tool-call__chevron--open {
  transform: rotate(180deg);
}

.tool-call__detail {
  border-top: 1px solid rgba(127, 127, 127, 0.14);
  background: rgba(127, 127, 127, 0.025);
  padding: 10px 12px 12px;
}

.tool-call__description {
  margin: 0 0 8px;
  color: rgba(127, 127, 127, 0.92);
  font-size: 12px;
  line-height: 1.5;
}

.tool-call__summary {
  margin: 0 0 8px;
  color: rgb(var(--base-text-color) / 0.88);
  font-size: 12px;
  line-height: 1.55;
}

.tool-call__section {
  margin-top: 8px;
}

.tool-call__section:first-of-type {
  margin-top: 6px;
}

.tool-call__label {
  margin-bottom: 4px;
  color: rgba(127, 127, 127, 0.88);
  font-size: 11.5px;
  font-weight: 600;
}

.tool-call__payload {
  max-height: 260px;
  overflow: auto;
  border: 1px solid rgba(127, 127, 127, 0.16);
  border-radius: 8px;
  background: rgba(15, 23, 42, 0.04);
  padding: 8px 10px;
}

:where(.dark) .tool-call__payload {
  background: rgba(9, 13, 24, 0.65);
}

.tool-call__empty {
  color: rgba(127, 127, 127, 0.7);
  font-size: 12px;
}

@media (prefers-reduced-motion: reduce) {
  .tool-call__surface,
  .tool-call__chevron {
    transition: none;
  }

  .tool-call--running .tool-call__surface {
    animation: none;
  }
}
</style>
