<script setup lang="tsx">
import { computed, h, onMounted, reactive, ref, watch } from 'vue';
import type { DataTableColumns } from 'naive-ui';
import { NButton, NPopconfirm, NSpace, NTag } from 'naive-ui';
import { $t } from '@/locales';
import {
  AI_ACTIONS,
  MODEL_FEATURES,
  MODEL_MODALITIES,
  MODEL_TYPES,
  STRUCTURED_OUTPUT_STRATEGIES
} from '@/constants/ai';
import {
  createAIDeployment,
  deleteAIAlias,
  deleteAIDeployment,
  discoverAIModels,
  fetchAIAliases,
  fetchAIDeployments,
  fetchAIProviders,
  importAIDeployments,
  updateAIDeployment,
  updateAIDeploymentStatus,
  upsertAIAlias
} from '@/service/api';
import { useAuthStore } from '@/store/modules/auth';
import { resolveModelContextBudget, type ContextBudgetFieldError } from './model-context-budget';
defineOptions({ name: 'AIModels' });
const auth = useAuthStore();
const rows = ref<Api.AI.Deployment[]>([]);
const aliases = ref<Api.AI.Alias[]>([]);
const providers = ref<Api.AI.Provider[]>([]);
const loading = ref(false);
const query = reactive({ page: 1, page_size: 20, keyword: '', status: '' });
const total = ref(0);
const visible = ref(false);
const editing = ref<Api.AI.Deployment | null>(null);
const aliasVisible = ref(false);
const aliasDeployment = ref<Api.AI.Deployment | null>(null);
const aliasCode = ref('');
const form = reactive({
  provider_id: '' as string | null,
  code: '',
  remote_model_id: '',
  display_name: '',
  model_type: 'chat',
  input_modalities: ['text'] as string[],
  output_modalities: ['text'] as string[],
  features: [] as string[],
  structured_output_strategies: [] as string[],
  context_window: null as number | null,
  max_output_tokens: null as number | null
});
const can = (p: string) => auth.userInfo.buttons.includes(p) || auth.userInfo.buttons.includes('*');
const discoveryVisible = ref(false);
const discoveryLoading = ref(false);
const discoveryProvider = ref('');
const remoteModels = ref<Api.AI.RemoteModel[]>([]);
const selectedRemote = ref<Array<string | number>>([]);
const remoteKeyword = ref('');
const importType = ref('chat');
const importInputModalities = ref<string[]>(['text']);
const importOutputModalities = ref<string[]>(['text']);
const importFeatures = ref<string[]>([]);
const importStructuredOutputStrategies = ref<string[]>([]);
const importContextWindow = ref<number | null>(null);
const importMaxOutputTokens = ref<number | null>(null);
const formContextBudget = computed(() =>
  resolveModelContextBudget(form.model_type, form.context_window, form.max_output_tokens)
);
const importContextBudget = computed(() =>
  resolveModelContextBudget(importType.value, importContextWindow.value, importMaxOutputTokens.value)
);
function contextBudgetError(error?: ContextBudgetFieldError) {
  return error ? $t(`aiModel.contextBudgetErrors.${error}`) : undefined;
}
watch(
  () => form.features.includes('structured_output'),
  enabled => {
    if (!enabled) form.structured_output_strategies = [];
  }
);
watch(
  () => importFeatures.value.includes('structured_output'),
  enabled => {
    if (!enabled) importStructuredOutputStrategies.value = [];
  }
);
const importCandidates = computed(() => {
  const keyword = remoteKeyword.value.trim().toLowerCase();
  return remoteModels.value.filter(
    x =>
      !x.imported &&
      (!keyword || x.id.toLowerCase().includes(keyword) || x.display_name.toLowerCase().includes(keyword))
  );
});
function featureLabel(feature: string) {
  const known = MODEL_FEATURES.find(item => item === feature);
  return known ? $t(`aiModel.features.${known}`) : feature;
}
function strategyLabel(strategy: string) {
  const known = STRUCTURED_OUTPUT_STRATEGIES.find(item => item === strategy);
  return known ? $t(`aiModel.structuredOutputStrategies.${known}`) : strategy;
}
function modalityLabel(modality: string) {
  const known = MODEL_MODALITIES.find(item => item === modality);
  return known ? $t(`aiModel.modalities.${known}`) : modality;
}
function invalidVisionConfig(input: string[], features: string[]) {
  return features.includes('vision') && !input.includes('image');
}
function invalidStructuredOutputConfig(features: string[], strategies: string[]) {
  return features.includes('structured_output') !== strategies.length > 0;
}
async function load() {
  loading.value = true;
  try {
    const [d, a, p] = await Promise.all([
      fetchAIDeployments(query),
      fetchAIAliases(),
      fetchAIProviders({ page: 1, page_size: 100 })
    ]);
    if (d.data) {
      rows.value = d.data.items;
      total.value = d.data.total;
    }
    aliases.value = a.data || [];
    providers.value = p.data?.items || [];
  } finally {
    loading.value = false;
  }
}
function openEditor(row?: Api.AI.Deployment) {
  editing.value = row || null;
  Object.assign(
    form,
    row
      ? {
          provider_id: row.provider_id,
          code: row.code,
          remote_model_id: row.remote_model_id,
          display_name: row.display_name,
          model_type: row.model_type,
          input_modalities: [...(row.capabilities_json.input_modalities || [])],
          output_modalities: [...(row.capabilities_json.output_modalities || [])],
          features: [...(row.capabilities_json.features || [])],
          structured_output_strategies: [...(row.capabilities_json.structured_output_strategies || [])],
          context_window: row.limits_json.context_window || null,
          max_output_tokens: row.limits_json.max_output_tokens || null
        }
      : {
          provider_id: '',
          code: '',
          remote_model_id: '',
          display_name: '',
          model_type: 'chat',
          input_modalities: ['text'],
          output_modalities: ['text'],
          features: [],
          structured_output_strategies: [],
          context_window: null,
          max_output_tokens: null
        }
  );
  visible.value = true;
}
async function save() {
  if (!formContextBudget.value.valid) {
    window.$message?.warning($t('aiModel.contextBudgetInvalid'));
    return;
  }
  if (invalidVisionConfig(form.input_modalities, form.features)) {
    window.$message?.warning($t('aiModel.visionNeedsImage'));
    return;
  }
  if (invalidStructuredOutputConfig(form.features, form.structured_output_strategies)) {
    window.$message?.warning('结构化输出能力与策略必须同时配置');
    return;
  }
  const common = {
    display_name: form.display_name,
    model_type: form.model_type,
    capabilities: {
      input_modalities: form.input_modalities,
      output_modalities: form.output_modalities,
      features: form.features,
      structured_output_strategies: form.structured_output_strategies
    },
    limits: { context_window: form.context_window || 0, max_output_tokens: form.max_output_tokens || 0 },
    config: {}
  };
  const result = editing.value
    ? await updateAIDeployment({ id: editing.value.id, ...common })
    : await createAIDeployment({
        provider_id: form.provider_id ?? '',
        code: form.code,
        remote_model_id: form.remote_model_id,
        ...common,
        status: 'enabled'
      });
  if (!result.error) {
    visible.value = false;
    await load();
  }
}
async function toggle(r: Api.AI.Deployment) {
  await updateAIDeploymentStatus(r.id, r.status === 'enabled' ? 'disabled' : 'enabled');
  await load();
}
async function removeDeployment(r: Api.AI.Deployment) {
  const { error } = await deleteAIDeployment(r.id);
  if (!error) await load();
}
async function openDiscovery() {
  if (!discoveryProvider.value) {
    discoveryProvider.value = providers.value.find(x => x.status === 'enabled')?.id || '';
  }
  if (!discoveryProvider.value) {
    window.$message?.warning('请先选择可用的模型供应商');
    return;
  }
  remoteModels.value = [];
  selectedRemote.value = [];
  remoteKeyword.value = '';
  discoveryVisible.value = true;
  discoveryLoading.value = true;
  try {
    const { data, error } = await discoverAIModels(discoveryProvider.value);
    if (!error && data) remoteModels.value = data.items;
  } finally {
    discoveryLoading.value = false;
  }
}
function deploymentCode(id: string) {
  const normalized = id
    .toLowerCase()
    .replace(/[^a-z0-9_-]+/g, '-')
    .replace(/^-+|-+$/g, '');
  return (normalized || 'model').slice(0, 64);
}
async function importSelected() {
  if (!importContextBudget.value.valid) {
    window.$message?.warning($t('aiModel.contextBudgetInvalid'));
    return;
  }
  if (invalidVisionConfig(importInputModalities.value, importFeatures.value)) {
    window.$message?.warning($t('aiModel.visionNeedsImage'));
    return;
  }
  if (invalidStructuredOutputConfig(importFeatures.value, importStructuredOutputStrategies.value)) {
    window.$message?.warning('结构化输出能力与策略必须同时配置');
    return;
  }
  const selected = new Set(selectedRemote.value.map(String));
  const items = remoteModels.value
    .filter(x => selected.has(x.id))
    .map(x => ({
      provider_id: discoveryProvider.value,
      code: deploymentCode(x.id),
      remote_model_id: x.id,
      display_name: x.display_name || x.id,
      model_type: importType.value,
      capabilities: {
        input_modalities: importInputModalities.value,
        output_modalities: importOutputModalities.value,
        features: importFeatures.value,
        structured_output_strategies: importStructuredOutputStrategies.value
      },
      limits: { context_window: importContextWindow.value || 0, max_output_tokens: importMaxOutputTokens.value || 0 },
      status: 'enabled',
      config: {}
    }));
  if (!items.length) return;
  const { error } = await importAIDeployments(items);
  if (!error) {
    discoveryVisible.value = false;
    await load();
  }
}
function openAlias(r: Api.AI.Deployment) {
  aliasDeployment.value = r;
  aliasCode.value = '';
  aliasVisible.value = true;
}
async function bindAlias() {
  const code = aliasCode.value.trim();
  if (!aliasDeployment.value || !code) return;
  const { error } = await upsertAIAlias(code, aliasDeployment.value.id);
  if (!error) {
    aliasVisible.value = false;
    await load();
  }
}
async function removeAlias(code: string) {
  const { error } = await deleteAIAlias(code);
  if (!error) await load();
}
const columns = computed<DataTableColumns<Api.AI.Deployment>>(() => [
  { title: '代码', key: 'code', minWidth: 120 },
  { title: '远端模型', key: 'remote_model_id', minWidth: 180 },
  { title: '类型', key: 'model_type', width: 110 },
  {
    title: '能力',
    key: 'capabilities_json',
    minWidth: 220,
    render: r =>
      h(
        NSpace,
        { size: 4 },
        {
          default: () =>
            r.capabilities_json.features.map(x => h(NTag, { size: 'small' }, { default: () => featureLabel(x) }))
        }
      )
  },
  {
    title: $t('aiModel.alias'),
    key: 'aliases',
    minWidth: 180,
    render: r =>
      h(
        NSpace,
        { size: 4 },
        {
          default: () =>
            aliases.value
              .filter(a => a.deployment_id === r.id)
              .map(a =>
                h(
                  NTag,
                  { closable: can(AI_ACTIONS.aliasDelete), onClose: () => removeAlias(a.code) },
                  { default: () => a.code }
                )
              )
        }
      )
  },
  {
    title: '状态',
    key: 'status',
    width: 100,
    render: r =>
      h(NTag, { type: r.status === 'enabled' ? 'success' : 'default' }, { default: () => $t(`aiModel.${r.status}`) })
  },
  {
    title: '操作',
    key: 'actions',
    width: 310,
    render: r =>
      h(NSpace, null, {
        default: () =>
          [
            can(AI_ACTIONS.deploymentUpdate)
              ? h(NButton, { size: 'small', onClick: () => openEditor(r) }, { default: () => $t('common.edit') })
              : null,
            can(AI_ACTIONS.aliasUpsert)
              ? h(NButton, { size: 'small', onClick: () => openAlias(r) }, { default: () => $t('aiModel.bindAlias') })
              : null,
            can(AI_ACTIONS.deploymentStatus)
              ? h(
                  NButton,
                  { size: 'small', onClick: () => toggle(r) },
                  { default: () => $t(`aiModel.${r.status === 'enabled' ? 'disabled' : 'enabled'}`) }
                )
              : null,
            can(AI_ACTIONS.deploymentDelete)
              ? h(
                  NPopconfirm,
                  { onPositiveClick: () => removeDeployment(r) },
                  {
                    trigger: () => h(NButton, { size: 'small', type: 'error' }, { default: () => $t('common.delete') }),
                    default: () => $t('aiModel.deleteConfirm')
                  }
                )
              : null
          ].filter(Boolean)
      })
  }
]);
onMounted(load);
</script>

<template>
  <div class="flex-col-stretch gap-16px">
    <NCard title="模型目录" :bordered="false">
      <NSpace>
        <NInput v-model:value="query.keyword" placeholder="搜索模型" @keyup.enter="load" />
        <NButton @click="load">查询</NButton>
        <AppSelect
          v-model:value="discoveryProvider"
          class="w-180px"
          placeholder="选择供应商"
          :options="providers.filter(x => x.status === 'enabled').map(x => ({ label: x.name, value: x.id }))"
        />
        <NButton v-if="can(AI_ACTIONS.deploymentImport) && can(AI_ACTIONS.providerDiscover)" @click="openDiscovery">
          远端发现
        </NButton>
        <NButton v-if="can(AI_ACTIONS.deploymentCreate)" type="primary" @click="openEditor()">手工新增</NButton>
      </NSpace>
    </NCard>
    <NCard :bordered="false">
      <NDataTable :columns="columns" :data="rows" :loading="loading" :scroll-x="1200" />
      <NPagination
        v-model:page="query.page"
        class="mt-16px justify-end"
        :item-count="total"
        :page-size="query.page_size"
        @update:page="load"
      />
    </NCard>
    <NModal
      v-model:show="visible"
      preset="card"
      :title="editing ? $t('aiModel.editTitle') : $t('aiModel.createTitle')"
      class="w-660px max-w-90vw"
    >
      <NAlert v-if="!editing" type="warning" class="mb-12px">手工模型必须明确确认类型、能力和限制。</NAlert>
      <NForm :model="form" label-placement="left" label-width="110">
        <NFormItem label="供应商">
          <AppSelect
            v-model:value="form.provider_id"
            :disabled="Boolean(editing)"
            :options="providers.filter(x => x.status === 'enabled').map(x => ({ label: x.name, value: x.id }))"
            placeholder="请选择供应商"
          />
        </NFormItem>
        <NFormItem label="代码"><NInput v-model:value="form.code" :disabled="Boolean(editing)" /></NFormItem>
        <NFormItem label="远端模型 ID">
          <NInput v-model:value="form.remote_model_id" :disabled="Boolean(editing)" />
        </NFormItem>
        <NFormItem label="显示名称"><NInput v-model:value="form.display_name" /></NFormItem>
        <NFormItem label="模型类型">
          <AppSelect v-model:value="form.model_type" :options="MODEL_TYPES.map(x => ({ label: x, value: x }))" />
        </NFormItem>
        <NFormItem :label="$t('aiModel.inputModalities')">
          <NCheckboxGroup v-model:value="form.input_modalities">
            <NSpace><NCheckbox v-for="x in MODEL_MODALITIES" :key="x" :value="x" :label="modalityLabel(x)" /></NSpace>
          </NCheckboxGroup>
        </NFormItem>
        <NFormItem :label="$t('aiModel.outputModalities')">
          <NCheckboxGroup v-model:value="form.output_modalities">
            <NSpace><NCheckbox v-for="x in MODEL_MODALITIES" :key="x" :value="x" :label="modalityLabel(x)" /></NSpace>
          </NCheckboxGroup>
        </NFormItem>
        <NFormItem label="能力">
          <NCheckboxGroup v-model:value="form.features">
            <NSpace><NCheckbox v-for="x in MODEL_FEATURES" :key="x" :value="x" :label="featureLabel(x)" /></NSpace>
          </NCheckboxGroup>
        </NFormItem>
        <NFormItem v-if="form.features.includes('structured_output')" label="输出策略">
          <AppSelect
            v-model:value="form.structured_output_strategies"
            multiple
            :options="STRUCTURED_OUTPUT_STRATEGIES.map(x => ({ label: strategyLabel(x), value: x }))"
          />
        </NFormItem>
        <NAlert
          v-if="invalidVisionConfig(form.input_modalities, form.features)"
          type="warning"
          class="mb-12px"
          :title="$t('aiModel.visionNeedsImage')"
        />
        <NAlert v-if="form.model_type === 'chat'" type="info" class="mb-12px" :bordered="false">
          {{ $t('aiModel.tokenBudgetHelp') }}
        </NAlert>
        <NFormItem
          :label="$t('aiModel.contextWindow')"
          :required="form.model_type === 'chat'"
          :validation-status="formContextBudget.contextWindowError ? 'error' : undefined"
          :feedback="contextBudgetError(formContextBudget.contextWindowError)"
        >
          <NInputNumber v-model:value="form.context_window" :min="1" :precision="0" />
        </NFormItem>
        <NFormItem
          :label="$t('aiModel.maxOutputTokens')"
          :required="form.model_type === 'chat'"
          :validation-status="formContextBudget.maxOutputTokensError ? 'error' : undefined"
          :feedback="contextBudgetError(formContextBudget.maxOutputTokensError)"
        >
          <NInputNumber v-model:value="form.max_output_tokens" :min="1" :precision="0" />
        </NFormItem>
        <NFormItem v-if="form.model_type === 'chat'" :label="$t('aiModel.availableInputTokens')">
          <NText>{{ formContextBudget.availableInputTokens?.toLocaleString() ?? '-' }}</NText>
        </NFormItem>
      </NForm>
      <template #footer>
        <NSpace justify="end">
          <NButton @click="visible = false">取消</NButton>
          <NButton type="primary" :disabled="!formContextBudget.valid" @click="save">保存</NButton>
        </NSpace>
      </template>
    </NModal>
    <NModal v-model:show="discoveryVisible" preset="card" title="发现并导入模型" class="w-820px max-w-90vw">
      <NAlert type="warning" class="mb-12px">供应商未提供可信能力元数据。请确认本批模型的类型和能力后再导入。</NAlert>
      <NSpace class="mb-12px">
        <NInput v-model:value="remoteKeyword" :disabled="discoveryLoading" placeholder="搜索模型 ID" />
        <AppSelect
          v-model:value="importType"
          class="w-150px"
          :disabled="discoveryLoading"
          :options="MODEL_TYPES.map(x => ({ label: x, value: x }))"
        />
      </NSpace>
      <div class="mb-8px">{{ $t('aiModel.inputModalities') }}</div>
      <NCheckboxGroup v-model:value="importInputModalities" class="mb-12px" :disabled="discoveryLoading">
        <NSpace><NCheckbox v-for="x in MODEL_MODALITIES" :key="x" :value="x" :label="modalityLabel(x)" /></NSpace>
      </NCheckboxGroup>
      <div class="mb-8px">{{ $t('aiModel.outputModalities') }}</div>
      <NCheckboxGroup v-model:value="importOutputModalities" class="mb-12px" :disabled="discoveryLoading">
        <NSpace><NCheckbox v-for="x in MODEL_MODALITIES" :key="x" :value="x" :label="modalityLabel(x)" /></NSpace>
      </NCheckboxGroup>
      <div class="mb-8px">{{ $t('aiModel.capabilities') }}</div>
      <NCheckboxGroup v-model:value="importFeatures" class="mb-12px" :disabled="discoveryLoading">
        <NSpace><NCheckbox v-for="x in MODEL_FEATURES" :key="x" :value="x" :label="featureLabel(x)" /></NSpace>
      </NCheckboxGroup>
      <div v-if="importFeatures.includes('structured_output')" class="mb-8px">结构化输出策略</div>
      <AppSelect
        v-if="importFeatures.includes('structured_output')"
        v-model:value="importStructuredOutputStrategies"
        multiple
        class="mb-12px"
        :disabled="discoveryLoading"
        :options="STRUCTURED_OUTPUT_STRATEGIES.map(x => ({ label: strategyLabel(x), value: x }))"
      />
      <NAlert
        v-if="invalidVisionConfig(importInputModalities, importFeatures)"
        type="warning"
        class="mb-12px"
        :title="$t('aiModel.visionNeedsImage')"
      />
      <NAlert v-if="importType === 'chat'" type="info" class="mb-12px" :bordered="false">
        {{ $t('aiModel.discoveryLimitsRequired') }}
      </NAlert>
      <NSpace v-if="importType === 'chat'" class="mb-12px" align="start">
        <NFormItem
          :label="$t('aiModel.contextWindow')"
          required
          :validation-status="importContextBudget.contextWindowError ? 'error' : undefined"
          :feedback="contextBudgetError(importContextBudget.contextWindowError)"
        >
          <NInputNumber v-model:value="importContextWindow" :min="1" :precision="0" />
        </NFormItem>
        <NFormItem
          :label="$t('aiModel.maxOutputTokens')"
          required
          :validation-status="importContextBudget.maxOutputTokensError ? 'error' : undefined"
          :feedback="contextBudgetError(importContextBudget.maxOutputTokensError)"
        >
          <NInputNumber v-model:value="importMaxOutputTokens" :min="1" :precision="0" />
        </NFormItem>
        <NFormItem :label="$t('aiModel.availableInputTokens')">
          <NText>{{ importContextBudget.availableInputTokens?.toLocaleString() ?? '-' }}</NText>
        </NFormItem>
      </NSpace>
      <NDataTable
        v-model:checked-row-keys="selectedRemote"
        :loading="discoveryLoading"
        max-height="min(400px, 46vh)"
        virtual-scroll
        :row-key="(r:Api.AI.RemoteModel)=>r.id"
        :columns="[{ type: 'selection' }, { title: '模型 ID', key: 'id' }, { title: '名称', key: 'display_name' }]"
        :data="importCandidates"
      />
      <template #footer>
        <NSpace justify="end">
          <NButton @click="discoveryVisible = false">取消</NButton>
          <NButton
            type="primary"
            :disabled="discoveryLoading || selectedRemote.length === 0 || !importContextBudget.valid"
            @click="importSelected"
          >
            导入所选
          </NButton>
        </NSpace>
      </template>
    </NModal>
    <NModal v-model:show="aliasVisible" preset="card" :title="$t('aiModel.bindAlias')" class="w-460px max-w-90vw">
      <NInput v-model:value="aliasCode" maxlength="64" placeholder="general-chat" @keyup.enter="bindAlias" />
      <template #footer>
        <NButton type="primary" :disabled="!aliasCode.trim()" @click="bindAlias">绑定</NButton>
      </template>
    </NModal>
  </div>
</template>
