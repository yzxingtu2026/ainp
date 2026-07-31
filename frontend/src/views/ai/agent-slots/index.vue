<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { AI_ACTIONS } from '@/constants/ai';
import {
  fetchAIAgents,
  fetchAIAgentSlots,
  reconcileAIAgentSlots,
  resetAIAgentSlot,
  updateAIAgentSlot
} from '@/service/api';
import { useAuthStore } from '@/store/modules/auth';

defineOptions({ name: 'AIAgentSlots' });
const auth = useAuthStore();
const can = (permission: string) => auth.userInfo.buttons.includes(permission) || auth.userInfo.buttons.includes('*');
const query = reactive<{
  page: number;
  page_size: number;
  keyword: string;
  module: string;
  status: string | null;
  source: string | null;
}>({ page: 1, page_size: 20, keyword: '', module: '', status: null, source: null });
const rows = ref<Api.AI.AgentSlotResolution[]>([]);
const total = ref(0);
const loading = ref(false);
const drawer = ref(false);
const selected = ref<Api.AI.AgentSlotResolution | null>(null);
const agentID = ref<string | null>(null);
const candidates = ref<Api.AI.Agent[]>([]);
const saving = ref(false);
const candidateOptions = computed(() => candidates.value.map(agent => ({ label: agent.name, value: agent.id })));

async function load() {
  loading.value = true;
  try {
    const { data } = await fetchAIAgentSlots(query);
    rows.value = data?.items || [];
    total.value = data?.total || 0;
  } finally { loading.value = false; }
}
async function search() {
  query.page = 1;
  await load();
}
async function configure(row: Api.AI.AgentSlotResolution) {
  selected.value = row;
  agentID.value = row.source === 'tenant' ? row.agent?.id || null : null;
  const { data } = await fetchAIAgents({ page: 1, page_size: 100, status: 'enabled', contract_key: row.slot.default_contract_key });
  candidates.value = (data?.items || []).filter(agent => agent.current_version > 0 && agent.visibility === 'tenant');
  drawer.value = true;
}
async function save() {
  if (!selected.value || !agentID.value) return;
  saving.value = true;
  const { error } = await updateAIAgentSlot(selected.value.slot.key, agentID.value, selected.value.binding_revision || 0);
  saving.value = false;
  if (!error) { drawer.value = false; await load(); }
}
async function reset() {
  if (!selected.value?.binding_revision) return;
  const { error } = await resetAIAgentSlot(selected.value.slot.key, selected.value.binding_revision);
  if (!error) { drawer.value = false; await load(); }
}
async function reconcile() { const { error } = await reconcileAIAgentSlots(); if (!error) await load(); }
onMounted(load);
</script>

<template>
  <div class="flex-col-stretch gap-16px">
    <div class="slot-toolbar">
      <NInput v-model:value="query.keyword" class="slot-toolbar__search" clearable placeholder="搜索业务功能" @keyup.enter="search" />
      <AppSelect v-model:value="query.status" class="slot-toolbar__status" clearable placeholder="筛选当前状态" :options="[{ label: '健康', value: 'healthy' }, { label: '异常', value: 'unhealthy' }]" />
      <AppSelect v-model:value="query.source" class="slot-toolbar__source" clearable placeholder="筛选配置来源" :options="[{ label: '租户指定', value: 'tenant' }, { label: '系统默认', value: 'builtin' }, { label: '用户偏好', value: 'user' }]" />
      <NButton @click="search">查询</NButton>
      <NButton v-if="can(AI_ACTIONS.agentSlotReconcile)" @click="reconcile">同步内置功能</NButton>
    </div>

    <NSpin :show="loading">
      <NEmpty v-if="!rows.length && !loading" description="暂无 AI 功能配置" />
      <div v-else class="grid grid-cols-1 gap-12px lg:grid-cols-2">
        <NCard v-for="row in rows" :key="row.slot.key" size="small">
          <NThing :title="row.slot.title" :description="row.slot.description">
            <template #header-extra><NTag :type="row.healthy ? 'success' : 'error'">{{ row.healthy ? '健康' : '不可用' }}</NTag></template>
            <div class="grid grid-cols-1 gap-8px text-14px sm:grid-cols-2">
              <div>来源模块：{{ row.slot.module }}</div><div>配置来源：{{ row.source === 'tenant' ? '租户指定' : row.source === 'user' ? '用户偏好' : '系统默认' }}</div>
              <div>当前智能体：{{ row.agent?.name || '不可用' }}</div><div>允许用户选择：{{ row.slot.user_override ? '是' : '否' }}</div>
            </div>
            <NAlert v-if="row.diagnostics.length" class="mt-12px" type="error" :title="row.diagnostics[0].message" />
            <template #footer><NButton v-if="can(AI_ACTIONS.agentSlotUpdate)" size="small" @click="configure(row)">配置</NButton></template>
          </NThing>
        </NCard>
      </div>
    </NSpin>
    <NPagination v-model:page="query.page" :page-size="query.page_size" :item-count="total" @update:page="load" />

    <NDrawer v-model:show="drawer" :width="520" placement="right" class="max-w-100vw">
      <NDrawerContent :title="selected?.slot.title" closable>
        <NDescriptions v-if="selected" :column="1" bordered label-placement="left">
          <NDescriptionsItem label="Slot Key">{{ selected.slot.key }}</NDescriptionsItem>
          <NDescriptionsItem label="Contract">{{ selected.slot.default_contract_key }}@{{ selected.slot.default_contract_version }}</NDescriptionsItem>
          <NDescriptionsItem label="能力要求">{{ selected.slot.required_capabilities.join(', ') }}</NDescriptionsItem>
        </NDescriptions>
        <NFormItem class="mt-20px" label="默认智能体">
          <AppSelect v-model:value="agentID" filterable :options="candidateOptions" placeholder="选择已启用且已发布的兼容智能体" />
        </NFormItem>
        <NAlert v-if="!candidates.length" type="warning" title="没有可配置的兼容智能体" />
        <template #footer>
          <NSpace justify="end">
            <NButton v-if="selected?.binding_revision && can(AI_ACTIONS.agentSlotReset)" @click="reset">恢复系统默认</NButton>
            <NButton @click="drawer = false">取消</NButton>
            <NButton type="primary" :loading="saving" :disabled="!agentID" @click="save">保存</NButton>
          </NSpace>
        </template>
      </NDrawerContent>
    </NDrawer>
  </div>
</template>

<style scoped>
.slot-toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 12px;
}

.slot-toolbar__search {
  flex: 0 1 360px;
  min-width: 240px;
}

.slot-toolbar__status {
  flex: 0 0 160px;
}

.slot-toolbar__source {
  flex: 0 0 180px;
}

@media (max-width: 640px) {
  .slot-toolbar__search {
    flex-basis: 100%;
  }

  .slot-toolbar__status,
  .slot-toolbar__source {
    flex: 1 1 160px;
  }
}
</style>
