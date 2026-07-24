<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import CodeTag from '@/components/common/code-tag.vue';
import { CODESET_KEYS } from '@/constants/codeset';

defineProps<{ run: Api.AI.AgentRun }>();
const emit = defineEmits<{ cancel: []; resume: []; retry: [] }>();
const { t } = useI18n();
</script>

<template>
  <NSpace align="center" justify="space-between" :wrap="true">
    <NSpace align="center">
      <CodeTag :code-set="CODESET_KEYS.agentRunStatus" :value="run.status" />
      <span class="text-12px text-gray-500">{{ t('page.aiRun.progress', { step: run.current_step, model: run.model_calls, tool: run.tool_calls }) }}</span>
      <NText v-if="run.error_category" type="error">{{ run.error_category }}</NText>
    </NSpace>
    <NSpace>
      <NButton v-if="run.status === 'queued' || run.status === 'running' || run.status === 'waiting_confirmation' || run.status === 'paused'" size="small" @click="emit('cancel')">{{ t('page.aiRun.cancel') }}</NButton>
      <NButton v-if="run.status === 'waiting_confirmation' || run.status === 'paused'" size="small" type="primary" @click="emit('resume')">{{ t('page.aiRun.resume') }}</NButton>
      <NButton v-if="run.status === 'failed'" size="small" type="primary" @click="emit('retry')">{{ t('page.aiRun.retry') }}</NButton>
    </NSpace>
  </NSpace>
</template>
