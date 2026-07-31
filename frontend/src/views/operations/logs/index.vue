<script setup lang="ts">
import { computed, h, onMounted, reactive, ref, watch } from 'vue';
import { useRoute } from 'vue-router';
import type { DataTableColumns } from 'naive-ui';
import {
  NButton,
  NCard,
  NDataTable,
  NDatePicker,
  NDescriptions,
  NDescriptionsItem,
  NDrawer,
  NDrawerContent,
  NEmpty,
  NInput,
  NPagination,
  NSpace,
  NSpin,
  NTabPane,
  NTabs,
  NTag
} from 'naive-ui';
import { $t } from '@/locales';
import { fetchLogDetail, fetchLogQuery, fetchLogSources } from '@/service/api';
import { formatDateTime } from '@/utils/datetime';

defineOptions({ name: 'OperationsLogs' });
const route = useRoute();
const loading = ref(false);
const sourceLoading = ref(false);
const sources = ref<Api.Operations.LogSource[]>([]);
const activeSource = ref('');
const rows = ref<Api.Operations.LogEntry[]>([]);
const total = ref(0);
const range = ref<[number, number] | null>([Date.now() - 15 * 60 * 1000, Date.now()]);
const drawer = ref(false);
const detail = ref<Api.Operations.LogEntry | null>(null);
const query = reactive({
  page: 1,
  page_size: 20,
  level: '',
  service: '',
  component: '',
  keyword: '',
  request_id: '',
  trace_id: '',
  action_id: '',
  job_type: '',
  job_run_id: '',
  event_id: ''
});
const levelOptions = computed(() =>
  (
    sources.value.find(s => s.name === activeSource.value)?.capabilities.levels || ['debug', 'info', 'warn', 'error']
  ).map(value => ({ value, label: value.toUpperCase() }))
);
const levelType = (level: string) =>
  (({ debug: 'default', info: 'info', warn: 'warning', error: 'error' })[level] || 'default') as
    | 'default'
    | 'info'
    | 'warning'
    | 'error';
const columns = computed<DataTableColumns<Api.Operations.LogEntry>>(() => [
  { title: $t('logCenter.timestamp'), key: 'timestamp', width: 180, render: r => formatDateTime(r.timestamp) },
  {
    title: $t('logCenter.level'),
    key: 'level',
    width: 90,
    render: r => h(NTag, { size: 'small', type: levelType(r.level) }, { default: () => r.level.toUpperCase() })
  },
  { title: $t('logCenter.service'), key: 'service', width: 130, ellipsis: { tooltip: true } },
  { title: $t('logCenter.component'), key: 'component', width: 140, ellipsis: { tooltip: true } },
  { title: $t('logCenter.message'), key: 'message', minWidth: 360, ellipsis: { tooltip: true } },
  { title: 'Request ID', key: 'request_id', width: 180, ellipsis: { tooltip: true } },
  {
    title: $t('common.action'),
    key: 'action',
    width: 90,
    fixed: 'right',
    render: r => h(NButton, { size: 'small', onClick: () => openDetail(r.id) }, { default: () => $t('common.detail') })
  }
]);
function payload(): Api.Operations.LogQuery {
  return {
    ...query,
    source: activeSource.value,
    start_at: range.value?.[0] ? new Date(range.value[0]).toISOString() : undefined,
    end_at: range.value?.[1] ? new Date(range.value[1]).toISOString() : undefined,
    level: query.level || undefined,
    service: query.service || undefined,
    component: query.component || undefined,
    keyword: query.keyword || undefined,
    request_id: query.request_id || undefined,
    trace_id: query.trace_id || undefined,
    action_id: query.action_id || undefined,
    job_type: query.job_type || undefined,
    job_run_id: query.job_run_id || undefined,
    event_id: query.event_id || undefined
  };
}
async function loadSources() {
  sourceLoading.value = true;
  try {
    const res = await fetchLogSources();
    if (!res.error && res.data) {
      sources.value = res.data.sources || [];
      const requestedSource = typeof route.query.source === 'string' ? route.query.source : '';
      activeSource.value = sources.value.some(source => source.name === requestedSource)
        ? requestedSource
        : sources.value[0]?.name || '';
    }
  } finally {
    sourceLoading.value = false;
  }
}
function applyRouteQuery() {
  for (const key of [
    'level',
    'service',
    'component',
    'keyword',
    'request_id',
    'trace_id',
    'action_id',
    'job_type',
    'job_run_id',
    'event_id'
  ] as const) {
    const value = route.query[key];
    if (typeof value === 'string') query[key] = value;
  }
  const start = typeof route.query.start_at === 'string' ? Date.parse(route.query.start_at) : Number.NaN;
  const end = typeof route.query.end_at === 'string' ? Date.parse(route.query.end_at) : Number.NaN;
  if (Number.isFinite(start) && Number.isFinite(end)) range.value = [start, end];
}
async function load() {
  if (!activeSource.value) return;
  loading.value = true;
  try {
    const res = await fetchLogQuery(payload());
    if (!res.error && res.data) {
      rows.value = res.data.items || [];
      total.value = res.data.total || 0;
    }
  } finally {
    loading.value = false;
  }
}
async function openDetail(id: string) {
  drawer.value = true;
  detail.value = null;
  const res = await fetchLogDetail(activeSource.value, id);
  if (!res.error && res.data) detail.value = res.data.log_entry;
}
function search() {
  query.page = 1;
  load();
}
watch(activeSource, () => {
  rows.value = [];
  total.value = 0;
  query.page = 1;
  if (activeSource.value) load();
});
onMounted(() => {
  applyRouteQuery();
  loadSources();
});
</script>

<template>
  <div class="min-h-500px">
    <NCard :title="$t('logCenter.title')" :bordered="false">
      <NSpin :show="sourceLoading">
        <NEmpty v-if="sources.length === 0" class="py-64px" :description="$t('logCenter.noProvider')" />
        <template v-else>
          <NTabs v-model:value="activeSource" type="line" animated>
            <NTabPane v-for="source in sources" :key="source.name" :name="source.name" :tab="source.display_name" />
          </NTabs>
          <NSpace class="mb-12px" wrap>
            <NDatePicker v-model:value="range" type="datetimerange" clearable class="w-360px" />
            <AppSelect
              v-model:value="query.level"
              clearable
              class="w-130px"
              :options="levelOptions"
              :placeholder="$t('logCenter.level')"
            />
            <NInput v-model:value="query.service" clearable :placeholder="$t('logCenter.service')" />
            <NInput v-model:value="query.component" clearable :placeholder="$t('logCenter.component')" />
            <NInput
              v-model:value="query.keyword"
              clearable
              :placeholder="$t('logCenter.keyword')"
              @keyup.enter="search"
            />
            <NInput v-model:value="query.request_id" clearable placeholder="Request ID" />
            <NInput v-model:value="query.trace_id" clearable placeholder="Trace ID" />
            <NInput v-model:value="query.action_id" clearable placeholder="Action ID" />
            <NInput v-model:value="query.job_type" clearable placeholder="Job Type" />
            <NInput v-model:value="query.job_run_id" clearable placeholder="Job Run ID" />
            <NInput v-model:value="query.event_id" clearable placeholder="Event ID" />
            <NButton type="primary" @click="search">{{ $t('common.search') }}</NButton>
          </NSpace>
          <NDataTable :columns="columns" :data="rows" :loading="loading" :row-key="r => r.id" :scroll-x="1450" />
          <div class="mt-16px flex justify-end">
            <NPagination
              v-model:page="query.page"
              v-model:page-size="query.page_size"
              :item-count="total"
              show-size-picker
              :page-sizes="[10, 20, 50, 100]"
              @update:page="load"
              @update:page-size="search"
            />
          </div>
        </template>
      </NSpin>
    </NCard>
    <NDrawer v-model:show="drawer" :width="680">
      <NDrawerContent :title="$t('logCenter.detail')" closable>
        <NDescriptions v-if="detail" :column="1" bordered>
          <NDescriptionsItem :label="$t('logCenter.timestamp')">
            {{ formatDateTime(detail.timestamp) }}
          </NDescriptionsItem>
          <NDescriptionsItem :label="$t('logCenter.level')">{{ detail.level.toUpperCase() }}</NDescriptionsItem>
          <NDescriptionsItem :label="$t('logCenter.source')">{{ detail.source }}</NDescriptionsItem>
          <NDescriptionsItem :label="$t('logCenter.service')">{{ detail.service || '-' }}</NDescriptionsItem>
          <NDescriptionsItem :label="$t('logCenter.component')">{{ detail.component || '-' }}</NDescriptionsItem>
          <NDescriptionsItem :label="$t('logCenter.message')">
            <pre class="m-0 whitespace-pre-wrap break-all">{{ detail.message }}</pre>
          </NDescriptionsItem>
          <NDescriptionsItem :label="$t('logCenter.correlation')">
            request={{ detail.request_id || '-' }} / trace={{ detail.trace_id || '-' }} / action={{
              detail.action_id || '-'
            }}
            / job={{ detail.job_run_id || '-' }} / event={{ detail.event_id || '-' }}
          </NDescriptionsItem>
          <NDescriptionsItem :label="$t('logCenter.attributes')">
            <pre class="m-0 whitespace-pre-wrap break-all">{{ JSON.stringify(detail.attributes || {}, null, 2) }}</pre>
          </NDescriptionsItem>
        </NDescriptions>
      </NDrawerContent>
    </NDrawer>
  </div>
</template>
