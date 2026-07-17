<script setup lang="ts">
import { ref } from 'vue';
import { $t } from '@/locales';
import { approveAIToolConfirmation, rejectAIToolConfirmation } from '@/service/api';

const props=defineProps<{confirmation:Api.AI.ToolConfirmation}>();
const emit=defineEmits<{resolved:[status:'approved'|'rejected',result?:Api.AI.ToolResult]}>();
const loading=ref(false);
async function approve(){loading.value=true;try{const {data,error}=await approveAIToolConfirmation(props.confirmation.id);if(!error&&data)emit('resolved','approved',data)}finally{loading.value=false}}
async function reject(){loading.value=true;try{const {error}=await rejectAIToolConfirmation(props.confirmation.id);if(!error)emit('resolved','rejected')}finally{loading.value=false}}
</script>

<template>
  <NCard size="small" :bordered="true" class="max-w-640px">
    <template #header><NSpace align="center"><span>{{ $t('toolConfirmation.title') }}</span><NTag :type="confirmation.risk==='high'?'error':'warning'" size="small">{{ $t(`toolConfirmation.risk.${confirmation.risk}` as any) }}</NTag><NTag size="small">{{ $t(`toolConfirmation.sideEffect.${confirmation.side_effect}` as any) }}</NTag></NSpace></template>
    <NDescriptions :column="1" label-placement="left" size="small">
      <NDescriptionsItem :label="$t('toolConfirmation.tool')">{{ confirmation.tool_id }}</NDescriptionsItem>
      <NDescriptionsItem :label="$t('toolConfirmation.arguments')"><NCode :code="JSON.stringify(confirmation.arguments_summary,null,2)" language="json" word-wrap /></NDescriptionsItem>
    </NDescriptions>
    <template #footer><NSpace justify="end"><NButton :disabled="loading" @click="reject">{{ $t('toolConfirmation.reject') }}</NButton><NButton type="warning" :loading="loading" @click="approve">{{ $t('toolConfirmation.approveOnce') }}</NButton></NSpace></template>
  </NCard>
</template>
