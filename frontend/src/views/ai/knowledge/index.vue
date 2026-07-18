<script setup lang="tsx">
import { computed, h, onMounted, reactive, ref, watch } from 'vue';
import type { DataTableColumns, FormInst, FormRules } from 'naive-ui';
import { NButton, NSpace } from 'naive-ui';
import CodeTag from '@/components/common/code-tag.vue';
import DateTimeText from '@/components/common/datetime-text.vue';
import { AI_ACTIONS } from '@/constants/ai';
import { CODESET_KEYS } from '@/constants/codeset';
import { pageAfterDelete } from '@/hooks/common/table';
import { $t } from '@/locales';
import {
  createKnowledgeBase,
  createKnowledgeDocument,
  deleteKnowledgeBase,
  deleteKnowledgeDocument,
  fetchAIAliases,
  fetchAIDeployments,
  fetchKnowledgeBase,
  fetchKnowledgeBases,
  fetchKnowledgeDocuments,
  resolveKnowledgeCitation,
  retryKnowledgeDocument,
  testKnowledgeRetrieval,
  updateKnowledgeBase,
  updateKnowledgeBaseStatus
} from '@/service/api/ai';
import { useAuthStore } from '@/store/modules/auth';

defineOptions({ name: 'AIKnowledge' });

const authStore = useAuthStore();
const loadingBases = ref(false);
const loadingDocuments = ref(false);
const baseError = ref('');
const bases = ref<Api.AI.KnowledgeBase[]>([]);
const selected = ref<Api.AI.KnowledgeBase | null>(null);
const baseTotal = ref(0);
const baseQuery = reactive({ page: 1, page_size: 10, keyword: '', status: null as string | null });
const documents = ref<Api.AI.KnowledgeDocument[]>([]);
const documentTotal = ref(0);
const documentQuery = reactive({ page: 1, page_size: 10, status: null as string | null });
const editorVisible = ref(false);
const editorLoading = ref(false);
const editorRef = ref<FormInst | null>(null);
const pickerVisible = ref(false);
const retrievalLoading = ref(false);
const retrievalQuery = ref('');
const retrievalResults = ref<Api.AI.RetrievalResult[]>([]);
const citationLookups = ref<Record<string, Api.AI.CitationLookup>>({});
const aliases = ref<Api.AI.Alias[]>([]);
const deployments = ref<Api.AI.Deployment[]>([]);
const embeddingAliasesLoading = ref(false);

const defaultForm = (): Api.AI.KnowledgeBaseCreate & { id?: string; revision?: number } => ({
  name: '',
  description: '',
  visibility: 'private',
  embedding_alias: '',
  chunking_config: { strategy: 'fixed_tokens', chunk_size: 800, chunk_overlap: 100 },
  retrieval_config: { top_k: 5, min_score: 0, max_context_tokens: 4000 }
});
const form = reactive(defaultForm());
const editing = computed(() => Boolean(form.id));
const embeddingAliasOptions = computed(() => {
  const enabledEmbeddingDeployments = new Map(
    deployments.value
      .filter(item => item.status === 'enabled' && item.model_type === 'embedding')
      .map(item => [item.id, item])
  );
  const options = aliases.value.flatMap(alias => {
    const deployment = enabledEmbeddingDeployments.get(alias.deployment_id);
    if (!deployment) return [];
    return [{ label: `${alias.code} · ${deployment.display_name}`, value: alias.code }];
  });
  if (form.embedding_alias && !options.some(item => item.value === form.embedding_alias)) {
    options.unshift({
      label: `${form.embedding_alias} · ${$t('page.knowledge.currentAliasUnavailable')}`,
      value: form.embedding_alias
    });
  }
  return options;
});
const embeddingAliasValue = computed<string | null>({
  get: () => form.embedding_alias || null,
  set: value => {
    form.embedding_alias = value || '';
  }
});
const rules: FormRules = {
  name: { required: true, message: () => $t('page.knowledge.validation.name'), trigger: 'blur' }
};

function can(permission: string) {
  return authStore.userInfo.buttons.includes(permission) || authStore.userInfo.buttons.includes('*');
}

const baseColumns = computed<DataTableColumns<Api.AI.KnowledgeBase>>(() => [
  {
    title: $t('page.knowledge.name'),
    key: 'name',
    minWidth: 180,
    ellipsis: { tooltip: true },
    render: row =>
      h(
        NButton,
        { text: true, type: selected.value?.id === row.id ? 'primary' : 'default', onClick: () => selectBase(row) },
        { default: () => row.name }
      )
  },
  { title: $t('page.knowledge.code'), key: 'code', width: 140, ellipsis: { tooltip: true } },
  {
    title: $t('page.knowledge.status'),
    key: 'status',
    width: 100,
    render: row => h(CodeTag, { codeSet: CODESET_KEYS.knowledgeBaseStatus, value: row.status })
  },
  {
    title: $t('page.knowledge.updatedAt'),
    key: 'updated_at',
    width: 170,
    render: row => h(DateTimeText, { value: row.updated_at })
  }
]);

const documentColumns = computed<DataTableColumns<Api.AI.KnowledgeDocument>>(() => [
  { title: $t('page.knowledge.documentName'), key: 'original_name', minWidth: 220, ellipsis: { tooltip: true } },
  { title: $t('page.knowledge.contentType'), key: 'content_type', width: 150, ellipsis: { tooltip: true } },
  {
    title: $t('page.knowledge.ingestionStatus'),
    key: 'ingestion_status',
    width: 120,
    render: row => h(CodeTag, { codeSet: CODESET_KEYS.knowledgeDocumentIngestionStatus, value: row.ingestion_status })
  },
  {
    title: $t('page.knowledge.updatedAt'),
    key: 'updated_at',
    width: 170,
    render: row => h(DateTimeText, { value: row.updated_at })
  },
  {
    title: $t('common.action'),
    key: 'actions',
    width: 170,
    fixed: 'right',
    render: row =>
      h(
        NSpace,
        { wrap: false },
        {
          default: () =>
            [
              ['pending', 'failed', 'ready'].includes(row.ingestion_status) && can(AI_ACTIONS.knowledgeDocumentRetry)
                ? h(
                    NButton,
                    {
                      size: 'small',
                      secondary: true,
                      type: 'warning',
                      loading: loadingDocuments.value,
                      onClick: () => retryDocument(row)
                    },
                    { default: () => $t(row.ingestion_status === 'ready' ? 'page.knowledge.reindex' : 'page.knowledge.retry') }
                  )
                : null,
              can(AI_ACTIONS.knowledgeDocumentDelete)
                ? h(
                    NButton,
                    { size: 'small', secondary: true, type: 'error', onClick: () => confirmDeleteDocument(row) },
                    { default: () => $t('common.delete') }
                  )
                : null
            ].filter(Boolean)
        }
      )
  }
]);

async function loadBases() {
  loadingBases.value = true;
  baseError.value = '';
  try {
    const { data, error } = await fetchKnowledgeBases({
      page: baseQuery.page,
      page_size: baseQuery.page_size,
      keyword: baseQuery.keyword.trim() || undefined,
      status: baseQuery.status || undefined
    });
    if (error) {
      baseError.value = error.message || $t('page.knowledge.loadFailed');
      bases.value = [];
      baseTotal.value = 0;
      return;
    }
    bases.value = data?.items || [];
    baseTotal.value = data?.total || 0;
    if (!selected.value && bases.value.length) await selectBase(bases.value[0]);
    if (selected.value && !bases.value.some(item => item.id === selected.value?.id)) selected.value = null;
  } finally {
    loadingBases.value = false;
  }
}

async function loadEmbeddingAliases() {
  if (!can(AI_ACTIONS.aliasList) || !can(AI_ACTIONS.deploymentList)) return;
  embeddingAliasesLoading.value = true;
  try {
    const [aliasResult, deploymentResult] = await Promise.all([
      fetchAIAliases(),
      fetchAIDeployments({ page: 1, page_size: 100, status: 'enabled' })
    ]);
    aliases.value = aliasResult.data || [];
    deployments.value = deploymentResult.data?.items || [];
  } finally {
    embeddingAliasesLoading.value = false;
  }
}

async function selectBase(row: Api.AI.KnowledgeBase) {
  const { data, error } = await fetchKnowledgeBase(row.id);
  if (error || !data) {
    selected.value = null;
    if (!error) window.$message?.error($t('page.knowledge.permissionRevoked'));
    return;
  }
  selected.value = data;
  documentQuery.page = 1;
  retrievalResults.value = [];
  citationLookups.value = {};
  await loadDocuments();
}

async function loadDocuments() {
  if (!selected.value) return;
  loadingDocuments.value = true;
  try {
    const { data, error } = await fetchKnowledgeDocuments({
      knowledge_base_id: selected.value.id,
      page: documentQuery.page,
      page_size: documentQuery.page_size,
      status: documentQuery.status || undefined
    });
    if (error) {
      documents.value = [];
      documentTotal.value = 0;
      return;
    }
    documents.value = data?.items || [];
    documentTotal.value = data?.total || 0;
  } finally {
    loadingDocuments.value = false;
  }
}

function openCreate() {
  Object.assign(form, defaultForm());
  editorVisible.value = true;
}

function openEdit() {
  if (!selected.value) return;
  Object.assign(form, {
    id: selected.value.id,
    revision: selected.value.revision,
    name: selected.value.name,
    description: selected.value.description,
    visibility: selected.value.visibility,
    embedding_alias: selected.value.embedding_alias,
    chunking_config: { ...selected.value.chunking_config },
    retrieval_config: { ...selected.value.retrieval_config }
  });
  editorVisible.value = true;
}

async function saveBase() {
  await editorRef.value?.validate();
  editorLoading.value = true;
  try {
    const request = editing.value
      ? updateKnowledgeBase({
          id: form.id!,
          revision: form.revision!,
          name: form.name,
          description: form.description,
          visibility: form.visibility,
          embedding_alias: form.embedding_alias,
          chunking_config: form.chunking_config,
          retrieval_config: form.retrieval_config
        })
      : createKnowledgeBase({
          name: form.name,
          description: form.description,
          visibility: form.visibility,
          embedding_alias: form.embedding_alias,
          chunking_config: form.chunking_config,
          retrieval_config: form.retrieval_config
        });
    const { data, error } = await request;
    if (error || !data) {
      if (!error) window.$message?.error($t('page.knowledge.saveFailed'));
      return;
    }
    editorVisible.value = false;
    selected.value = data;
    await loadBases();
    await loadDocuments();
  } finally {
    editorLoading.value = false;
  }
}

async function toggleStatus() {
  if (!selected.value) return;
  const next = selected.value.status === 'enabled' ? 'disabled' : 'enabled';
  const { data, error } = await updateKnowledgeBaseStatus(selected.value.id, next, selected.value.revision);
  if (error || !data) {
    if (!error) window.$message?.error($t('page.knowledge.statusFailed'));
    return;
  }
  selected.value = data;
  await loadBases();
}

function confirmDeleteBase() {
  if (!selected.value) return;
  const target = selected.value;
  window.$dialog?.warning({
    title: $t('common.warning'),
    content: $t('page.knowledge.confirmDeleteBase'),
    positiveText: $t('common.confirm'),
    negativeText: $t('common.cancel'),
    onPositiveClick: async () => {
      const { error } = await deleteKnowledgeBase(target.id, target.revision);
      if (error) return;
      selected.value = null;
      baseQuery.page = pageAfterDelete(baseQuery.page, bases.value.length);
      await loadBases();
    }
  });
}

async function addDocuments(items: Api.Attachment.Item[]) {
  if (!selected.value) return;
  loadingDocuments.value = true;
  try {
    for (const item of items) {
      const { error } = await createKnowledgeDocument(selected.value.id, item.id);
      if (error) continue;
    }
    pickerVisible.value = false;
    await loadDocuments();
  } finally {
    loadingDocuments.value = false;
  }
}

async function retryDocument(row: Api.AI.KnowledgeDocument) {
  const { error } = await retryKnowledgeDocument(row.id);
  if (error) return;
  window.$message?.success($t('page.knowledge.retryRequested'));
  await loadDocuments();
}

function confirmDeleteDocument(row: Api.AI.KnowledgeDocument) {
  window.$dialog?.warning({
    title: $t('common.warning'),
    content: $t('page.knowledge.confirmDeleteDocument'),
    positiveText: $t('common.confirm'),
    negativeText: $t('common.cancel'),
    onPositiveClick: async () => {
      const { error } = await deleteKnowledgeDocument(row.id, row.revision);
      if (error) return;
      documentQuery.page = pageAfterDelete(documentQuery.page, documents.value.length);
      await loadDocuments();
    }
  });
}

async function runRetrieval() {
  if (!selected.value || !retrievalQuery.value.trim()) return;
  retrievalLoading.value = true;
  citationLookups.value = {};
  try {
    const { data, error } = await testKnowledgeRetrieval({
      knowledge_base_id: selected.value.id,
      query: retrievalQuery.value.trim()
    });
    if (error) {
      retrievalResults.value = [];
      return;
    }
    retrievalResults.value = data?.items || [];
  } finally {
    retrievalLoading.value = false;
  }
}

async function resolveCitation(item: Api.AI.RetrievalResult) {
  const key = item.citation.chunk_id;
  const { data, error } = await resolveKnowledgeCitation(item.citation);
  if (error || !data) {
    if (!error) window.$message?.error($t('page.knowledge.citationFailed'));
    return;
  }
  citationLookups.value = { ...citationLookups.value, [key]: data };
}

watch(() => [baseQuery.page, baseQuery.page_size], loadBases);
watch(() => [documentQuery.page, documentQuery.page_size, documentQuery.status], loadDocuments);
onMounted(() => {
  loadBases();
  loadEmbeddingAliases();
});
</script>

<template>
  <div class="min-h-500px flex-col gap-16px overflow-hidden lt-sm:overflow-auto">
    <header class="flex flex-wrap items-center justify-between gap-12px">
      <div class="min-w-0">
        <h1 class="m-0 truncate text-20px font-600">{{ $t('page.knowledge.title') }}</h1>
        <div class="mt-4px text-13px text-gray-500">{{ $t('page.knowledge.subtitle') }}</div>
      </div>
      <NButton v-if="can(AI_ACTIONS.knowledgeBaseCreate)" type="primary" @click="openCreate">
        <template #icon><SvgIcon icon="mdi:plus" /></template>
        {{ $t('page.knowledge.createBase') }}
      </NButton>
    </header>

    <NAlert v-if="baseError" type="error" :title="$t('page.knowledge.loadFailed')">
      <div class="flex flex-wrap items-center gap-12px">
        <span class="break-all">{{ baseError }}</span>
        <NButton size="small" @click="loadBases">{{ $t('common.refresh') }}</NButton>
      </div>
    </NAlert>

    <NGrid responsive="screen" cols="1 m:5" :x-gap="16" :y-gap="16" class="min-h-0 flex-1">
      <NGi span="1 m:2" class="min-w-0">
        <section
          class="h-full flex-col rounded-6px border border-gray-200 bg-white p-16px dark:border-gray-700 dark:bg-#18181c"
        >
          <div class="mb-12px flex flex-wrap gap-8px">
            <NInput
              v-model:value="baseQuery.keyword"
              clearable
              class="min-w-160px flex-1"
              :placeholder="$t('page.knowledge.search')"
              @keyup.enter="
                baseQuery.page = 1;
                loadBases();
              "
            />
            <CodeSelect
              v-model:value="baseQuery.status"
              class="w-130px"
              :code-set="CODESET_KEYS.knowledgeBaseStatus"
              clearable
              :placeholder="$t('page.knowledge.allStatus')"
            />
            <NButton
              title="Search"
              @click="
                baseQuery.page = 1;
                loadBases();
              "
            >
              <template #icon><SvgIcon icon="mdi:magnify" /></template>
            </NButton>
          </div>
          <NDataTable
            :columns="baseColumns"
            :data="bases"
            :loading="loadingBases"
            :row-key="row => row.id"
            :scroll-x="590"
            size="small"
          />
          <NPagination
            v-model:page="baseQuery.page"
            v-model:page-size="baseQuery.page_size"
            class="mt-12px justify-end"
            :item-count="baseTotal"
            :page-sizes="[10, 20, 50]"
            show-size-picker
          />
        </section>
      </NGi>

      <NGi span="1 m:3" class="min-w-0">
        <section class="h-full rounded-6px border border-gray-200 bg-white p-16px dark:border-gray-700 dark:bg-#18181c">
          <NEmpty v-if="!selected" class="py-80px" :description="$t('page.knowledge.selectBase')" />
          <template v-else>
            <div class="mb-12px flex flex-wrap items-start justify-between gap-12px">
              <div class="min-w-0">
                <div class="flex flex-wrap items-center gap-8px">
                  <h2 class="m-0 max-w-full break-words text-18px font-600">{{ selected.name }}</h2>
                  <CodeTag :code-set="CODESET_KEYS.knowledgeBaseStatus" :value="selected.status" />
                </div>
                <p class="mb-0 mt-6px max-w-760px break-words text-13px text-gray-500">
                  {{ selected.description || '-' }}
                </p>
              </div>
              <NSpace>
                <NButton v-if="can(AI_ACTIONS.knowledgeBaseUpdate)" size="small" @click="openEdit">
                  <template #icon><SvgIcon icon="mdi:pencil-outline" /></template>
                  {{ $t('common.edit') }}
                </NButton>
                <NButton
                  v-if="can(AI_ACTIONS.knowledgeBaseStatus)"
                  size="small"
                  :type="selected.status === 'enabled' ? 'warning' : 'success'"
                  secondary
                  @click="toggleStatus"
                >
                  {{ selected.status === 'enabled' ? $t('page.knowledge.disable') : $t('page.knowledge.enable') }}
                </NButton>
                <NButton
                  v-if="can(AI_ACTIONS.knowledgeBaseDelete)"
                  size="small"
                  type="error"
                  secondary
                  @click="confirmDeleteBase"
                >
                  <template #icon><SvgIcon icon="mdi:delete-outline" /></template>
                </NButton>
              </NSpace>
            </div>

            <NAlert
              v-if="!selected.embedding_alias"
              type="warning"
              class="mb-12px"
              :title="$t('page.knowledge.noEmbedding')"
            >
              {{ $t('page.knowledge.noEmbeddingHint') }}
            </NAlert>

            <NTabs type="line" animated>
              <NTabPane name="documents" :tab="$t('page.knowledge.documents')">
                <div class="mb-12px flex flex-wrap items-center justify-between gap-8px">
                  <CodeSelect
                    v-model:value="documentQuery.status"
                    class="w-170px"
                    :code-set="CODESET_KEYS.knowledgeDocumentIngestionStatus"
                    clearable
                    :placeholder="$t('page.knowledge.allStatus')"
                  />
                  <NButton
                    v-if="can(AI_ACTIONS.knowledgeDocumentCreate)"
                    type="primary"
                    :loading="loadingDocuments"
                    @click="pickerVisible = true"
                  >
                    <template #icon><SvgIcon icon="mdi:paperclip" /></template>
                    {{ $t('page.knowledge.addDocument') }}
                  </NButton>
                </div>
                <NDataTable
                  :columns="documentColumns"
                  :data="documents"
                  :loading="loadingDocuments"
                  :row-key="row => row.id"
                  :scroll-x="850"
                  size="small"
                />
                <NPagination
                  v-model:page="documentQuery.page"
                  v-model:page-size="documentQuery.page_size"
                  class="mt-12px justify-end"
                  :item-count="documentTotal"
                  :page-sizes="[10, 20, 50]"
                  show-size-picker
                />
              </NTabPane>

              <NTabPane name="retrieval" :tab="$t('page.knowledge.retrievalTest')">
                <NAlert
                  v-if="selected.status !== 'enabled'"
                  class="mb-12px"
                  type="warning"
                  :title="$t('page.knowledge.retrievalRequiresEnabled')"
                />
                <NAlert
                  v-else-if="!selected.current_index_version_id"
                  class="mb-12px"
                  type="warning"
                  :title="$t('page.knowledge.retrievalIndexNotReady')"
                />
                <div class="flex gap-8px lt-sm:flex-col">
                  <NInput
                    v-model:value="retrievalQuery"
                    clearable
                    :placeholder="$t('page.knowledge.queryPlaceholder')"
                    @keyup.enter="runRetrieval"
                  />
                  <NButton
                    type="primary"
                    :loading="retrievalLoading"
                    :disabled="
                      !retrievalQuery.trim() || selected.status !== 'enabled' || !selected.current_index_version_id
                    "
                    @click="runRetrieval"
                  >
                    <template #icon><SvgIcon icon="mdi:database-search-outline" /></template>
                    {{ $t('page.knowledge.run') }}
                  </NButton>
                </div>
                <NSpin :show="retrievalLoading">
                  <NEmpty
                    v-if="!retrievalResults.length && !retrievalLoading"
                    class="py-48px"
                    :description="$t('page.knowledge.noResults')"
                  />
                  <div
                    v-else
                    class="mt-16px max-h-[min(60vh,640px)] flex-col gap-12px overflow-y-auto pr-8px"
                  >
                    <article
                      v-for="item in retrievalResults"
                      :key="item.citation.chunk_id"
                      class="overflow-hidden rounded-6px border border-gray-200 p-12px dark:border-gray-700"
                    >
                      <div class="flex flex-wrap items-center justify-between gap-8px">
                        <div class="min-w-0 break-words font-500">{{ item.citation.title }}</div>
                        <NSpace align="center">
                          <NTag
                            size="small"
                            :type="
                              item.citation.stale || citationLookups[item.citation.chunk_id]?.stale ? 'warning' : 'info'
                            "
                          >
                            {{ Math.round(item.score * 100) }}%
                          </NTag>
                          <NButton size="tiny" secondary @click="resolveCitation(item)">
                            {{ $t('page.knowledge.previewCitation') }}
                          </NButton>
                        </NSpace>
                      </div>
                      <p
                        class="my-10px whitespace-pre-wrap break-words text-13px leading-6 text-gray-600 dark:text-gray-300"
                      >
                        {{ item.text }}
                      </p>
                      <div class="break-all text-12px text-gray-500">
                        {{ item.citation.heading_path || $t('page.knowledge.noHeading') }} · #{{
                          item.citation.ordinal
                        }}
                      </div>
                      <NAlert
                        v-if="citationLookups[item.citation.chunk_id]"
                        class="mt-10px"
                        :type="citationLookups[item.citation.chunk_id].available ? 'success' : 'warning'"
                        :title="
                          citationLookups[item.citation.chunk_id].available
                            ? $t('page.knowledge.citationAvailable')
                            : $t('page.knowledge.citationStale')
                        "
                      />
                    </article>
                  </div>
                </NSpin>
              </NTabPane>
            </NTabs>
          </template>
        </section>
      </NGi>
    </NGrid>

    <NModal
      v-model:show="editorVisible"
      preset="card"
      :title="editing ? $t('page.knowledge.editBase') : $t('page.knowledge.createBase')"
      class="w-680px max-w-[calc(100vw-24px)]"
    >
      <NForm ref="editorRef" :model="form" :rules="rules" label-placement="top">
        <NFormItem path="name" :label="$t('page.knowledge.name')"><NInput v-model:value="form.name" /></NFormItem>
        <NFormItem :label="$t('page.knowledge.description')">
          <NInput v-model:value="form.description" type="textarea" :maxlength="1024" show-count />
        </NFormItem>
        <NGrid responsive="screen" cols="1 s:2" :x-gap="16">
          <NFormItemGi :label="$t('page.knowledge.visibility')">
            <NSelect
              v-model:value="form.visibility"
              :options="[
                { label: $t('page.knowledge.private'), value: 'private' },
                { label: $t('page.knowledge.tenant'), value: 'tenant' }
              ]"
            />
          </NFormItemGi>
          <NFormItemGi :label="$t('page.knowledge.embeddingAlias')">
            <NSelect
              v-model:value="embeddingAliasValue"
              :options="embeddingAliasOptions"
              :loading="embeddingAliasesLoading"
              :placeholder="$t('page.knowledge.embeddingAliasPlaceholder')"
              :disabled="!can(AI_ACTIONS.aliasList) || !can(AI_ACTIONS.deploymentList)"
              clearable
              filterable
            />
          </NFormItemGi>
        </NGrid>
        <NDivider title-placement="left">{{ $t('page.knowledge.chunking') }}</NDivider>
        <NGrid responsive="screen" cols="1 s:2" :x-gap="16">
          <NFormItemGi :label="$t('page.knowledge.chunkSize')">
            <NInputNumber v-model:value="form.chunking_config.chunk_size" class="w-full" :min="100" :max="4000" />
          </NFormItemGi>
          <NFormItemGi :label="$t('page.knowledge.chunkOverlap')">
            <NInputNumber v-model:value="form.chunking_config.chunk_overlap" class="w-full" :min="0" :max="3999" />
          </NFormItemGi>
          <NFormItemGi :label="$t('page.knowledge.topK')">
            <NInputNumber v-model:value="form.retrieval_config.top_k" class="w-full" :min="1" :max="100" />
          </NFormItemGi>
          <NFormItemGi :label="$t('page.knowledge.minScore')">
            <NInputNumber
              v-model:value="form.retrieval_config.min_score"
              class="w-full"
              :min="0"
              :max="1"
              :step="0.05"
            />
          </NFormItemGi>
        </NGrid>
      </NForm>
      <template #footer>
        <div class="flex justify-end gap-8px">
          <NButton @click="editorVisible = false">{{ $t('common.cancel') }}</NButton>
          <NButton type="primary" :loading="editorLoading" @click="saveBase">{{ $t('common.confirm') }}</NButton>
        </div>
      </template>
    </NModal>

    <AttachmentPicker
      v-model:show="pickerVisible"
      multiple
      :max-count="20"
      :selected-ids="documents.map(item => item.attachment_id)"
      @confirm="addDocuments"
    />
  </div>
</template>
