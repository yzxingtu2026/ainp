<script setup lang="ts">
import { ref, watch } from 'vue';
import {
  NButton,
  NEmpty,
  NInput,
  NList,
  NListItem,
  NModal,
  NSpace,
  NSpin,
  NTag,
  NThing
} from 'naive-ui';
import { $t } from '@/locales';
import { searchAIAgentTools } from '@/service/api';

defineOptions({ name: 'AgentToolPickerModal' });

const props = defineProps<{
  show: boolean;
  boundToolIds: string[];
  canBind: boolean;
  /** 绑定成功后父级刷新已绑定列表 */
  bindTool: (toolId: string) => Promise<void>;
}>();

const emit = defineEmits<{
  'update:show': [value: boolean];
}>();

const keyword = ref('');
const loading = ref(false);
const bindingId = ref('');
const candidates = ref<Api.AI.ToolCandidate[]>([]);

watch(
  () => props.show,
  async visible => {
    if (!visible) return;
    keyword.value = '';
    await search();
  }
);

async function search() {
  loading.value = true;
  try {
    const { data } = await searchAIAgentTools({ query: keyword.value.trim(), limit: 50 });
    candidates.value = data || [];
  } finally {
    loading.value = false;
  }
}

async function onBind(toolId: string) {
  if (!props.canBind || props.boundToolIds.includes(toolId) || bindingId.value) return;
  bindingId.value = toolId;
  try {
    await props.bindTool(toolId);
  } finally {
    bindingId.value = '';
  }
}

function riskType(risk: string): 'error' | 'warning' | 'success' {
  if (risk === 'high' || risk === 'critical') return 'error';
  if (risk === 'medium') return 'warning';
  return 'success';
}

function close() {
  emit('update:show', false);
}
</script>

<template>
  <NModal
    :show="show"
    preset="card"
    class="w-820px max-w-95vw"
    :title="$t('page.agentBuilder.toolPickerTitle')"
    :mask-closable="true"
    @update:show="emit('update:show', $event)"
  >
    <div class="mb-12px text-12px text-gray-500">{{ $t('page.agentBuilder.toolPickerHint') }}</div>
    <NSpace class="mb-12px">
      <NInput
        v-model:value="keyword"
        clearable
        class="w-360px max-w-full"
        :placeholder="$t('page.agentBuilder.toolSearchPlaceholder')"
        @keyup.enter="search"
      />
      <NButton type="primary" :loading="loading" @click="search">{{ $t('common.search') }}</NButton>
    </NSpace>
    <NSpin :show="loading">
      <div class="max-h-60vh overflow-y-auto">
        <NEmpty v-if="candidates.length === 0" :description="$t('page.agentBuilder.noToolCandidates')" />
        <NList v-else bordered>
          <NListItem v-for="tool in candidates" :key="tool.id">
            <NThing :title="tool.name" :description="tool.summary">
              <template #header-extra>
                <NSpace align="center">
                  <NTag size="small">{{ tool.source }}</NTag>
                  <NTag v-if="tool.binding_group === 'tool_discovery'" size="small" type="info">
                    {{ $t('page.agentBuilder.toolDiscovery') }}
                  </NTag>
                  <NTag v-if="tool.loading_mode === 'conditional'" size="small" type="info">
                    {{ $t('page.agentBuilder.conditionalLoading') }}
                  </NTag>
                  <NTag size="small" :type="riskType(tool.risk)">
                    {{ $t(`toolConfirmation.risk.${tool.risk}` as App.I18n.I18nKey) }}
                  </NTag>
                  <NTag size="small">
                    {{ $t(`toolConfirmation.sideEffect.${tool.side_effect}` as App.I18n.I18nKey) }}
                  </NTag>
                  <NButton
                    size="small"
                    type="primary"
                    :disabled="boundToolIds.includes(tool.id) || !canBind"
                    :loading="bindingId === tool.id"
                    @click="onBind(tool.id)"
                  >
                    {{
                      boundToolIds.includes(tool.id)
                        ? $t('page.agentBuilder.bound')
                        : $t('page.agentBuilder.bind')
                    }}
                  </NButton>
                </NSpace>
              </template>
            </NThing>
            <div class="mt-4px text-12px text-gray-400">{{ tool.id }}</div>
          </NListItem>
        </NList>
      </div>
    </NSpin>
    <template #footer>
      <NSpace justify="end">
        <NButton @click="close">{{ $t('common.close') }}</NButton>
      </NSpace>
    </template>
  </NModal>
</template>
