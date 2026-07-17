<script setup lang="ts">
import { computed } from 'vue';
import { $t } from '@/locales';

defineOptions({ name: 'AgentPreviewToolCallCard' });

const props = defineProps<{
  toolName: string;
  toolCallId: string;
  request: unknown;
  response: unknown;
  hasResponse: boolean;
}>();

function formatPayload(value: unknown) {
  if (value === undefined) return '';
  if (typeof value === 'string') {
    try {
      return JSON.stringify(JSON.parse(value), null, 2);
    } catch {
      return value;
    }
  }

  return JSON.stringify(value, null, 2) ?? String(value);
}

const requestCode = computed(() => formatPayload(props.request));
const responseCode = computed(() => formatPayload(props.response));
</script>

<template>
  <NCard size="small" embedded class="preview-tool-card">
    <template #header>
      <div class="min-w-0 flex flex-wrap items-center gap-6px">
        <NTag size="small" type="info" :bordered="false">{{ $t('page.agentBuilder.toolCall') }}</NTag>
        <span class="break-all font-600">{{ toolName || $t('page.agentBuilder.unknownTool') }}</span>
        <NTag size="small" :type="hasResponse ? 'success' : 'warning'" :bordered="false">
          {{ hasResponse ? $t('page.agentBuilder.toolResponseReceived') : $t('page.agentBuilder.waitingToolResponse') }}
        </NTag>
      </div>
    </template>

    <div class="mb-8px text-12px text-gray-500">
      {{ $t('page.agentBuilder.toolCallId') }}：
      <span class="break-all font-mono">{{ toolCallId || '-' }}</span>
    </div>

    <NCollapse display-directive="show">
      <NCollapseItem :title="$t('page.agentBuilder.toolRequest')" name="request">
        <div v-if="requestCode" class="preview-tool-payload">
          <NCode :code="requestCode" language="json" word-wrap />
        </div>
        <NEmpty v-else size="small" :description="$t('page.agentBuilder.noToolRequest')" />
      </NCollapseItem>
      <NCollapseItem :title="$t('page.agentBuilder.toolResponse')" name="response">
        <div v-if="hasResponse" class="preview-tool-payload">
          <NCode :code="responseCode" language="json" word-wrap />
        </div>
        <NEmpty v-else size="small" :description="$t('page.agentBuilder.waitingToolResponse')" />
      </NCollapseItem>
    </NCollapse>
  </NCard>
</template>

<style scoped>
.preview-tool-card {
  border-left: 3px solid rgba(var(--primary-color), 0.5);
}

.preview-tool-payload {
  max-height: 320px;
  overflow: auto;
  border-radius: 6px;
  background: rgba(127, 127, 127, 0.08);
  padding: 10px;
}
</style>
