<script setup lang="ts">
import { computed, ref } from 'vue';
import { $t } from '@/locales';
import { approveAIToolConfirmation, rejectAIToolConfirmation } from '@/service/api';

const props = defineProps<{ confirmation: Api.AI.ToolConfirmation }>();
const emit = defineEmits<{ resolved: [status: 'approved' | 'rejected', result?: Api.AI.ToolResult] }>();
const loading = ref(false);
const title = computed(() => props.confirmation.presentation?.tool.title || props.confirmation.tool_id);
const description = computed(() => props.confirmation.presentation?.tool.description || '');
const summary = computed(() => props.confirmation.presentation?.summary || $t('toolConfirmation.fallbackSummary'));

async function approve() {
  loading.value = true;
  try {
    const { data, error } = await approveAIToolConfirmation(props.confirmation.id);
    emit('resolved', 'approved', !error && data ? data : undefined);
  } finally {
    loading.value = false;
  }
}
async function reject() {
  loading.value = true;
  try {
    await rejectAIToolConfirmation(props.confirmation.id);
    emit('resolved', 'rejected');
  } finally {
    loading.value = false;
  }
}
</script>

<template>
  <NCard size="small" :bordered="true" class="confirmation-card max-w-640px">
    <template #header>
      <div class="confirmation-card__header">
        <div class="confirmation-card__heading">
          <strong>{{ title }}</strong>
          <span v-if="description">{{ description }}</span>
        </div>
        <NSpace :wrap="false">
          <NTag :type="confirmation.risk === 'high' ? 'error' : 'warning'" size="small">
            {{ $t(`toolConfirmation.risk.${confirmation.risk}` as any) }}
          </NTag>
          <NTag size="small">{{ $t(`toolConfirmation.sideEffect.${confirmation.side_effect}` as any) }}</NTag>
        </NSpace>
      </div>
    </template>

    <p class="confirmation-card__summary">{{ summary }}</p>
    <NDescriptions
      v-if="confirmation.presentation?.details?.length"
      :column="1"
      label-placement="left"
      size="small"
      class="confirmation-card__details"
    >
      <NDescriptionsItem v-for="detail in confirmation.presentation.details" :key="detail.label" :label="detail.label">
        {{ detail.value }}
      </NDescriptionsItem>
    </NDescriptions>

    <NCollapse class="confirmation-card__technical">
      <NCollapseItem :title="$t('toolConfirmation.technicalDetails')" name="technical">
        <NDescriptions :column="1" label-placement="left" size="small">
          <NDescriptionsItem :label="$t('toolConfirmation.tool')">{{ confirmation.tool_id }}</NDescriptionsItem>
          <NDescriptionsItem :label="$t('toolConfirmation.callId')">{{ confirmation.tool_call_id }}</NDescriptionsItem>
          <NDescriptionsItem :label="$t('toolConfirmation.arguments')">
            <NCode :code="JSON.stringify(confirmation.arguments_summary, null, 2)" language="json" word-wrap />
          </NDescriptionsItem>
        </NDescriptions>
      </NCollapseItem>
    </NCollapse>

    <template #footer>
      <NSpace justify="end">
        <NButton :disabled="loading" @click="reject">{{ $t('toolConfirmation.reject') }}</NButton>
        <NButton type="warning" :loading="loading" :disabled="loading" @click="approve">
          {{ $t('toolConfirmation.approveOnce') }}
        </NButton>
      </NSpace>
    </template>
  </NCard>
</template>

<style scoped>
.confirmation-card {
  width: min(640px, calc(100% - 24px));
}
.confirmation-card__header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}
.confirmation-card__heading {
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 3px;
}
.confirmation-card__heading strong,
.confirmation-card__heading span,
.confirmation-card__summary {
  overflow-wrap: anywhere;
}
.confirmation-card__heading span {
  color: rgba(127, 127, 127, 0.9);
  font-size: 12px;
  font-weight: 400;
}
.confirmation-card__summary {
  margin: 0;
  font-size: 14px;
  line-height: 1.6;
}
.confirmation-card__details,
.confirmation-card__technical {
  margin-top: 12px;
}
@media (max-width: 520px) {
  .confirmation-card__header {
    flex-direction: column;
  }
}
</style>
