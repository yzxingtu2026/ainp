<script setup lang="tsx">
import { h, onMounted, reactive, ref } from 'vue';
import type { DataTableColumns } from 'naive-ui';
import { NButton, NPopconfirm, NSpace, NTag } from 'naive-ui';
import { AI_ACTIONS } from '@/constants/ai';
import { useRouterPush } from '@/hooks/common/router';
import { $t } from '@/locales';
import {
  duplicateAIAgent,
  fetchAIAgentDiagnostics,
  fetchAIAgents,
  fetchAIAgentVersions,
  reconcileBuiltinAIAgent,
  updateAIAgentStatus
} from '@/service/api';
import { useAuthStore } from '@/store/modules/auth';

defineOptions({ name: 'AIAgents' });
const auth = useAuthStore();
const { routerPushByKey } = useRouterPush();
const can = (permission: string) =>
  auth.userInfo.buttons.includes(permission) || auth.userInfo.buttons.includes('*');

const rows = ref<Api.AI.Agent[]>([]);
const total = ref(0);
const loading = ref(false);
const query = reactive({ page: 1, page_size: 20, keyword: '', source: '', status: '' });
const versions = ref<Api.AI.AgentVersion[]>([]);
const diagnostics = ref<Api.AI.AgentDiagnostics | null>(null);

const duplicateVisible = ref(false);
const duplicateSource = ref<Api.AI.Agent | null>(null);
const duplicateForm = reactive({ code: '', name: '' });

const detailVisible = ref(false);
const detail = ref<Api.AI.Agent | null>(null);

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

function openBuilder(agent?: Api.AI.Agent) {
  if (agent) {
    routerPushByKey('ai_agent-builder', { query: { agentId: agent.id } });
    return;
  }
  routerPushByKey('ai_agent-builder');
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
  duplicateForm.name = `${agent.name} copy`;
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

async function reconcile() {
  const { error } = await reconcileBuiltinAIAgent();
  if (!error) await load();
}

const columns: DataTableColumns<Api.AI.Agent> = [
  {
    title: () => $t('page.aiAgents.colName'),
    key: 'name',
    render: agent =>
      h(NSpace, { vertical: true, size: 2 }, {
        default: () => [
          h('span', agent.name),
          agent.contract_key
            ? h(NTag, { size: 'small', bordered: false }, { default: () => `${$t('page.aiAgents.managed')}: ${agent.contract_key}@${agent.contract_version}` })
            : null
        ]
      })
  },
  { title: () => $t('page.aiAgents.colCode'), key: 'code' },
  {
    title: () => $t('page.aiAgents.colSource'),
    key: 'source',
    render: agent => h(NTag, { size: 'small' }, { default: () => $t(`page.aiAgents.source.${agent.source}` as App.I18n.I18nKey) })
  },
  {
    title: () => $t('page.aiAgents.colVisibility'),
    key: 'visibility',
    render: agent => $t(`page.aiAgents.visibility.${agent.visibility}` as App.I18n.I18nKey)
  },
  {
    title: () => $t('page.aiAgents.colStatus'),
    key: 'status',
    render: agent =>
      h(
        NTag,
        { type: agent.status === 'enabled' ? 'success' : agent.status === 'draft' ? 'warning' : 'default' },
        { default: () => $t(`page.aiAgents.status.${agent.status}` as App.I18n.I18nKey) }
      )
  },
  { title: () => $t('page.aiAgents.colVersion'), key: 'current_version' },
  {
    title: () => $t('common.action'),
    key: 'actions',
    width: 280,
    render: agent =>
      h(NSpace, null, {
        default: () => [
          h(NButton, { size: 'small', onClick: () => inspect(agent) }, { default: () => $t('common.detail') }),
          can(AI_ACTIONS.agentUpdate)
            ? h(NButton, { size: 'small', onClick: () => openBuilder(agent) }, { default: () => $t('page.aiAgents.openBuilder') })
            : null,
          can(AI_ACTIONS.agentDuplicate)
            ? h(NButton, { size: 'small', onClick: () => openDuplicate(agent) }, { default: () => $t('page.aiAgents.duplicate') })
            : null,
          can(AI_ACTIONS.agentStatus)
            ? h(
                NPopconfirm,
                { onPositiveClick: () => toggle(agent) },
                {
                  trigger: () =>
                    h(NButton, { size: 'small' }, {
                      default: () => (agent.status === 'enabled' ? $t('page.aiAgents.disable') : $t('page.aiAgents.enable'))
                    }),
                  default: () => $t('page.aiAgents.statusConfirm')
                }
              )
            : null
        ]
      })
  }
];

onMounted(() => load());
</script>

<template>
  <div class="h-full min-h-500px flex-col-stretch gap-16px overflow-hidden lt-sm:overflow-auto">
    <NCard :title="$t('page.aiAgents.title')" :bordered="false" size="small" class="card-wrapper">
      <template #header-extra>
        <NSpace>
          <NButton v-if="can(AI_ACTIONS.agentBuiltinReconcile)" @click="reconcile">
            {{ $t('page.aiAgents.reconcile') }}
          </NButton>
          <NButton v-if="can(AI_ACTIONS.agentCreate)" type="primary" @click="openBuilder()">
            {{ $t('page.aiAgents.create') }}
          </NButton>
        </NSpace>
      </template>
      <NSpace class="mb-16px">
        <NInput v-model:value="query.keyword" clearable :placeholder="$t('page.aiAgents.keyword')" @keyup.enter="load" />
        <NSelect
          v-model:value="query.source"
          class="w-140px"
          :options="[
            { label: $t('page.aiAgents.sourceAll'), value: '' },
            { label: $t('page.aiAgents.source.builtin'), value: 'builtin' },
            { label: $t('page.aiAgents.source.tenant'), value: 'tenant' },
            { label: $t('page.aiAgents.source.personal'), value: 'personal' }
          ]"
          @update:value="load"
        />
        <NSelect
          v-model:value="query.status"
          class="w-140px"
          :options="[
            { label: $t('page.aiAgents.statusAll'), value: '' },
            { label: $t('page.aiAgents.status.draft'), value: 'draft' },
            { label: $t('page.aiAgents.status.enabled'), value: 'enabled' },
            { label: $t('page.aiAgents.status.disabled'), value: 'disabled' }
          ]"
          @update:value="load"
        />
        <NButton @click="load">{{ $t('common.search') }}</NButton>
      </NSpace>
      <NDataTable
        remote
        :loading="loading"
        :columns="columns"
        :data="rows"
        :scroll-x="1180"
        :pagination="{
          page: query.page,
          pageSize: query.page_size,
          itemCount: total,
          onChange: (page: number) => {
            query.page = page;
            load();
          }
        }"
      />
    </NCard>

    <NModal v-model:show="duplicateVisible" preset="card" :title="$t('page.aiAgents.duplicate')" class="w-520px max-w-94vw">
      <NForm label-placement="left" label-width="90">
        <NFormItem :label="$t('page.aiAgents.colName')"><NInput v-model:value="duplicateForm.name" /></NFormItem>
        <NFormItem :label="$t('page.aiAgents.colCode')"><NInput v-model:value="duplicateForm.code" /></NFormItem>
      </NForm>
      <template #footer>
        <NSpace justify="end">
          <NButton @click="duplicateVisible = false">{{ $t('common.cancel') }}</NButton>
          <NButton type="primary" :disabled="!duplicateForm.name || !duplicateForm.code" @click="duplicate">
            {{ $t('page.aiAgents.duplicate') }}
          </NButton>
        </NSpace>
      </template>
    </NModal>

    <NDrawer v-model:show="detailVisible" :width="520">
      <NDrawerContent :title="detail?.name" closable>
        <NAlert
          :type="diagnostics?.healthy ? 'success' : 'warning'"
          :title="diagnostics?.healthy ? $t('page.agentBuilder.diagnosticsHealthy') : $t('page.agentBuilder.diagnosticsUnhealthy')"
        >
          <div v-for="issue in diagnostics?.issues" :key="`${issue.code}-${issue.resource || ''}`">
            {{ issue.code }}：{{ issue.message }}
          </div>
        </NAlert>
        <NDivider>{{ $t('page.agentBuilder.versions') }}</NDivider>
        <NList bordered>
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
