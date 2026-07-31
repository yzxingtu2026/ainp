<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { useDialog, type DropdownOption } from 'naive-ui';
import AgentAvatar from '@/components/ai/agent-avatar.vue';
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
import { formatDateTime } from '@/utils/datetime';

defineOptions({ name: 'AIAgents' });
const auth = useAuthStore();
const dialog = useDialog();
const { routerPushByKey } = useRouterPush();
const can = (permission: string) => auth.userInfo.buttons.includes(permission) || auth.userInfo.buttons.includes('*');

const rows = ref<Api.AI.Agent[]>([]);
const total = ref(0);
const loading = ref(false);
const loadError = ref(false);
const query = reactive({ page: 1, page_size: 20, keyword: '', source: '', status: '' });
const versions = ref<Api.AI.AgentVersion[]>([]);
const diagnostics = ref<Api.AI.AgentDiagnostics | null>(null);
const operatingIds = ref(new Set<string>());

const duplicateVisible = ref(false);
const duplicateSource = ref<Api.AI.Agent | null>(null);
const duplicateForm = reactive({ code: '', name: '' });
const detailVisible = ref(false);
const detail = ref<Api.AI.Agent | null>(null);

const hasFilters = computed(() => Boolean(query.keyword || query.source || query.status));
const pageCount = computed(() => Math.max(1, Math.ceil(total.value / query.page_size)));

function setOperating(id: string, value: boolean) {
  const next = new Set(operatingIds.value);
  if (value) next.add(id);
  else next.delete(id);
  operatingIds.value = next;
}

async function load() {
  loading.value = true;
  loadError.value = false;
  try {
    const { data, error } = await fetchAIAgents(query);
    if (error || !data) {
      loadError.value = true;
      return;
    }
    rows.value = data.items || [];
    total.value = data.total || 0;
    if (!rows.value.length && query.page > 1 && total.value > 0) {
      query.page = Math.min(query.page - 1, pageCount.value);
      await load();
    }
  } finally {
    loading.value = false;
  }
}

function search() {
  query.page = 1;
  load();
}

function resetFilters() {
  query.keyword = '';
  query.source = '';
  query.status = '';
  search();
}

function changePage(page: number) {
  query.page = page;
  load();
}

function openBuilder(agent?: Api.AI.Agent) {
  routerPushByKey('ai_agent-builder', agent ? { query: { agentId: agent.id } } : undefined);
}

function primaryAction(agent: Api.AI.Agent) {
  if (can(AI_ACTIONS.agentUpdate)) openBuilder(agent);
  else inspect(agent);
}

async function toggle(agent: Api.AI.Agent) {
  setOperating(agent.id, true);
  try {
    const { error } = await updateAIAgentStatus(
      agent.id,
      agent.status === 'enabled' ? 'disabled' : 'enabled',
      agent.revision
    );
    if (!error) await load();
  } finally {
    setOperating(agent.id, false);
  }
}

function confirmToggle(agent: Api.AI.Agent) {
  dialog.warning({
    title: $t('page.aiAgents.statusConfirmTitle'),
    content: $t('page.aiAgents.statusConfirm'),
    positiveText: agent.status === 'enabled' ? $t('page.aiAgents.disable') : $t('page.aiAgents.enable'),
    negativeText: $t('common.cancel'),
    onPositiveClick: () => toggle(agent)
  });
}

async function inspect(agent: Api.AI.Agent) {
  detail.value = agent;
  detailVisible.value = true;
  diagnostics.value = null;
  versions.value = [];
  const [diagnosticResult, versionResult] = await Promise.all([
    fetchAIAgentDiagnostics(agent.id),
    fetchAIAgentVersions(agent.id)
  ]);
  diagnostics.value = diagnosticResult.data || null;
  versions.value = versionResult.data || [];
}

function openDetailBuilder() {
  if (!detail.value) return;
  detailVisible.value = false;
  openBuilder(detail.value);
}

function openDuplicate(agent: Api.AI.Agent) {
  duplicateSource.value = agent;
  duplicateForm.code = `${agent.code}-copy`.slice(0, 64);
  duplicateForm.name = `${agent.name} copy`;
  duplicateVisible.value = true;
}

async function duplicate() {
  if (!duplicateSource.value) return;
  const agentId = duplicateSource.value.id;
  setOperating(agentId, true);
  try {
    const { error } = await duplicateAIAgent({ id: agentId, ...duplicateForm });
    if (!error) {
      duplicateVisible.value = false;
      await load();
    }
  } finally {
    setOperating(agentId, false);
  }
}

async function reconcile() {
  const { error } = await reconcileBuiltinAIAgent();
  if (!error) await load();
}

function menuOptions(agent: Api.AI.Agent): DropdownOption[] {
  const options: DropdownOption[] = [{ label: $t('common.detail'), key: 'detail', icon: () => null }];
  if (can(AI_ACTIONS.agentDuplicate)) options.push({ label: $t('page.aiAgents.duplicate'), key: 'duplicate' });
  if (can(AI_ACTIONS.agentStatus) && agent.current_version > 0) {
    options.push({
      label: agent.status === 'enabled' ? $t('page.aiAgents.disable') : $t('page.aiAgents.enable'),
      key: 'toggle'
    });
  }
  return options;
}

function selectMenu(key: string, agent: Api.AI.Agent) {
  if (key === 'detail') inspect(agent);
  if (key === 'duplicate') openDuplicate(agent);
  if (key === 'toggle') confirmToggle(agent);
}

onMounted(load);
</script>

<template>
  <div class="agent-page">
    <header class="agent-page__header">
      <div>
        <h2>{{ $t('page.aiAgents.title') }}</h2>
        <span class="agent-page__count">{{ $t('page.aiAgents.resultCount', { total }) }}</span>
      </div>
      <NSpace wrap>
        <NButton v-if="can(AI_ACTIONS.agentBuiltinReconcile)" @click="reconcile">
          <template #icon><SvgIcon icon="mdi:sync" /></template>
          {{ $t('page.aiAgents.reconcile') }}
        </NButton>
        <NButton v-if="can(AI_ACTIONS.agentCreate)" type="primary" @click="openBuilder()">
          <template #icon><SvgIcon icon="mdi:plus" /></template>
          {{ $t('page.aiAgents.create') }}
        </NButton>
      </NSpace>
    </header>

    <div class="agent-filters">
      <NInput
        v-model:value="query.keyword"
        clearable
        class="agent-filters__keyword"
        :placeholder="$t('page.aiAgents.keyword')"
        @keyup.enter="search"
      />
      <AppSelect
        v-model:value="query.source"
        class="agent-filters__select"
        :options="[
          { label: $t('page.aiAgents.sourceAll'), value: '' },
          { label: $t('page.aiAgents.source.builtin'), value: 'builtin' },
          { label: $t('page.aiAgents.source.tenant'), value: 'tenant' },
          { label: $t('page.aiAgents.source.personal'), value: 'personal' }
        ]"
        @update:value="search"
      />
      <AppSelect
        v-model:value="query.status"
        class="agent-filters__select"
        :options="[
          { label: $t('page.aiAgents.statusAll'), value: '' },
          { label: $t('page.aiAgents.status.draft'), value: 'draft' },
          { label: $t('page.aiAgents.status.enabled'), value: 'enabled' },
          { label: $t('page.aiAgents.status.disabled'), value: 'disabled' }
        ]"
        @update:value="search"
      />
      <NButton @click="search">
        <template #icon><SvgIcon icon="mdi:magnify" /></template>
        {{ $t('common.search') }}
      </NButton>
      <NButton :disabled="!hasFilters" @click="resetFilters">{{ $t('common.reset') }}</NButton>
    </div>

    <div v-if="loading" class="agent-grid" aria-live="polite">
      <NCard v-for="item in 6" :key="item" size="small" class="agent-card agent-card--skeleton">
        <NSkeleton text :repeat="4" />
      </NCard>
    </div>

    <NResult
      v-else-if="loadError"
      status="error"
      :title="$t('page.aiAgents.loadFailed')"
      :description="$t('page.aiAgents.loadFailedHint')"
    >
      <template #footer><NButton type="primary" @click="load">{{ $t('common.refresh') }}</NButton></template>
    </NResult>

    <NEmpty
      v-else-if="!rows.length"
      class="agent-page__empty"
      :description="hasFilters ? $t('page.aiAgents.emptyFiltered') : $t('page.aiAgents.empty')"
    >
      <template #extra>
        <NButton v-if="hasFilters" @click="resetFilters">{{ $t('page.aiAgents.clearFilters') }}</NButton>
        <NButton v-else-if="can(AI_ACTIONS.agentCreate)" type="primary" @click="openBuilder()">
          {{ $t('page.aiAgents.create') }}
        </NButton>
      </template>
    </NEmpty>

    <div v-else class="agent-grid">
      <NCard
        v-for="agent in rows"
        :key="agent.id"
        size="small"
        class="agent-card"
        :class="{ 'agent-card--clickable': true }"
        role="button"
        tabindex="0"
        :aria-label="$t('page.aiAgents.openAgent', { name: agent.name })"
        @click="primaryAction(agent)"
        @keydown.enter="primaryAction(agent)"
        @keydown.space.prevent="primaryAction(agent)"
      >
        <div class="agent-card__top">
          <div class="agent-card__identity">
            <div class="agent-card__title-row">
              <h3 :title="agent.name">{{ agent.name }}</h3>
              <NTag
                size="small"
                :type="agent.status === 'enabled' ? 'success' : agent.status === 'draft' ? 'warning' : 'default'"
              >
                {{ $t(`page.aiAgents.status.${agent.status}`) }}
              </NTag>
            </div>
            <code :title="agent.code">{{ agent.code }}</code>
          </div>
          <AgentAvatar :attachment-id="agent.avatar_attachment_id" :name="agent.name" :size="60" />
        </div>

        <p class="agent-card__description" :title="agent.description || $t('page.aiAgents.noDescription')">
          {{ agent.description || $t('page.aiAgents.noDescription') }}
        </p>

        <div class="agent-card__tags">
          <NTag size="small" :bordered="false">{{ $t(`page.aiAgents.source.${agent.source}`) }}</NTag>
          <NTag size="small" :bordered="false">{{ $t(`page.aiAgents.visibility.${agent.visibility}`) }}</NTag>
          <NTag v-if="agent.has_draft" size="small" type="warning" :bordered="false">
            {{ $t('page.aiAgents.hasDraft') }}
          </NTag>
          <NTag v-if="agent.contract_key" size="small" type="info" :bordered="false">
            {{ $t('page.aiAgents.builtinManaged') }} · {{ agent.contract_key }}@{{ agent.contract_version }}
          </NTag>
        </div>

        <div class="agent-card__meta">
          <span>
            {{ $t('page.aiAgents.colVersion') }}:
            <strong>{{ agent.current_version > 0 ? `v${agent.current_version}` : $t('page.aiAgents.unpublished') }}</strong>
          </span>
          <span>{{ $t('page.aiAgents.updatedAt') }}: {{ formatDateTime(agent.updated_at) }}</span>
        </div>

        <div class="agent-card__actions" @click.stop @keydown.stop>
          <NButton
            type="primary"
            size="small"
            :loading="operatingIds.has(agent.id)"
            @click="primaryAction(agent)"
          >
            {{
              agent.current_version <= 0 && can(AI_ACTIONS.agentVersionPublish)
                ? $t('page.aiAgents.publishFirstVersion')
                : can(AI_ACTIONS.agentUpdate)
                  ? $t('page.aiAgents.openBuilder')
                  : $t('common.detail')
            }}
          </NButton>
          <NDropdown
            trigger="click"
            :options="menuOptions(agent)"
            :disabled="operatingIds.has(agent.id)"
            @select="key => selectMenu(String(key), agent)"
          >
            <NButton quaternary circle :aria-label="$t('page.aiAgents.moreActions', { name: agent.name })">
              <template #icon><SvgIcon icon="mdi:dots-horizontal" /></template>
            </NButton>
          </NDropdown>
        </div>
      </NCard>
    </div>

    <NPagination
      v-if="!loading && !loadError && total > query.page_size"
      class="agent-page__pagination"
      :page="query.page"
      :page-size="query.page_size"
      :item-count="total"
      @update:page="changePage"
    />

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
            {{ issue.code }}: {{ issue.message }}
          </div>
        </NAlert>
        <NButton
          v-if="detail && detail.current_version <= 0 && can(AI_ACTIONS.agentVersionPublish)"
          class="mt-12px"
          type="warning"
          @click="openDetailBuilder"
        >
          {{ $t('page.aiAgents.publishFirstVersion') }}
        </NButton>
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

<style scoped>
.agent-page {
  min-height: 500px;
  padding: 4px;
}

.agent-page__header,
.agent-page__header > div,
.agent-filters,
.agent-card__top,
.agent-card__title-row,
.agent-card__actions {
  display: flex;
  align-items: center;
}

.agent-page__header {
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 16px;
}

.agent-page__header > div { gap: 10px; }
.agent-page__header h2 { margin: 0; font-size: 20px; }
.agent-page__count { color: var(--n-text-color-3); }

.agent-filters {
  flex-wrap: wrap;
  gap: 12px;
  margin-bottom: 18px;
}

.agent-filters__keyword { width: min(320px, 100%); }
.agent-filters__select { width: 150px; }

.agent-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 16px;
}

.agent-card {
  height: 258px;
}

.agent-card :deep(.n-card__content) {
  display: flex;
  height: 100%;
  flex-direction: column;
  padding: 16px;
}

.agent-card--clickable { cursor: pointer; }
.agent-card--clickable:focus-visible { outline: 2px solid rgb(var(--primary-color)); outline-offset: 2px; }
.agent-card--skeleton { height: 258px; }

.agent-card__top { align-items: flex-start; justify-content: space-between; gap: 16px; }
.agent-card__identity { min-width: 0; }
.agent-card__title-row { align-items: flex-start; gap: 8px; }
.agent-card h3 { overflow: hidden; margin: 0; font-size: 16px; text-overflow: ellipsis; white-space: nowrap; }
.agent-card code { display: block; overflow: hidden; margin-top: 5px; color: var(--n-text-color-3); text-overflow: ellipsis; white-space: nowrap; }

.agent-card__description {
  display: -webkit-box;
  min-height: 40px;
  overflow: hidden;
  margin: 12px 0 10px;
  color: var(--n-text-color-2);
  line-height: 20px;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

.agent-card__tags {
  display: flex;
  height: 46px;
  overflow: hidden;
  flex-wrap: wrap;
  align-content: flex-start;
  gap: 5px;
}

.agent-card__meta {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  margin-top: 9px;
  overflow: hidden;
  color: var(--n-text-color-3);
  font-size: 12px;
  white-space: nowrap;
}

.agent-card__meta span:last-child {
  overflow: hidden;
  text-overflow: ellipsis;
}

.agent-card__actions { justify-content: space-between; margin-top: auto; padding-top: 10px; }
.agent-page__empty { padding: 80px 0; }
.agent-page__pagination { justify-content: flex-end; margin-top: 20px; }

@media (max-width: 1100px) {
  .agent-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
}

@media (max-width: 640px) {
  .agent-page__header { align-items: flex-start; flex-direction: column; }
  .agent-filters__keyword, .agent-filters__select { width: 100%; }
  .agent-grid { grid-template-columns: minmax(0, 1fr); }
  .agent-card, .agent-card--skeleton { height: 270px; }
}
</style>
