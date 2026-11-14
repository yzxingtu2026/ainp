<script setup lang="ts">
import { computed, ref } from 'vue';
import { $t } from '@/locales';
import { approveAIToolConfirmation, rejectAIToolConfirmation } from '@/service/api';

defineOptions({ name: 'ToolConfirmationCard' });

const props = defineProps<{ confirmation: Api.AI.ToolConfirmation }>();
const emit = defineEmits<{ resolved: [status: 'approved' | 'rejected', result?: Api.AI.ToolResult] }>();

const pendingAction = ref<'approve' | 'reject' | null>(null);
const loading = computed(() => pendingAction.value !== null);

const isHighRisk = computed(() => props.confirmation.risk === 'high' || props.confirmation.risk === 'critical');
const shieldIcon = computed(() => (isHighRisk.value ? 'mdi:shield-alert-outline' : 'mdi:shield-lock-outline'));
const title = computed(() => props.confirmation.presentation?.tool.title || props.confirmation.tool_id);
const description = computed(() => props.confirmation.presentation?.tool.description || '');
const summary = computed(() => props.confirmation.presentation?.summary || $t('toolConfirmation.fallbackSummary'));
const argumentsCode = computed(() => JSON.stringify(props.confirmation.arguments_summary, null, 2));

async function approve() {
  if (loading.value) return;
  pendingAction.value = 'approve';
  try {
    const { data, error } = await approveAIToolConfirmation(props.confirmation.id);
    emit('resolved', 'approved', !error && data ? data : undefined);
  } finally {
    pendingAction.value = null;
  }
}

async function reject() {
  if (loading.value) return;
  pendingAction.value = 'reject';
  try {
    await rejectAIToolConfirmation(props.confirmation.id);
    emit('resolved', 'rejected');
  } finally {
    pendingAction.value = null;
  }
}
</script>

<template>
  <section
    class="confirmation-card"
    :class="isHighRisk ? 'confirmation-card--high' : 'confirmation-card--caution'"
    role="region"
    :aria-label="$t('toolConfirmation.title')"
  >
    <header class="confirmation-card__banner">
      <div class="confirmation-card__banner-main">
        <span class="confirmation-card__shield" aria-hidden="true">
          <SvgIcon :icon="shieldIcon" />
        </span>
        <div class="confirmation-card__heading">
          <span class="confirmation-card__eyebrow">{{ $t('toolConfirmation.securityGate') }}</span>
          <strong class="confirmation-card__title">{{ title }}</strong>
          <span v-if="description" class="confirmation-card__desc">{{ description }}</span>
        </div>
      </div>
      <div class="confirmation-card__tags">
        <NTag :type="isHighRisk ? 'error' : 'warning'" size="small" :bordered="false">
          {{ $t(`toolConfirmation.risk.${confirmation.risk}` as any) }}
        </NTag>
        <NTag size="small" :bordered="false">
          {{ $t(`toolConfirmation.sideEffect.${confirmation.side_effect}` as any) }}
        </NTag>
      </div>
    </header>

    <div class="confirmation-card__body">
      <p class="confirmation-card__summary">{{ summary }}</p>

      <div v-if="confirmation.presentation?.details?.length" class="confirmation-card__bento">
        <div
          v-for="detail in confirmation.presentation.details"
          :key="detail.label"
          class="confirmation-card__bento-item"
        >
          <span class="confirmation-card__bento-label">
            <span>{{ detail.label }}</span>
            <span v-if="detail.sensitive" class="confirmation-card__sensitive">
              <SvgIcon icon="mdi:lock-outline" class="text-11px" />
              {{ $t('toolConfirmation.sensitiveField') }}
            </span>
          </span>
          <span class="confirmation-card__bento-value">{{ detail.value || '-' }}</span>
        </div>
      </div>

      <NCollapse class="confirmation-card__technical">
        <NCollapseItem :title="$t('toolConfirmation.technicalDetails')" name="technical">
          <div class="confirmation-card__tech-meta">
            <div class="confirmation-card__tech-row">
              <span>{{ $t('toolConfirmation.tool') }}</span>
              <code>{{ confirmation.tool_id }}</code>
            </div>
            <div class="confirmation-card__tech-row">
              <span>{{ $t('toolConfirmation.callId') }}</span>
              <code>{{ confirmation.tool_call_id }}</code>
            </div>
          </div>
          <div class="confirmation-card__tech-code-label">{{ $t('toolConfirmation.arguments') }}</div>
          <div class="confirmation-card__tech-code">
            <NCode :code="argumentsCode" language="json" word-wrap />
          </div>
        </NCollapseItem>
      </NCollapse>
    </div>

    <footer class="confirmation-card__footer">
      <span class="confirmation-card__notice">{{ $t('toolConfirmation.singleUseNotice') }}</span>
      <div class="confirmation-card__actions">
        <NButton size="small" :loading="pendingAction === 'reject'" :disabled="loading" @click="reject">
          {{ $t('toolConfirmation.reject') }}
        </NButton>
        <NButton
          size="small"
          :type="isHighRisk ? 'error' : 'warning'"
          :loading="pendingAction === 'approve'"
          :disabled="loading"
          @click="approve"
        >
          {{ $t('toolConfirmation.approveOnce') }}
        </NButton>
      </div>
    </footer>
  </section>
</template>

<style scoped>
.confirmation-card {
  --gate-accent: 217, 119, 6;
  width: min(640px, 100%);
  min-width: 0;
  overflow: hidden;
  border: 1px solid rgba(var(--gate-accent), 0.36);
  border-radius: 14px;
  background: rgb(var(--container-bg-color));
  box-shadow:
    0 10px 28px rgba(15, 23, 42, 0.08),
    inset 0 1px 0 rgba(255, 255, 255, 0.06);
}

.confirmation-card--high {
  --gate-accent: 220, 38, 38;
}

:where(.dark) .confirmation-card--caution {
  --gate-accent: 245, 158, 11;
}

:where(.dark) .confirmation-card--high {
  --gate-accent: 248, 113, 113;
}

.confirmation-card__banner {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  justify-content: space-between;
  gap: 10px;
  border-bottom: 1px solid rgba(var(--gate-accent), 0.2);
  background: linear-gradient(
    135deg,
    rgba(var(--gate-accent), 0.13) 0%,
    rgba(var(--gate-accent), 0.04) 65%,
    transparent 100%
  );
  padding: 12px 14px;
}

.confirmation-card__banner-main {
  min-width: 0;
  flex: 1 1 200px;
  display: flex;
  align-items: flex-start;
  gap: 10px;
}

.confirmation-card__shield {
  width: 32px;
  height: 32px;
  flex: none;
  display: grid;
  place-items: center;
  border: 1px solid rgba(var(--gate-accent), 0.34);
  border-radius: 9px;
  background: rgba(var(--gate-accent), 0.14);
  color: rgb(var(--gate-accent));
  font-size: 18px;
}

.confirmation-card__heading {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.confirmation-card__eyebrow {
  font-size: 10.5px;
  font-weight: 650;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  color: rgb(var(--gate-accent));
}

.confirmation-card__title,
.confirmation-card__desc,
.confirmation-card__summary,
.confirmation-card__bento-value {
  overflow-wrap: anywhere;
}

.confirmation-card__title {
  font-size: 14px;
  font-weight: 650;
  line-height: 1.35;
}

.confirmation-card__desc {
  color: rgba(127, 127, 127, 0.92);
  font-size: 12px;
  line-height: 1.45;
}

.confirmation-card__tags {
  display: inline-flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
}

.confirmation-card__body {
  padding: 12px 14px;
}

.confirmation-card__summary {
  margin: 0;
  font-size: 13.5px;
  font-weight: 500;
  line-height: 1.6;
}

.confirmation-card__bento {
  margin-top: 11px;
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(176px, 100%), 1fr));
  gap: 8px;
}

.confirmation-card__bento-item {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 3px;
  border: 1px solid rgba(127, 127, 127, 0.16);
  border-radius: 9px;
  background: rgba(127, 127, 127, 0.045);
  padding: 8px 10px;
}

.confirmation-card__bento-label {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
  color: rgba(127, 127, 127, 0.92);
  font-size: 11px;
  font-weight: 550;
}

.confirmation-card__sensitive {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  color: rgb(var(--gate-accent));
  font-size: 10.5px;
}

.confirmation-card__bento-value {
  font-size: 12.5px;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  line-height: 1.45;
}

.confirmation-card__technical {
  margin-top: 12px;
}

.confirmation-card__tech-meta {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-bottom: 8px;
}

.confirmation-card__tech-row {
  min-width: 0;
  display: flex;
  flex-wrap: wrap;
  align-items: baseline;
  gap: 6px;
  font-size: 11.5px;
}

.confirmation-card__tech-row span {
  color: rgba(127, 127, 127, 0.88);
}

.confirmation-card__tech-row code {
  min-width: 0;
  overflow-wrap: anywhere;
  border-radius: 4px;
  background: rgba(127, 127, 127, 0.1);
  padding: 1px 5px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 11px;
}

.confirmation-card__tech-code-label {
  margin-bottom: 4px;
  color: rgba(127, 127, 127, 0.88);
  font-size: 11.5px;
  font-weight: 600;
}

.confirmation-card__tech-code {
  max-height: 220px;
  overflow: auto;
  border: 1px solid rgba(127, 127, 127, 0.16);
  border-radius: 8px;
  background: rgba(15, 23, 42, 0.04);
  padding: 8px 10px;
}

:where(.dark) .confirmation-card__tech-code {
  background: rgba(9, 13, 24, 0.65);
}

.confirmation-card__footer {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  border-top: 1px solid rgba(127, 127, 127, 0.14);
  background: rgba(127, 127, 127, 0.03);
  padding: 9px 14px;
}

.confirmation-card__notice {
  min-width: 0;
  flex: 1 1 160px;
  color: rgba(127, 127, 127, 0.88);
  font-size: 11px;
  line-height: 1.4;
}

.confirmation-card__actions {
  margin-left: auto;
  display: inline-flex;
  align-items: center;
  gap: 8px;
}
</style>
