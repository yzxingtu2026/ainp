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
    <button type="button" class="tool-call__row" @click="expanded = !expanded">
      <SvgIcon icon="mdi:api" class="tool-call__icon" />
      <span class="tool-call__name">{{ displayName }}</span>
      <span class="tool-call__status">
        <NSpin v-if="status === 'running'" :size="14" />
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
</template>

<style scoped>
.tool-call {
  border: 1px solid rgba(127, 127, 127, 0.16);
  border-radius: 8px;
  background: rgba(127, 127, 127, 0.05);
  overflow: hidden;
  transition:
    border-color 0.2s ease,
    background 0.2s ease;
}

.tool-call--running {
  border-color: rgba(var(--primary-color), 0.35);
}

.tool-call--failed {
  border-color: rgba(var(--error-color, 208, 48, 80), 0.4);
}

.tool-call__row {
  width: 100%;
  display: flex;
  align-items: center;
  gap: 8px;
  border: 0;
  background: transparent;
  padding: 8px 12px;
  color: inherit;
  font-size: 13px;
  text-align: left;
  cursor: pointer;
}

.tool-call__row:hover {
  background: rgba(127, 127, 127, 0.08);
}

.tool-call__icon {
  flex: none;
  color: rgba(var(--primary-color), 0.85);
  font-size: 16px;
}

.tool-call__name {
  min-width: 0;
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-weight: 600;
}

.tool-call__description {
  margin: 0 0 8px;
  color: rgba(127, 127, 127, 0.9);
  font-size: 12px;
}

.tool-call__status {
  flex: none;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
}

.tool-call--running .tool-call__status {
  color: rgb(var(--primary-color));
}

.tool-call--success .tool-call__status {
  color: rgb(var(--success-color, 82, 196, 26));
}

.tool-call--failed .tool-call__status {
  color: rgb(var(--error-color, 208, 48, 80));
}

.tool-call--waiting .tool-call__status {
  color: rgba(127, 127, 127, 0.85);
}

.tool-call--rehydrated .tool-call__status {
  color: rgba(127, 127, 127, 0.95);
}

.tool-call--deferred .tool-call__status {
  color: rgba(127, 127, 127, 0.95);
}

.tool-call__status-text {
  color: rgba(127, 127, 127, 0.95);
}

.tool-call__chevron {
  flex: none;
  color: rgba(127, 127, 127, 0.7);
  font-size: 18px;
  transition: transform 0.2s ease;
}

.tool-call__chevron--open {
  transform: rotate(180deg);
}

.tool-call__detail {
  border-top: 1px dashed rgba(127, 127, 127, 0.2);
  padding: 10px 12px 12px;
}

.tool-call__summary {
  margin: 0 0 8px;
  color: rgba(127, 127, 127, 1);
  font-size: 12px;
}

.tool-call__section {
  margin-top: 8px;
}

.tool-call__section:first-of-type {
  margin-top: 0;
}

.tool-call__label {
  margin-bottom: 4px;
  color: rgba(127, 127, 127, 0.85);
  font-size: 12px;
  font-weight: 600;
}

.tool-call__payload {
  max-height: 280px;
  overflow: auto;
  border-radius: 6px;
  background: rgba(127, 127, 127, 0.08);
  padding: 8px 10px;
}

.tool-call__empty {
  color: rgba(127, 127, 127, 0.7);
  font-size: 12px;
}
</style>
