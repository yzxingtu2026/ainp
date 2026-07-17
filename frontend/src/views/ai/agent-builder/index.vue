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
  unbindAIAgentTool,
  updateAIAgent
} from '@/service/api';
import { useAuthStore } from '@/store/modules/auth';
import MarkdownPromptEditor from '@/components/common/markdown-prompt-editor.vue';
import AgentToolPickerModal from './modules/agent-tool-picker-modal.vue';
import PreviewComposer from './modules/preview-composer.vue';
import { useBuilderPreview } from './use-builder-preview';
import AgentRunStatus from '@/components/ai/agent-run-status.vue';
import ToolConfirmationCard from '@/components/ai/tool-confirmation-card.vue';

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
const dirty = ref(false);
const mobileTab = ref<'prompt' | 'config' | 'preview'>('prompt');
const versionsVisible = ref(false);
const toolPickerVisible = ref(false);
/** 中间栏折叠分组：默认只展开基本信息 */
const configExpandedNames = ref<Array<string | number>>(['basic']);
const collapsed = reactive({ left: false, right: false });
const leftWidth = ref(360);
const rightWidth = ref(360);

const agent = ref<Api.AI.AgentDetail | null>(null);
const versions = ref<Api.AI.AgentVersion[]>([]);
const diagnostics = ref<Api.AI.AgentDiagnostics | null>(null);
const boundTools = ref<Api.AI.AgentToolBinding[]>([]);
const aliases = ref<Api.AI.Alias[]>([]);
const deployments = ref<Api.AI.Deployment[]>([]);
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

const {
  status: previewStatus,
  error: previewError,
  input: previewInput,
  messages: previewMessages,
  confirmations: previewConfirmations,
  run: previewRun,
  busy: previewBusy,
  canSend: previewCanSend,
  runSummary: previewRunSummary,
  streamingText: previewStreamingText,
  streaming: previewStreaming,
  currentStepType: previewStepType,
  messageListEl: previewMessageListEl,
  messageText: previewMessageText,
  send: sendPreview,
  stop: stopPreview,
  clear: clearPreview,
  onConfirmationResolved,
  resetPreviewBinding
} = useBuilderPreview({
  agent,
  draft,
  diagnostics,
  dirty
});

const isCreate = computed(() => !agent.value);
const managedFields = computed(() => new Set(agent.value?.managed_fields || []));
const requiredTools = computed(() => new Set(agent.value?.required_tool_selectors || []));
const isManaged = (field: string) => managedFields.value.has(field);
const boundToolIDs = computed(() => boundTools.value.map(item => item.tool_id));
const previewActive = computed(
  () =>
    previewBusy.value ||
    previewStatus.value === 'materializing' ||
    previewStatus.value === 'running' ||
    previewStatus.value === 'waiting_confirmation'
);
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
    resetPreviewBinding();
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

async function bindTool(toolID: string) {
  if (!agent.value || !can(AI_ACTIONS.agentToolBind)) return;
  const { error } = await bindAIAgentTool(agent.value.id, toolID);
  if (error) return;
  const { data } = await fetchAIAgentTools(agent.value.id);
  boundTools.value = data || [];
  await refreshDiagnostics();
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

function currentReleaseVersion() {
  return versions.value.find(item => item.version === agent.value?.current_version);
}

function isMetaDirty() {
  if (!agent.value) return false;
  return draft.name !== agent.value.name || draft.description !== (agent.value.description || '');
}

function isVersionContentDirty() {
  const current = currentReleaseVersion();
  if (!current) return Boolean(draft.model_alias || draft.tenant_instructions);
  const gen = current.generation_config || {};
  return (
    draft.tenant_instructions !== (current.tenant_instructions || '') ||
    draft.model_alias !== (current.model_alias || '') ||
    Number(draft.temperature) !== Number(gen.temperature ?? 0.7) ||
    Number(draft.top_p) !== Number(gen.top_p ?? 1) ||
    Number(draft.max_output_tokens) !== Number(gen.max_output_tokens ?? 2048)
  );
}

/** 仅更新 Agent 元数据（名称/描述/头像），不碰版本内容 */
async function persistAgentMeta(): Promise<boolean> {
  if (!agent.value || !canEditMeta.value) return false;
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
    return false;
  }
  agent.value = { ...agent.value, ...data };
  draft.revision = data.revision;
  return true;
}

/** 发布 release 版本（提示词 / 模型 / 生成参数） */
async function persistVersion(): Promise<Api.AI.AgentVersion | null> {
  if (!agent.value) return null;
  const { data, error } = await publishAIAgentVersion({
    agent_id: agent.value.id,
    model_alias: draft.model_alias,
    tenant_instructions: draft.tenant_instructions,
    generation_config: generationConfig(),
    tool_policy: {},
    revision: draft.revision
  });
  if (error || !data) {
    message.error($t('page.agentBuilder.publishFailed'));
    return null;
  }
  // CreateVersion 会同步递增 Agent.revision，返回体是 Version 不含 revision。
  draft.revision += 1;
  return data;
}

async function saveMetadata() {
  if (!agent.value || !canEditMeta.value) return;
  if (instructionError.value) {
    message.error(instructionError.value);
    return;
  }
  const needMeta = isMetaDirty();
  const needVersion = isVersionContentDirty();
  if (!needMeta && !needVersion) {
    dirty.value = false;
    return;
  }
  if (needVersion && !can(AI_ACTIONS.agentVersionPublish)) {
    message.error($t('page.agentBuilder.promptSaveNeedPublish'));
    return;
  }
  savingMeta.value = true;
  try {
    if (needMeta) {
      const ok = await persistAgentMeta();
      if (!ok) return;
    }
    if (needVersion) {
      const version = await persistVersion();
      if (!version) return;
      dirty.value = false;
      message.success(
        needMeta
          ? $t('page.agentBuilder.metaAndPromptSaved')
          : $t('page.agentBuilder.promptSaved', { version: version.version })
      );
      resetPreviewBinding();
      await loadAgent(agent.value.id);
      return;
    }
    dirty.value = false;
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
    if (isMetaDirty() && canEditMeta.value) {
      const ok = await persistAgentMeta();
      if (!ok) return;
    }
    const data = await persistVersion();
    if (!data) return;
    dirty.value = false;
    message.success($t('page.agentBuilder.publishSuccess', { version: data.version }));
    resetPreviewBinding();
    await loadAgent(agent.value!.id);
  } finally {
    publishing.value = false;
  }
}

async function runPreview() {
  if (!agent.value) {
    message.warning($t('page.agentBuilder.previewNeedAgent'));
    return;
  }
  if (agent.value.status === 'disabled') {
    message.warning($t('page.agentBuilder.previewAgentDisabled'));
    return;
  }
  if (diagnostics.value && !diagnostics.value.healthy) {
    // 预览会物化当前编辑配置，不依赖已发布 current；仅「缺 current version」不拦截。
    const blocking = (diagnostics.value.issues || []).some(item => item.code !== 'version.missing');
    if (blocking) {
      message.warning($t('page.agentBuilder.previewBlocked'));
      return;
    }
  }
  if (!previewInput.value.trim()) {
    message.warning($t('page.agentBuilder.previewNeedInput'));
    return;
  }
  await sendPreview();
  if (previewError.value) message.error(previewError.value);
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
  () => ({ ...draft }),
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
      resetPreviewBinding();
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
          <div class="min-h-0 flex flex-1 flex-col gap-8px overflow-hidden px-12px py-10px">
            <NAlert v-if="instructionError" type="error" class="shrink-0">{{ instructionError }}</NAlert>
            <MarkdownPromptEditor
              v-model="draft.tenant_instructions"
              :maxlength="instructionLimit"
              :placeholder="$t('page.agentBuilder.tenantPlaceholder')"
            />
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
                    <NSpace class="mb-12px" justify="space-between">
                      <div class="text-12px text-gray-500">
                        {{ $t('page.agentBuilder.boundTools') }}（{{ boundTools.length }}）
                      </div>
                      <NButton
                        type="primary"
                        size="small"
                        :disabled="!can(AI_ACTIONS.agentToolBind)"
                        @click="toolPickerVisible = true"
                      >
                        {{ $t('page.agentBuilder.openToolPicker') }}
                      </NButton>
                    </NSpace>
                    <NEmpty v-if="boundTools.length === 0" :description="$t('page.agentBuilder.noBoundTools')" />
                    <NList v-else bordered class="max-h-280px overflow-auto">
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
            <NTag size="small" type="info" class="ml-8px">{{ $t('page.agentBuilder.previewLiveTag') }}</NTag>
          </div>
          <div class="min-h-0 flex flex-1 flex-col overflow-hidden px-12px py-10px">
            <NAlert type="info" class="mb-12px shrink-0">{{ $t('page.agentBuilder.previewLiveBanner') }}</NAlert>
            <div class="mb-8px shrink-0 text-12px text-gray-500">{{ previewRunSummary }}</div>
            <AgentRunStatus
              v-if="previewRun"
              class="mb-8px shrink-0"
              :run="previewRun"
              @cancel="stopPreview()"
              @resume="onConfirmationResolved()"
            />
            <div v-if="previewConfirmations.length" class="mb-12px flex shrink-0 flex-col gap-8px">
              <ToolConfirmationCard
                v-for="item in previewConfirmations"
                :key="item.id"
                :confirmation="item"
                @resolved="onConfirmationResolved()"
              />
            </div>
            <div ref="previewMessageListEl" class="min-h-0 flex-1 overflow-y-auto">
              <div
                v-for="item in previewMessages"
                :key="item.id"
                class="mb-8px rounded-8px px-10px py-8px"
                :class="item.role === 'user' ? 'bg-primary/10' : 'bg-gray-500/10'"
              >
                <div class="mb-4px text-12px text-gray-500">{{ item.role }}</div>
                <div class="whitespace-pre-wrap break-words">{{ previewMessageText(item) }}</div>
              </div>
              <div
                v-if="previewStreaming || previewStreamingText"
                class="mb-8px rounded-8px bg-gray-500/10 px-10px py-8px"
              >
                <div class="mb-4px flex items-center gap-6px text-12px text-gray-500">
                  <span>assistant</span>
                  <NTag v-if="previewStreaming" size="small" type="info" :bordered="false">
                    {{ previewStepType || 'streaming' }}
                  </NTag>
                </div>
                <div class="whitespace-pre-wrap break-words">
                  {{ previewStreamingText || '…' }}
                  <span v-if="previewStreaming" class="preview-caret">▍</span>
                </div>
              </div>
              <NEmpty
                v-if="previewMessages.length === 0 && !previewStreaming && !previewStreamingText"
                class="py-24px"
                :description="$t('page.agentBuilder.previewIdleSummary')"
              />
            </div>
            <div class="mt-8px shrink-0 border-t pt-10px">
              <PreviewComposer
                v-model="previewInput"
                :can-send="previewCanSend"
                :active="previewActive"
                @send="runPreview"
                @stop="stopPreview()"
                @clear="clearPreview()"
              />
            </div>
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
            <div class="h-360px flex flex-col">
              <MarkdownPromptEditor
                v-model="draft.tenant_instructions"
                :maxlength="instructionLimit"
                :placeholder="$t('page.agentBuilder.tenantPlaceholder')"
              />
            </div>
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
            <NAlert type="info" class="mb-12px">{{ $t('page.agentBuilder.previewLiveBanner') }}</NAlert>
            <div class="mb-8px text-12px text-gray-500">{{ previewRunSummary }}</div>
            <AgentRunStatus
              v-if="previewRun"
              class="mb-8px"
              :run="previewRun"
              @cancel="stopPreview()"
              @resume="onConfirmationResolved()"
            />
            <ToolConfirmationCard
              v-for="item in previewConfirmations"
              :key="item.id"
              class="mb-8px"
              :confirmation="item"
              @resolved="onConfirmationResolved()"
            />
            <div
              v-for="item in previewMessages"
              :key="item.id"
              class="mb-8px rounded-8px bg-gray-500/10 px-10px py-8px"
            >
              <div class="text-12px text-gray-500">{{ item.role }}</div>
              <div class="whitespace-pre-wrap break-words">{{ previewMessageText(item) }}</div>
            </div>
            <div
              v-if="previewStreaming || previewStreamingText"
              class="mb-8px rounded-8px bg-gray-500/10 px-10px py-8px"
            >
              <div class="mb-4px text-12px text-gray-500">assistant</div>
              <div class="whitespace-pre-wrap break-words">
                {{ previewStreamingText || '…' }}
                <span v-if="previewStreaming" class="preview-caret">▍</span>
              </div>
            </div>
            <PreviewComposer
              v-model="previewInput"
              class="mt-12px"
              :can-send="previewCanSend"
              :active="previewActive"
              @send="runPreview"
              @stop="stopPreview()"
              @clear="clearPreview()"
            />
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

    <AgentToolPickerModal
      v-model:show="toolPickerVisible"
      :bound-tool-ids="boundToolIDs"
      :can-bind="can(AI_ACTIONS.agentToolBind)"
      :bind-tool="bindTool"
    />
  </div>
</template>

<style scoped>
.preview-caret {
  display: inline-block;
  margin-left: 1px;
  animation: preview-caret-blink 1s steps(1) infinite;
  color: rgb(var(--primary-color));
}

@keyframes preview-caret-blink {
  0%,
  49% {
    opacity: 1;
  }
  50%,
  100% {
    opacity: 0;
  }
}
</style>
