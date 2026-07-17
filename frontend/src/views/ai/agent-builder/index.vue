<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue';
import { useRoute } from 'vue-router';
import {
  NAlert,
  NButton,
  NCollapse,
  NCollapseItem,
  NDrawer,
  NDrawerContent,
  NEmpty,
  NForm,
  NFormItem,
  NInput,
  NInputNumber,
  NList,
  NListItem,
  NSelect,
  NSpace,
  NSpin,
  NTabPane,
  NTabs,
  NTag,
  NThing,
  useDialog,
  useMessage
} from 'naive-ui';
import { AI_ACTIONS } from '@/constants/ai';
import { useRouterPush } from '@/hooks/common/router';
import { $t } from '@/locales';
import {
  bindAIAgentTool,
  createAIAgent,
  fetchAIAgent,
  fetchAIAgentDiagnostics,
  fetchAIAgentTools,
  fetchAIAgentVersions,
  fetchAIAliases,
  fetchAIDeployments,
  publishAIAgentVersion,
  searchAIAgentTools,
  unbindAIAgentTool,
  updateAIAgent
} from '@/service/api';
import { useAuthStore } from '@/store/modules/auth';
import { createDemoMockPreview, createIdleMockPreview, type MockPreviewState } from './mock-preview';

defineOptions({ name: 'AIAgentBuilder' });

const route = useRoute();
const { routerPushByKey } = useRouterPush();
const auth = useAuthStore();
const message = useMessage();
const dialog = useDialog();

const can = (permission: string) =>
  auth.userInfo.buttons.includes(permission) || auth.userInfo.buttons.includes('*');

const loading = ref(false);
const hydrating = ref(false);
const savingMeta = ref(false);
const publishing = ref(false);
const toolsLoading = ref(false);
const dirty = ref(false);
const mobileTab = ref<'prompt' | 'config' | 'preview'>('prompt');
const versionsVisible = ref(false);
/** 中间栏折叠分组：默认只展开基本信息 */
const configExpandedNames = ref<Array<string | number>>(['basic']);
const collapsed = reactive({ left: false, right: false });
const leftWidth = ref(360);
const rightWidth = ref(360);

const agent = ref<Api.AI.AgentDetail | null>(null);
const versions = ref<Api.AI.AgentVersion[]>([]);
const diagnostics = ref<Api.AI.AgentDiagnostics | null>(null);
const boundTools = ref<Api.AI.AgentToolBinding[]>([]);
const toolCandidates = ref<Api.AI.ToolCandidate[]>([]);
const aliases = ref<Api.AI.Alias[]>([]);
const deployments = ref<Api.AI.Deployment[]>([]);
const toolKeyword = ref('');
const runInstructions = ref('');
const previewInput = ref('');
const preview = ref<MockPreviewState>(createIdleMockPreview(key => $t(key as App.I18n.I18nKey)));

const draft = reactive({
  code: '',
  name: '',
  description: '',
  source: 'personal' as Api.AI.AgentSource,
  visibility: 'private' as Api.AI.AgentVisibility,
  model_alias: '',
  tenant_instructions: '',
  temperature: 0.7,
  top_p: 1,
  max_output_tokens: 2048,
  revision: 0
});

const isCreate = computed(() => !agent.value);
const managedFields = computed(() => new Set(agent.value?.managed_fields || []));
const requiredTools = computed(() => new Set(agent.value?.required_tool_selectors || []));
const isManaged = (field: string) => managedFields.value.has(field);
const boundToolIDs = computed(() => new Set(boundTools.value.map(item => item.tool_id)));
const canPublish = computed(
  () =>
    Boolean(draft.name && draft.code && draft.model_alias) &&
    (isCreate.value ? can(AI_ACTIONS.agentCreate) : can(AI_ACTIONS.agentVersionPublish)) &&
    (!diagnostics.value || diagnostics.value.healthy || isCreate.value)
);
const canEditMeta = computed(() => isCreate.value || can(AI_ACTIONS.agentUpdate));
const instructionLimit = 12000;
const instructionError = computed(() => {
  if (draft.tenant_instructions.length > instructionLimit) {
    return $t('page.agentBuilder.instructionTooLong');
  }
  return '';
});

const modelOptions = computed(() => {
  const enabled = new Map(deployments.value.filter(item => item.status === 'enabled').map(item => [item.id, item]));
  return aliases.value.flatMap(alias => {
    const deployment = enabled.get(alias.deployment_id);
    if (!deployment || deployment.model_type !== 'chat') return [];
    const caps = (deployment.capabilities_json?.features || []).slice(0, 3).join(', ');
    return [
      {
        label: `${alias.code} · ${deployment.display_name}${caps ? ` (${caps})` : ''}`,
        value: alias.code
      }
    ];
  });
});

function markDirty() {
  if (!hydrating.value && !loading.value) dirty.value = true;
}

async function loadModels() {
  const [aliasResult, deploymentResult] = await Promise.all([
    fetchAIAliases(),
    fetchAIDeployments({ page: 1, page_size: 100, status: 'enabled' })
  ]);
  aliases.value = aliasResult.data || [];
  deployments.value = deploymentResult.data?.items || [];
}

async function loadAgent(id: string) {
  loading.value = true;
  hydrating.value = true;
  try {
    const [agentResult, versionResult, toolResult, diagnosticResult] = await Promise.all([
      fetchAIAgent(id),
      fetchAIAgentVersions(id),
      fetchAIAgentTools(id),
      fetchAIAgentDiagnostics(id)
    ]);
    if (agentResult.error || !agentResult.data) {
      message.error($t('page.agentBuilder.loadFailed'));
      return;
    }
    agent.value = agentResult.data;
    versions.value = versionResult.data || [];
    boundTools.value = toolResult.data || [];
    diagnostics.value = diagnosticResult.data || null;
    const current = versions.value.find(item => item.version === agent.value?.current_version);
    Object.assign(draft, {
      code: agent.value.code,
      name: agent.value.name,
      description: agent.value.description,
      source: agent.value.source,
      visibility: agent.value.visibility,
      model_alias: current?.model_alias || '',
      tenant_instructions: current?.tenant_instructions || '',
      temperature: Number(current?.generation_config?.temperature ?? 0.7),
      top_p: Number(current?.generation_config?.top_p ?? 1),
      max_output_tokens: Number(current?.generation_config?.max_output_tokens ?? 2048),
      revision: agent.value.revision
    });
    dirty.value = false;
    preview.value = createIdleMockPreview(key => $t(key as App.I18n.I18nKey));
  } finally {
    loading.value = false;
    queueMicrotask(() => {
      hydrating.value = false;
    });
  }
}

async function refreshDiagnostics() {
  if (!agent.value) return;
  const { data } = await fetchAIAgentDiagnostics(agent.value.id);
  diagnostics.value = data || null;
}

async function searchTools() {
  toolsLoading.value = true;
  try {
    const { data } = await searchAIAgentTools({ query: toolKeyword.value, limit: 10 });
    toolCandidates.value = data || [];
  } finally {
    toolsLoading.value = false;
  }
}

async function bindTool(toolID: string) {
  if (!agent.value || !can(AI_ACTIONS.agentToolBind)) return;
  const { error } = await bindAIAgentTool(agent.value.id, toolID);
  if (!error) {
    const { data } = await fetchAIAgentTools(agent.value.id);
    boundTools.value = data || [];
    await refreshDiagnostics();
  }
}

async function unbindTool(toolID: string) {
  if (!agent.value || !can(AI_ACTIONS.agentToolUnbind)) return;
  if (requiredTools.value.has(toolID)) {
    message.warning($t('page.agentBuilder.requiredToolReadonly'));
    return;
  }
  const { error } = await unbindAIAgentTool(agent.value.id, toolID);
  if (!error) {
    const { data } = await fetchAIAgentTools(agent.value.id);
    boundTools.value = data || [];
    await refreshDiagnostics();
  }
}

function generationConfig() {
  return {
    temperature: draft.temperature,
    top_p: draft.top_p,
    max_output_tokens: draft.max_output_tokens
  };
}

async function saveMetadata() {
  if (!agent.value || !canEditMeta.value) return;
  savingMeta.value = true;
  try {
    const { data, error } = await updateAIAgent({
      id: agent.value.id,
      name: draft.name,
      description: draft.description,
      avatar_attachment_id: agent.value.avatar_attachment_id ?? null,
      revision: draft.revision
    });
    if (error || !data) {
      if (String(error).includes('revision') || String(error).includes('conflict')) {
        message.error($t('page.agentBuilder.revisionConflict'));
      }
      return;
    }
    agent.value = { ...agent.value, ...data };
    draft.revision = data.revision;
    message.success($t('page.agentBuilder.metaSaved'));
  } finally {
    savingMeta.value = false;
  }
}

async function publishVersion() {
  if (!canPublish.value || instructionError.value) {
    if (instructionError.value) message.error(instructionError.value);
    return;
  }
  publishing.value = true;
  try {
    if (isCreate.value) {
      const { data, error } = await createAIAgent({
        code: draft.code,
        name: draft.name,
        description: draft.description,
        source: draft.source,
        visibility: draft.visibility,
        model_alias: draft.model_alias,
        tenant_instructions: draft.tenant_instructions,
        generation_config: generationConfig(),
        tool_policy: {}
      });
      if (error || !data) return;
      dirty.value = false;
      message.success($t('page.agentBuilder.createSuccess'));
      await routerPushByKey('ai_agent-builder', { query: { agentId: data.id } });
      await loadAgent(data.id);
      return;
    }
    if (dirty.value && canEditMeta.value) {
      await saveMetadata();
    }
    const { data, error } = await publishAIAgentVersion({
      agent_id: agent.value!.id,
      model_alias: draft.model_alias,
      tenant_instructions: draft.tenant_instructions,
      generation_config: generationConfig(),
      tool_policy: {},
      revision: draft.revision
    });
    if (error || !data) {
      message.error($t('page.agentBuilder.publishFailed'));
      return;
    }
    dirty.value = false;
    message.success($t('page.agentBuilder.publishSuccess', { version: data.version }));
    await loadAgent(agent.value!.id);
  } finally {
    publishing.value = false;
  }
}

function runMockPreview() {
  if (!diagnostics.value?.healthy && agent.value) {
    message.warning($t('page.agentBuilder.previewBlocked'));
    return;
  }
  const text = previewInput.value.trim() || runInstructions.value.trim();
  if (!text) {
    message.warning($t('page.agentBuilder.previewNeedInput'));
    return;
  }
  preview.value = createDemoMockPreview(text, key => $t(key as App.I18n.I18nKey));
  preview.value.status = 'streaming';
  window.setTimeout(() => {
    if (preview.value.status === 'streaming') preview.value.status = 'completed';
  }, 600);
}

function stopMockPreview() {
  if (preview.value.status === 'streaming') {
    preview.value = { ...preview.value, status: 'stopped', runSummary: $t('page.agentBuilder.mockStoppedSummary') };
  }
}

function clearMockPreview() {
  previewInput.value = '';
  preview.value = createIdleMockPreview(key => $t(key as App.I18n.I18nKey));
}

function leaveBuilder() {
  if (!dirty.value) {
    routerPushByKey('ai_agents');
    return;
  }
  dialog.warning({
    title: $t('page.agentBuilder.leaveTitle'),
    content: $t('page.agentBuilder.leaveContent'),
    positiveText: $t('page.agentBuilder.leaveConfirm'),
    negativeText: $t('common.cancel'),
    onPositiveClick: () => routerPushByKey('ai_agents')
  });
}

function onBeforeUnload(event: BeforeUnloadEvent) {
  if (!dirty.value) return;
  event.preventDefault();
  event.returnValue = '';
}

watch(
  () => ({ ...draft, runInstructions: runInstructions.value }),
  () => {
    if (!loading.value) markDirty();
  },
  { deep: true }
);

watch(
  () => route.query.agentId,
  async id => {
    await loadModels();
    if (typeof id === 'string' && id) {
      await loadAgent(id);
      await searchTools();
    } else {
      agent.value = null;
      versions.value = [];
      boundTools.value = [];
      diagnostics.value = null;
      Object.assign(draft, {
        code: '',
        name: '',
        description: '',
        source: 'personal',
        visibility: 'private',
        model_alias: '',
        tenant_instructions: '',
        temperature: 0.7,
        top_p: 1,
        max_output_tokens: 2048,
        revision: 0
      });
      dirty.value = false;
      preview.value = createIdleMockPreview(key => $t(key as App.I18n.I18nKey));
    }
  },
  { immediate: true }
);

onMounted(() => window.addEventListener('beforeunload', onBeforeUnload));
onBeforeUnmount(() => window.removeEventListener('beforeunload', onBeforeUnload));

function startResize(side: 'left' | 'right', event: MouseEvent) {
  const startX = event.clientX;
  const start = side === 'left' ? leftWidth.value : rightWidth.value;
  function onMove(e: MouseEvent) {
    const delta = e.clientX - startX;
    if (side === 'left') leftWidth.value = Math.min(520, Math.max(260, start + delta));
    else rightWidth.value = Math.min(520, Math.max(260, start - delta));
  }
  function onUp() {
    window.removeEventListener('mousemove', onMove);
    window.removeEventListener('mouseup', onUp);
  }
  window.addEventListener('mousemove', onMove);
  window.addEventListener('mouseup', onUp);
}
</script>

<template>
  <div class="h-screen flex flex-col overflow-hidden bg-[var(--n-color)] text-14px">
    <header class="flex shrink-0 items-center justify-between gap-12px border-b px-16px py-10px">
      <NSpace align="center">
        <NButton quaternary @click="leaveBuilder">{{ $t('page.agentBuilder.back') }}</NButton>
        <div class="min-w-0">
          <div class="truncate text-16px font-600">
            {{ isCreate ? $t('page.agentBuilder.createTitle') : draft.name || $t('page.agentBuilder.editTitle') }}
          </div>
          <div class="text-12px text-gray-500">
            <span v-if="dirty">{{ $t('page.agentBuilder.dirty') }}</span>
            <span v-else>{{ $t('page.agentBuilder.clean') }}</span>
            <span v-if="agent"> · v{{ agent.current_version }} · rev {{ draft.revision }}</span>
          </div>
        </div>
      </NSpace>
      <NSpace>
        <NButton class="lt-md:hidden" @click="collapsed.left = !collapsed.left">
          {{ collapsed.left ? $t('page.agentBuilder.showLeft') : $t('page.agentBuilder.hideLeft') }}
        </NButton>
        <NButton class="lt-md:hidden" @click="collapsed.right = !collapsed.right">
          {{ collapsed.right ? $t('page.agentBuilder.showRight') : $t('page.agentBuilder.hideRight') }}
        </NButton>
        <NButton v-if="agent" @click="versionsVisible = true">{{ $t('page.agentBuilder.versions') }}</NButton>
        <NButton
          v-if="agent && canEditMeta"
          :loading="savingMeta"
          :disabled="!dirty"
          @click="saveMetadata"
        >
          {{ $t('page.agentBuilder.saveMeta') }}
        </NButton>
        <NButton type="primary" :loading="publishing" :disabled="!canPublish || Boolean(instructionError)" @click="publishVersion">
          {{ isCreate ? $t('page.agentBuilder.createPublish') : $t('page.agentBuilder.publishVersion') }}
        </NButton>
      </NSpace>
    </header>

    <NSpin :show="loading" class="min-h-0 flex-1 overflow-hidden [&_.n-spin-content]:h-full [&_.n-spin-content]:min-h-0">
      <div class="hidden h-full min-h-0 md:flex">
        <section
          v-show="!collapsed.left"
          class="min-h-0 flex flex-col overflow-hidden border-r"
          :style="{ width: `${leftWidth}px` }"
        >
          <div class="shrink-0 border-b px-12px py-8px font-600">{{ $t('page.agentBuilder.panePrompt') }}</div>
          <div class="min-h-0 flex-1 overflow-y-auto px-12px py-10px">
            <NForm label-placement="top">
              <NFormItem :show-label="false">
                <NInput
                  v-model:value="draft.tenant_instructions"
                  type="textarea"
                  :rows="10"
                  :maxlength="instructionLimit"
                  show-count
                  :placeholder="$t('page.agentBuilder.tenantPlaceholder')"
                />
              </NFormItem>
              <NFormItem :label="$t('page.agentBuilder.runInstructions')">
                <NInput
                  v-model:value="runInstructions"
                  type="textarea"
                  :rows="4"
                  :placeholder="$t('page.agentBuilder.runPlaceholder')"
                />
                <template #feedback>{{ $t('page.agentBuilder.runHint') }}</template>
              </NFormItem>
              <NAlert v-if="instructionError" type="error" class="mb-12px">{{ instructionError }}</NAlert>
            </NForm>
          </div>
        </section>
        <div
          v-show="!collapsed.left"
          class="w-4px cursor-col-resize bg-transparent hover:bg-primary/30"
          @mousedown="startResize('left', $event)"
        />

        <section class="min-h-0 min-w-0 flex flex-1 flex-col overflow-hidden">
          <div class="shrink-0 border-b px-12px py-8px font-600">{{ $t('page.agentBuilder.paneConfig') }}</div>
          <div class="min-h-0 flex-1 overflow-y-auto px-12px py-10px">
            <NForm label-placement="top">
              <NCollapse v-model:expanded-names="configExpandedNames" display-directive="show">
                <NCollapseItem :title="$t('page.agentBuilder.groupBasic')" name="basic">
                  <div class="grid grid-cols-2 gap-12px">
                    <NFormItem :label="$t('page.agentBuilder.name')">
                      <NInput v-model:value="draft.name" :disabled="!canEditMeta" />
                    </NFormItem>
                    <NFormItem :label="$t('page.agentBuilder.code')">
                      <NInput v-model:value="draft.code" :disabled="!isCreate || isManaged('code')" />
                    </NFormItem>
                  </div>
                  <NFormItem :label="$t('page.agentBuilder.description')">
                    <NInput
                      v-model:value="draft.description"
                      type="textarea"
                      :rows="2"
                      :maxlength="512"
                      :disabled="!canEditMeta"
                    />
                  </NFormItem>
                  <div class="grid grid-cols-2 gap-12px">
                    <NFormItem :label="$t('page.agentBuilder.source')">
                      <NSelect
                        v-model:value="draft.source"
                        :disabled="!isCreate || isManaged('source')"
                        :options="[
                          { label: $t('page.agentBuilder.sourcePersonal'), value: 'personal' },
                          { label: $t('page.agentBuilder.sourceTenant'), value: 'tenant' }
                        ]"
                        @update:value="value => (draft.visibility = value === 'tenant' ? 'tenant' : 'private')"
                      />
                    </NFormItem>
                    <NFormItem :label="$t('page.agentBuilder.visibility')">
                      <NSelect
                        v-model:value="draft.visibility"
                        :disabled="!isCreate || isManaged('visibility')"
                        :options="[
                          { label: $t('page.agentBuilder.visibilityPrivate'), value: 'private' },
                          { label: $t('page.agentBuilder.visibilityTenant'), value: 'tenant' }
                        ]"
                      />
                    </NFormItem>
                  </div>
                  <NFormItem :label="$t('page.agentBuilder.modelAlias')">
                    <NSelect
                      v-model:value="draft.model_alias"
                      filterable
                      clearable
                      :options="modelOptions"
                      :placeholder="$t('page.agentBuilder.modelAliasPlaceholder')"
                    />
                  </NFormItem>
                </NCollapseItem>

                <NCollapseItem :title="$t('page.agentBuilder.generationParams')" name="generation">
                  <div class="grid grid-cols-3 gap-12px">
                    <NFormItem :label="$t('page.agentBuilder.temperature')">
                      <NInputNumber v-model:value="draft.temperature" :min="0" :max="2" :step="0.1" class="w-full" />
                    </NFormItem>
                    <NFormItem :label="$t('page.agentBuilder.topP')">
                      <NInputNumber v-model:value="draft.top_p" :min="0" :max="1" :step="0.05" class="w-full" />
                    </NFormItem>
                    <NFormItem :label="$t('page.agentBuilder.maxOutputTokens')">
                      <NInputNumber v-model:value="draft.max_output_tokens" :min="1" :max="1000000" class="w-full" />
                    </NFormItem>
                  </div>
                </NCollapseItem>

                <NCollapseItem :title="$t('page.agentBuilder.tools')" name="tools">
                  <template v-if="agent">
                    <NSpace class="mb-12px">
                      <NInput
                        v-model:value="toolKeyword"
                        clearable
                        class="w-280px"
                        :placeholder="$t('page.agentBuilder.toolSearchPlaceholder')"
                        @keyup.enter="searchTools"
                      />
                      <NButton :loading="toolsLoading" @click="searchTools">{{ $t('common.search') }}</NButton>
                    </NSpace>
                    <div class="mb-8px text-12px text-gray-500">{{ $t('page.agentBuilder.boundTools') }}</div>
                    <NEmpty v-if="boundTools.length === 0" :description="$t('page.agentBuilder.noBoundTools')" />
                    <NList v-else bordered class="mb-12px max-h-180px overflow-auto">
                      <NListItem v-for="binding in boundTools" :key="binding.id">
                        <NSpace justify="space-between" class="w-full">
                          <NSpace>
                            <span>{{ binding.tool_id }}</span>
                            <NTag v-if="requiredTools.has(binding.tool_id)" size="small" type="warning">
                              {{ $t('page.agentBuilder.requiredTool') }}
                            </NTag>
                          </NSpace>
                          <NButton
                            v-if="can(AI_ACTIONS.agentToolUnbind) && !requiredTools.has(binding.tool_id)"
                            size="small"
                            type="error"
                            ghost
                            @click="unbindTool(binding.tool_id)"
                          >
                            {{ $t('page.agentBuilder.unbind') }}
                          </NButton>
                        </NSpace>
                      </NListItem>
                    </NList>
                    <div class="mb-8px text-12px text-gray-500">{{ $t('page.agentBuilder.candidateTools') }}</div>
                    <NSpin :show="toolsLoading">
                      <NEmpty v-if="toolCandidates.length === 0" :description="$t('page.agentBuilder.noToolCandidates')" />
                      <NList v-else bordered class="max-h-240px overflow-auto">
                        <NListItem v-for="tool in toolCandidates" :key="tool.id">
                          <NThing :title="tool.name" :description="tool.summary">
                            <template #header-extra>
                              <NSpace>
                                <NTag size="small">{{ tool.source }}</NTag>
                                <NTag
                                  size="small"
                                  :type="
                                    tool.risk === 'high' || tool.risk === 'critical'
                                      ? 'error'
                                      : tool.risk === 'medium'
                                        ? 'warning'
                                        : 'success'
                                  "
                                >
                                  {{ $t(`toolConfirmation.risk.${tool.risk}` as App.I18n.I18nKey) }}
                                </NTag>
                                <NTag size="small">
                                  {{ $t(`toolConfirmation.sideEffect.${tool.side_effect}` as App.I18n.I18nKey) }}
                                </NTag>
                                <NButton
                                  size="small"
                                  type="primary"
                                  :disabled="boundToolIDs.has(tool.id) || !can(AI_ACTIONS.agentToolBind)"
                                  @click="bindTool(tool.id)"
                                >
                                  {{ boundToolIDs.has(tool.id) ? $t('page.agentBuilder.bound') : $t('page.agentBuilder.bind') }}
                                </NButton>
                              </NSpace>
                            </template>
                          </NThing>
                        </NListItem>
                      </NList>
                    </NSpin>
                  </template>
                  <NAlert v-else type="info">{{ $t('page.agentBuilder.toolsAfterCreate') }}</NAlert>
                </NCollapseItem>

                <NCollapseItem :title="$t('page.agentBuilder.diagnostics')" name="diagnostics">
                  <NAlert
                    v-if="diagnostics"
                    :type="diagnostics.healthy ? 'success' : 'warning'"
                    :title="
                      diagnostics.healthy
                        ? $t('page.agentBuilder.diagnosticsHealthy')
                        : $t('page.agentBuilder.diagnosticsUnhealthy')
                    "
                  >
                    <div v-for="issue in diagnostics.issues" :key="`${issue.code}-${issue.resource || ''}`">
                      {{ issue.code }}：{{ issue.message }}
                    </div>
                  </NAlert>
                  <NAlert v-else-if="isCreate" type="info">{{ $t('page.agentBuilder.diagnosticsAfterCreate') }}</NAlert>
                </NCollapseItem>
              </NCollapse>
            </NForm>
          </div>
        </section>

        <div
          v-show="!collapsed.right"
          class="w-4px cursor-col-resize bg-transparent hover:bg-primary/30"
          @mousedown="startResize('right', $event)"
        />
        <section
          v-show="!collapsed.right"
          class="min-h-0 flex flex-col overflow-hidden border-l"
          :style="{ width: `${rightWidth}px` }"
        >
          <div class="shrink-0 border-b px-12px py-8px font-600">
            {{ $t('page.agentBuilder.panePreview') }}
            <NTag size="small" type="warning" class="ml-8px">{{ $t('page.agentBuilder.mockTag') }}</NTag>
          </div>
          <div class="min-h-0 flex-1 overflow-y-auto px-12px py-10px">
            <NAlert type="warning" class="mb-12px">{{ $t('page.agentBuilder.mockBanner') }}</NAlert>
            <div class="mb-8px text-12px text-gray-500">{{ preview.runSummary }} · {{ preview.status }}</div>
            <div class="mb-12px flex-col gap-8px">
              <div
                v-for="item in preview.messages"
                :key="item.id"
                class="rounded-8px px-10px py-8px"
                :class="item.role === 'user' ? 'bg-primary/10' : 'bg-gray-500/10'"
              >
                <div class="mb-4px text-12px text-gray-500">{{ item.role }}{{ item.kind ? ` / ${item.kind}` : '' }}</div>
                <div class="whitespace-pre-wrap break-words">{{ item.text }}</div>
              </div>
            </div>
            <NInput
              v-model:value="previewInput"
              type="textarea"
              :rows="3"
              :placeholder="$t('page.agentBuilder.previewPlaceholder')"
              class="mb-8px"
            />
            <NSpace>
              <NButton type="primary" :disabled="Boolean(agent && diagnostics && !diagnostics.healthy)" @click="runMockPreview">
                {{ $t('page.agentBuilder.runMock') }}
              </NButton>
              <NButton @click="stopMockPreview">{{ $t('page.agentBuilder.stopMock') }}</NButton>
              <NButton @click="clearMockPreview">{{ $t('page.agentBuilder.clearMock') }}</NButton>
            </NSpace>
          </div>
        </section>
      </div>

      <div class="h-full min-h-0 flex flex-col md:hidden">
        <NTabs v-model:value="mobileTab" type="segment" class="shrink-0 px-12px pt-8px">
          <NTabPane name="prompt" :tab="$t('page.agentBuilder.panePrompt')" />
          <NTabPane name="config" :tab="$t('page.agentBuilder.paneConfig')" />
          <NTabPane name="preview" :tab="$t('page.agentBuilder.panePreview')" />
        </NTabs>
        <div class="min-h-0 flex-1 overflow-y-auto px-12px py-10px">
          <template v-if="mobileTab === 'prompt'">
            <NForm label-placement="top">
              <NFormItem :show-label="false">
                <NInput
                  v-model:value="draft.tenant_instructions"
                  type="textarea"
                  :rows="8"
                  :maxlength="instructionLimit"
                  show-count
                  :placeholder="$t('page.agentBuilder.tenantPlaceholder')"
                />
              </NFormItem>
              <NFormItem :label="$t('page.agentBuilder.runInstructions')">
                <NInput v-model:value="runInstructions" type="textarea" :rows="3" />
              </NFormItem>
            </NForm>
          </template>
          <template v-else-if="mobileTab === 'config'">
            <NForm label-placement="top">
              <NCollapse v-model:expanded-names="configExpandedNames" display-directive="show">
                <NCollapseItem :title="$t('page.agentBuilder.groupBasic')" name="basic">
                  <NFormItem :label="$t('page.agentBuilder.name')"><NInput v-model:value="draft.name" /></NFormItem>
                  <NFormItem :label="$t('page.agentBuilder.code')"><NInput v-model:value="draft.code" :disabled="!isCreate" /></NFormItem>
                  <NFormItem :label="$t('page.agentBuilder.modelAlias')">
                    <NSelect v-model:value="draft.model_alias" filterable :options="modelOptions" />
                  </NFormItem>
                </NCollapseItem>
                <NCollapseItem :title="$t('page.agentBuilder.generationParams')" name="generation">
                  <NFormItem :label="$t('page.agentBuilder.temperature')">
                    <NInputNumber v-model:value="draft.temperature" :min="0" :max="2" :step="0.1" class="w-full" />
                  </NFormItem>
                  <NFormItem :label="$t('page.agentBuilder.topP')">
                    <NInputNumber v-model:value="draft.top_p" :min="0" :max="1" :step="0.05" class="w-full" />
                  </NFormItem>
                  <NFormItem :label="$t('page.agentBuilder.maxOutputTokens')">
                    <NInputNumber v-model:value="draft.max_output_tokens" :min="1" :max="1000000" class="w-full" />
                  </NFormItem>
                </NCollapseItem>
                <NCollapseItem :title="$t('page.agentBuilder.diagnostics')" name="diagnostics">
                  <NAlert
                    v-if="diagnostics"
                    :type="diagnostics.healthy ? 'success' : 'warning'"
                    :title="
                      diagnostics.healthy
                        ? $t('page.agentBuilder.diagnosticsHealthy')
                        : $t('page.agentBuilder.diagnosticsUnhealthy')
                    "
                  />
                </NCollapseItem>
              </NCollapse>
            </NForm>
          </template>
          <template v-else>
            <NAlert type="warning" class="mb-12px">{{ $t('page.agentBuilder.mockBanner') }}</NAlert>
            <div v-for="item in preview.messages" :key="item.id" class="mb-8px rounded-8px bg-gray-500/10 px-10px py-8px">
              <div class="text-12px text-gray-500">{{ item.role }}</div>
              <div class="whitespace-pre-wrap break-words">{{ item.text }}</div>
            </div>
            <NInput v-model:value="previewInput" type="textarea" :rows="3" class="mb-8px" />
            <NButton type="primary" @click="runMockPreview">{{ $t('page.agentBuilder.runMock') }}</NButton>
          </template>
        </div>
      </div>
    </NSpin>

    <NDrawer v-model:show="versionsVisible" :width="420">
      <NDrawerContent :title="$t('page.agentBuilder.versions')" closable>
        <NEmpty v-if="versions.length === 0" :description="$t('page.agentBuilder.noVersions')" />
        <NList v-else bordered>
          <NListItem v-for="version in versions" :key="version.id">
            <NThing
              :title="`${$t('page.agentBuilder.versionLabel')} ${version.version}`"
              :description="`${$t('page.agentBuilder.modelAlias')}: ${version.model_alias}`"
            />
          </NListItem>
        </NList>
      </NDrawerContent>
    </NDrawer>
  </div>
</template>
