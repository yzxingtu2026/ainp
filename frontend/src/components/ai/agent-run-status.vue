<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import CodeTag from '@/components/common/code-tag.vue';
import { CODESET_KEYS } from '@/constants/codeset';
import { isBudgetFailureCategory, runNotice } from './run-budget-message';

const props = defineProps<{ run: Api.AI.AgentRun }>();
const emit = defineEmits<{ cancel: []; resume: []; retry: [] }>();
const { t } = useI18n();
const notice = computed(() => runNotice(props.run));
const cancellable = computed(() => ['queued', 'running', 'waiting_confirmation', 'paused'].includes(props.run.status));
</script>

<template>
  <NSpace align="center" justify="space-between" :wrap="true">
    <NSpace align="center">
      <CodeTag :code-set="CODESET_KEYS.agentRunStatus" :value="run.status" />
      <span class="text-12px text-gray-500">
        {{ t('page.aiRun.progress', { step: run.current_step, model: run.model_calls, tool: run.tool_calls }) }}
      </span>
      <NText v-if="notice" :type="notice === 'budgetFailed' ? 'error' : 'warning'">
        {{ t(`page.aiRun.${notice}`) }}
      </NText>
      <NText v-else-if="run.error_category && !isBudgetFailureCategory(run.error_category)" type="error">
        {{ run.error_category }}
      </NText>
    </NSpace>
    <NSpace>
      <NButton v-if="cancellable" size="small" @click="emit('cancel')">
        {{ t('page.aiRun.cancel') }}
      </NButton>
      <NButton
        v-if="run.status === 'waiting_confirmation' || run.status === 'paused'"
        size="small"
        type="primary"
        @click="emit('resume')"
      >
        {{ t('page.aiRun.resume') }}
      </NButton>
      <NButton v-if="run.status === 'failed'" size="small" type="primary" @click="emit('retry')">
        {{ t('page.aiRun.retry') }}
      </NButton>
    </NSpace>
  </NSpace>
</template>
