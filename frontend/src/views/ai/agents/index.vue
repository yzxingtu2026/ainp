<script setup lang="tsx">
import { computed, h, onMounted, reactive, ref } from 'vue';
import type { DataTableColumns } from 'naive-ui';
import { NButton, NPopconfirm, NSpace, NTag } from 'naive-ui';
import { AI_ACTIONS } from '@/constants/ai';
import {
  bindAIAgentTool,
  createAIAgent,
  duplicateAIAgent,
  fetchAIAgentDiagnostics,
  fetchAIAgents,
  fetchAIAgentTools,
  fetchAIAgentVersions,
  fetchAIAliases,
  fetchAIDeployments,
  publishAIAgentVersion,
  reconcileBuiltinAIAgent,
  searchAIAgentTools,
  unbindAIAgentTool,
  updateAIAgent,
  updateAIAgentStatus
} from '@/service/api';
import { useAuthStore } from '@/store/modules/auth';

defineOptions({ name: 'AIAgents' });
const auth = useAuthStore();
const can = (permission: string) =>
  auth.userInfo.buttons.includes(permission) || auth.userInfo.buttons.includes('*');

const rows = ref<Api.AI.Agent[]>([]);
const total = ref(0);
const loading = ref(false);
const query = reactive({ page: 1, page_size: 20, keyword: '', source: '', status: '' });
const aliases = ref<Api.AI.Alias[]>([]);
const deployments = ref<Api.AI.Deployment[]>([]);
const versions = ref<Api.AI.AgentVersion[]>([]);
const diagnostics = ref<Api.AI.AgentDiagnostics | null>(null);

const editorVisible = ref(false);
const editing = ref<Api.AI.Agent | null>(null);
const saving = ref(false);
const editor = reactive({
  code: '',
  name: '',
  description: '',
  source: 'personal',
  visibility: 'private',
  model_alias: '',
  tenant_instructions: '',
  user_instructions: '',
  temperature: 0.7,
  top_p: 1,
  max_output_tokens: 0
});

const duplicateVisible = ref(false);
const duplicateSource = ref<Api.AI.Agent | null>(null);
const duplicateForm = reactive({ code: '', name: '' });

const toolsVisible = ref(false);
const toolsAgent = ref<Api.AI.Agent | null>(null);
const boundTools = ref<Api.AI.AgentToolBinding[]>([]);
const toolCandidates = ref<Api.AI.ToolCandidate[]>([]);
const toolKeyword = ref('');
const toolsLoading = ref(false);

const detailVisible = ref(false);
const detail = ref<Api.AI.Agent | null>(null);

const modelOptions = computed(() => {
  const enabled = new Map(deployments.value.filter(item => item.status === 'enabled').map(item => [item.id, item]));
  return aliases.value.flatMap(alias => {
    const deployment = enabled.get(alias.deployment_id);
    return deployment ? [{ label: `${alias.code} - ${deployment.display_name}`, value: alias.code }] : [];
  });
});
const boundToolIDs = computed(() => new Set(boundTools.value.map(item => item.tool_id)));

async function load() {
  loading.value = true;
  try {
    const { data } = await fetchAIAgents(query);
    rows.value = data?.items || [];
    total.value = data?.total || 0;
  } finally {
    loading.value = false;
  }
}
async function loadModels() {
  const [aliasResult, deploymentResult] = await Promise.all([
    fetchAIAliases(),
    fetchAIDeployments({ page: 1, page_size: 100, status: 'enabled' })
  ]);
  aliases.value = aliasResult.data || [];
  deployments.value = deploymentResult.data?.items || [];
}
function resetEditor() {
  Object.assign(editor, {
    code: '', name: '', description: '', source: 'personal', visibility: 'private', model_alias: '',
    tenant_instructions: '', user_instructions: '', temperature: 0.7, top_p: 1, max_output_tokens: 0
  });
}
async function openEditor(agent?: Api.AI.Agent) {
  editing.value = agent || null;
  resetEditor();
  if (agent) {
    const { data } = await fetchAIAgentVersions(agent.id);
    const current = data?.find(item => item.version === agent.current_version);
    Object.assign(editor, {
      code: agent.code,
      name: agent.name,
      description: agent.description,
      source: agent.source,
      visibility: agent.visibility,
      model_alias: current?.model_alias || '',
      tenant_instructions: current?.tenant_instructions || '',
      user_instructions: current?.user_instructions || '',
      temperature: Number(current?.generation_config?.temperature ?? 0.7),
      top_p: Number(current?.generation_config?.top_p ?? 1),
      max_output_tokens: Number(current?.generation_config?.max_output_tokens ?? 0)
    });
  }
  editorVisible.value = true;
}
async function saveEditor() {
  if (!editor.model_alias) return;
  saving.value = true;
  try {
    if (!editing.value) {
      const { error } = await createAIAgent({
        ...editor,
        generation_config: {
          temperature: editor.temperature,
          top_p: editor.top_p,
          max_output_tokens: editor.max_output_tokens
        },
        tool_policy: {}
      });
      if (error) return;
    } else {
      const agent = editing.value;
      const metadata = await updateAIAgent({
        id: agent.id,
        name: editor.name,
        description: editor.description,
        avatar_attachment_id: null,
        revision: agent.revision
      });
      if (metadata.error || !metadata.data) return;
      const published = await publishAIAgentVersion({
        agent_id: agent.id,
        model_alias: editor.model_alias,
        tenant_instructions: editor.tenant_instructions,
        user_instructions: editor.user_instructions,
        generation_config: {
          temperature: editor.temperature,
          top_p: editor.top_p,
          max_output_tokens: editor.max_output_tokens
        },
        tool_policy: {},
        revision: metadata.data.revision
      });
      if (published.error) return;
    }
    editorVisible.value = false;
    await load();
  } finally {
    saving.value = false;
  }
}
async function toggle(agent: Api.AI.Agent) {
  const { error } = await updateAIAgentStatus(agent.id, agent.status === 'enabled' ? 'disabled' : 'enabled', agent.revision);
  if (!error) await load();
}
async function inspect(agent: Api.AI.Agent) {
  detail.value = agent;
  detailVisible.value = true;
  const [diagnosticResult, versionResult] = await Promise.all([
    fetchAIAgentDiagnostics(agent.id),
    fetchAIAgentVersions(agent.id)
  ]);
  diagnostics.value = diagnosticResult.data || null;
  versions.value = versionResult.data || [];
}
function openDuplicate(agent: Api.AI.Agent) {
  duplicateSource.value = agent;
  duplicateForm.code = `${agent.code}-copy`.slice(0, 64);
  duplicateForm.name = `${agent.name} 副本`;
  duplicateVisible.value = true;
}
async function duplicate() {
  if (!duplicateSource.value) return;
  const { error } = await duplicateAIAgent({ id: duplicateSource.value.id, ...duplicateForm });
  if (!error) {
    duplicateVisible.value = false;
    await load();
  }
}
async function openTools(agent: Api.AI.Agent) {
  toolsAgent.value = agent;
  toolKeyword.value = '';
  toolCandidates.value = [];
  toolsVisible.value = true;
  const { data } = await fetchAIAgentTools(agent.id);
  boundTools.value = data || [];
  await searchTools();
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
  if (!toolsAgent.value) return;
  const { error } = await bindAIAgentTool(toolsAgent.value.id, toolID);
  if (!error) await openTools(toolsAgent.value);
}
async function unbindTool(toolID: string) {
  if (!toolsAgent.value) return;
  const { error } = await unbindAIAgentTool(toolsAgent.value.id, toolID);
  if (!error) await openTools(toolsAgent.value);
}
async function reconcile() {
  const { error } = await reconcileBuiltinAIAgent();
  if (!error) await load();
}

const sourceLabels: Record<Api.AI.AgentSource, string> = { builtin: '内置', tenant: '租户', personal: '个人' };
const statusLabels = { draft: '草稿', enabled: '已启用', disabled: '已停用' } as const;
const columns: DataTableColumns<Api.AI.Agent> = [
  { title: '名称', key: 'name', render: agent => h(NSpace, { vertical: true, size: 2 }, { default: () => [h('span', agent.name), agent.contract_key ? h(NTag, { size: 'small', bordered: false }, { default: () => `托管: ${agent.contract_key}@${agent.contract_version}` }) : null] }) },
  { title: '代码', key: 'code' },
  { title: '来源', key: 'source', render: agent => h(NTag, { size: 'small' }, { default: () => sourceLabels[agent.source] }) },
  { title: '可见性', key: 'visibility', render: agent => agent.visibility === 'tenant' ? '租户共享' : '仅自己' },
  { title: '状态', key: 'status', render: agent => h(NTag, { type: agent.status === 'enabled' ? 'success' : agent.status === 'draft' ? 'warning' : 'default' }, { default: () => statusLabels[agent.status] }) },
  { title: '版本', key: 'current_version' },
  { title: '操作', key: 'actions', width: 320, render: agent => h(NSpace, null, { default: () => [
    h(NButton, { size: 'small', onClick: () => inspect(agent) }, { default: () => '详情' }),
    can(AI_ACTIONS.agentUpdate) ? h(NButton, { size: 'small', onClick: () => openEditor(agent) }, { default: () => '编辑' }) : null,
    can(AI_ACTIONS.agentToolBind) ? h(NButton, { size: 'small', onClick: () => openTools(agent) }, { default: () => '工具' }) : null,
    can(AI_ACTIONS.agentDuplicate) ? h(NButton, { size: 'small', onClick: () => openDuplicate(agent) }, { default: () => '复制' }) : null,
    can(AI_ACTIONS.agentStatus) ? h(NPopconfirm, { onPositiveClick: () => toggle(agent) }, { trigger: () => h(NButton, { size: 'small' }, { default: () => agent.status === 'enabled' ? '停用' : '启用' }), default: () => '确认更新状态？' }) : null
  ] }) }
];

onMounted(() => Promise.all([load(), loadModels()]));
</script>

<template>
  <div class="h-full min-h-500px flex-col-stretch gap-16px overflow-hidden lt-sm:overflow-auto">
    <NCard title="智能体" :bordered="false" size="small" class="card-wrapper">
      <template #header-extra>
        <NSpace>
          <NButton v-if="can(AI_ACTIONS.agentBuiltinReconcile)" @click="reconcile">同步内置智能体</NButton>
          <NButton v-if="can(AI_ACTIONS.agentCreate)" type="primary" @click="openEditor()">新建智能体</NButton>
        </NSpace>
      </template>
      <NSpace class="mb-16px">
        <NInput v-model:value="query.keyword" clearable placeholder="名称或代码" @keyup.enter="load" />
        <NSelect v-model:value="query.source" class="w-140px" :options="[{label:'全部来源',value:''},{label:'内置',value:'builtin'},{label:'租户',value:'tenant'},{label:'个人',value:'personal'}]" @update:value="load" />
        <NSelect v-model:value="query.status" class="w-140px" :options="[{label:'全部状态',value:''},{label:'草稿',value:'draft'},{label:'已启用',value:'enabled'},{label:'已停用',value:'disabled'}]" @update:value="load" />
        <NButton @click="load">查询</NButton>
      </NSpace>
      <NDataTable remote :loading="loading" :columns="columns" :data="rows" :scroll-x="1180" :pagination="{page:query.page,pageSize:query.page_size,itemCount:total,onChange:(page:number)=>{query.page=page;load()}}" />
    </NCard>

    <NModal v-model:show="editorVisible" preset="card" :title="editing ? '编辑智能体' : '新建智能体'" class="w-720px max-w-94vw">
      <NForm label-placement="left" label-width="110">
        <NFormItem label="名称"><NInput v-model:value="editor.name" /></NFormItem>
        <NFormItem label="代码"><NInput v-model:value="editor.code" :disabled="Boolean(editing)" placeholder="my-agent" /></NFormItem>
        <NFormItem label="描述"><NInput v-model:value="editor.description" type="textarea" :maxlength="512" /></NFormItem>
        <NFormItem label="模型选择"><NSelect v-model:value="editor.model_alias" filterable clearable :options="modelOptions" placeholder="请选择可用的模型" /></NFormItem>
        <NFormItem label="类型"><NSelect v-model:value="editor.source" :disabled="Boolean(editing)" :options="[{label:'个人',value:'personal'},{label:'租户共享',value:'tenant'}]" @update:value="value=>editor.visibility=value==='tenant'?'tenant':'private'" /></NFormItem>
        <NFormItem label="租户指令"><NInput v-model:value="editor.tenant_instructions" type="textarea" :maxlength="12000" show-count /></NFormItem>
        <NFormItem label="个人指令"><NInput v-model:value="editor.user_instructions" type="textarea" :maxlength="12000" show-count /></NFormItem>
        <NDivider>生成参数</NDivider>
        <NGrid :cols="3" :x-gap="12">
          <NFormItemGi label="Temperature"><NInputNumber v-model:value="editor.temperature" :min="0" :max="2" :step="0.1" /></NFormItemGi>
          <NFormItemGi label="Top P"><NInputNumber v-model:value="editor.top_p" :min="0" :max="1" :step="0.1" /></NFormItemGi>
          <NFormItemGi label="最大输出"><NInputNumber v-model:value="editor.max_output_tokens" :min="0" /></NFormItemGi>
        </NGrid>
      </NForm>
      <template #footer><NSpace justify="end"><NButton @click="editorVisible=false">取消</NButton><NButton type="primary" :loading="saving" :disabled="!editor.name||!editor.code||!editor.model_alias" @click="saveEditor">{{ editing ? '保存并发布新版本' : '创建' }}</NButton></NSpace></template>
    </NModal>

    <NModal v-model:show="duplicateVisible" preset="card" title="复制智能体" class="w-520px max-w-94vw">
      <NForm label-placement="left" label-width="90"><NFormItem label="名称"><NInput v-model:value="duplicateForm.name" /></NFormItem><NFormItem label="代码"><NInput v-model:value="duplicateForm.code" /></NFormItem></NForm>
      <template #footer><NSpace justify="end"><NButton @click="duplicateVisible=false">取消</NButton><NButton type="primary" :disabled="!duplicateForm.name||!duplicateForm.code" @click="duplicate">复制</NButton></NSpace></template>
    </NModal>

    <NModal v-model:show="toolsVisible" preset="card" :title="`工具配置 - ${toolsAgent?.name || ''}`" class="w-760px max-w-94vw">
      <NInputGroup><NInput v-model:value="toolKeyword" clearable placeholder="按用途、名称或能力搜索" @keyup.enter="searchTools" /><NButton :loading="toolsLoading" @click="searchTools">搜索</NButton></NInputGroup>
      <NDivider>已绑定</NDivider>
      <NEmpty v-if="boundTools.length===0" description="尚未绑定工具" />
      <NList v-else bordered class="max-h-180px overflow-auto"><NListItem v-for="binding in boundTools" :key="binding.id"><NSpace justify="space-between"><span>{{ binding.tool_id }}</span><NButton v-if="can(AI_ACTIONS.agentToolUnbind)" size="small" type="error" ghost @click="unbindTool(binding.tool_id)">解绑</NButton></NSpace></NListItem></NList>
      <NDivider>可用工具</NDivider>
      <NSpin :show="toolsLoading"><NEmpty v-if="toolCandidates.length===0" description="没有匹配的工具" /><NList v-else bordered class="max-h-320px overflow-auto"><NListItem v-for="tool in toolCandidates" :key="tool.id"><NThing :title="tool.name" :description="tool.summary"><template #header-extra><NSpace><NTag size="small">{{ tool.source }}</NTag><NTag size="small" :type="tool.risk==='high'||tool.risk==='critical'?'error':tool.risk==='medium'?'warning':'success'">{{ tool.risk }}</NTag><NButton size="small" type="primary" :disabled="boundToolIDs.has(tool.id)" @click="bindTool(tool.id)">{{ boundToolIDs.has(tool.id) ? '已绑定' : '绑定' }}</NButton></NSpace></template></NThing></NListItem></NList></NSpin>
    </NModal>

    <NDrawer v-model:show="detailVisible" :width="520"><NDrawerContent :title="detail?.name" closable><NAlert :type="diagnostics?.healthy?'success':'warning'" :title="diagnostics?.healthy?'运行条件正常':'存在运行问题'"><div v-for="issue in diagnostics?.issues" :key="`${issue.code}-${issue.resource||''}`">{{ issue.code }}：{{ issue.message }}</div></NAlert><NDivider>版本历史</NDivider><NList bordered><NListItem v-for="version in versions" :key="version.id"><NThing :title="`版本 ${version.version}`" :description="`模型选择：${version.model_alias}`" /></NListItem></NList></NDrawerContent></NDrawer>
  </div>
</template>
