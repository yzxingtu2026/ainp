<script setup lang="ts">
import { computed, h, onMounted, reactive, ref } from 'vue';
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
  NInput,
  NPagination,
  NSpace,
  NTag
} from 'naive-ui';
import { $t } from '@/locales';
import { fetchAuditLogDetail, fetchAuditLogList } from '@/service/api';
import { formatDateTime } from '@/utils/datetime';
import { usePageContextProvider } from '@/composables/page-context';

defineOptions({ name: 'OperationsAuditLogs' });
const loading = ref(false);
const rows = ref<Api.Operations.AuditLog[]>([]);
const total = ref(0);
const drawer = ref(false);
const detail = ref<Api.Operations.AuditLog | null>(null);
const range = ref<[number, number] | null>([Date.now() - 86400000, Date.now()]);
const query = reactive({
  page: 1,
  page_size: 20,
  user_keyword: '',
  actor_type: '',
  action_id: '',
  result: '',
  resource_type: '',
  resource_id: '',
  request_id: '',
  client: ''
});
usePageContextProvider(() => ({
  schema_version: '1.0',
  page_type: 'operations.audit-logs',
  route_name: 'operations_audit-logs',
  mode: drawer.value && detail.value ? 'detail' : 'list',
  resource_refs:
    drawer.value && detail.value
      ? [{ resource_type: 'operations.audit-log', resource_id: detail.value.id, revision: detail.value.occurred_at }]
      : undefined,
  ui_state:
    drawer.value && detail.value
      ? undefined
      : {
          actor_type: query.actor_type,
          action_id: query.action_id,
          result: query.result,
          resource_type: query.resource_type,
          client: query.client
        }
}));
const actor = (row: Api.Operations.AuditLog) => {
  const identity = [row.actor_type, row.user_id].filter(Boolean).join(': ');
  return row.actor_name ? `${row.actor_name} (${identity})` : identity || '-';
};
const columns = computed<DataTableColumns<Api.Operations.AuditLog>>(() => [
  { title: $t('auditLog.occurredAt'), key: 'occurred_at', width: 180, render: r => formatDateTime(r.occurred_at) },
  { title: $t('auditLog.actor'), key: 'user_id', width: 180, ellipsis: { tooltip: true }, render: actor },
  {
    title: 'Action',
    key: 'action_id',
    width: 230,
    ellipsis: { tooltip: true },
    render: r => `${r.action_name || r.action_id} (${r.action_id})`
  },
  {
    title: $t('auditLog.result'),
    key: 'result',
    width: 100,
    render: r =>
      h(
        NTag,
        { type: r.result === 'success' ? 'success' : 'error', size: 'small' },
        { default: () => $t(`auditLog.${r.result}` as any) }
      )
  },
  {
    title: $t('auditLog.resource'),
    key: 'resource_id',
    width: 190,
    ellipsis: { tooltip: true },
    render: r => [r.resource_type, r.resource_id].filter(Boolean).join('/') || '-'
  },
  { title: $t('auditLog.client'), key: 'client', width: 110 },
  { title: $t('auditLog.summary'), key: 'summary', minWidth: 180, ellipsis: { tooltip: true } },
  { title: $t('auditLog.requestId'), key: 'request_id', width: 190, ellipsis: { tooltip: true } },
  {
    title: $t('common.action'),
    key: 'action',
    width: 90,
    fixed: 'right',
    render: r => h(NButton, { size: 'small', onClick: () => openDetail(r.id) }, { default: () => $t('common.detail') })
  }
]);
function payload(): Api.Operations.AuditLogQuery {
  return {
    ...query,
    start_at: range.value?.[0] ? new Date(range.value[0]).toISOString() : undefined,
    end_at: range.value?.[1] ? new Date(range.value[1]).toISOString() : undefined,
    user_keyword: query.user_keyword || undefined,
    actor_type: query.actor_type || undefined,
    action_id: query.action_id || undefined,
    result: query.result || undefined,
    resource_type: query.resource_type || undefined,
    resource_id: query.resource_id || undefined,
    request_id: query.request_id || undefined,
    client: query.client || undefined
  };
}
async function load() {
  loading.value = true;
  try {
    const res = await fetchAuditLogList(payload());
    if (!res.error && res.data) {
      rows.value = res.data.items;
      total.value = res.data.total;
    }
  } finally {
    loading.value = false;
  }
}
async function openDetail(id: string) {
  drawer.value = true;
  detail.value = null;
  const res = await fetchAuditLogDetail(id);
  if (!res.error && res.data) detail.value = res.data.audit_log;
}
function search() {
  query.page = 1;
  load();
}
onMounted(load);
</script>

<template>
  <div class="min-h-500px">
    <NCard :title="$t('auditLog.title')" :bordered="false">
      <NSpace class="mb-12px" wrap>
        <NDatePicker v-model:value="range" type="datetimerange" clearable class="w-360px" />
        <NInput v-model:value="query.user_keyword" clearable :placeholder="$t('auditLog.userKeyword')" />
        <NInput v-model:value="query.action_id" clearable placeholder="Action ID" />
        <AppSelect
          v-model:value="query.result"
          clearable
          class="w-140px"
          :options="[
            { label: $t('auditLog.success'), value: 'success' },
            { label: $t('auditLog.failure'), value: 'failure' }
          ]"
          :placeholder="$t('auditLog.result')"
        />
        <NInput v-model:value="query.actor_type" clearable :placeholder="$t('auditLog.actorType')" />
        <NInput v-model:value="query.resource_type" clearable :placeholder="$t('auditLog.resourceType')" />
        <NInput v-model:value="query.resource_id" clearable :placeholder="$t('auditLog.resourceId')" />
        <NInput v-model:value="query.request_id" clearable :placeholder="$t('auditLog.requestId')" />
        <NInput v-model:value="query.client" clearable :placeholder="$t('auditLog.client')" />
        <NButton type="primary" @click="search">{{ $t('common.search') }}</NButton>
      </NSpace>
      <NDataTable :columns="columns" :data="rows" :loading="loading" :row-key="r => r.id" :scroll-x="1500" />
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
    </NCard>
    <NDrawer v-model:show="drawer" :width="620">
      <NDrawerContent :title="$t('auditLog.detail')" closable>
        <NDescriptions v-if="detail" :column="1" bordered>
          <NDescriptionsItem :label="$t('auditLog.occurredAt')">
            {{ formatDateTime(detail.occurred_at) }}
          </NDescriptionsItem>
          <NDescriptionsItem :label="$t('auditLog.actor')">{{ actor(detail) }}</NDescriptionsItem>
          <NDescriptionsItem label="Action">
            {{ detail.action_name || detail.action_id }} ({{ detail.action_id }})
          </NDescriptionsItem>
          <NDescriptionsItem :label="$t('auditLog.permission')">{{ detail.permission }}</NDescriptionsItem>
          <NDescriptionsItem :label="$t('auditLog.result')">
            {{ $t(`auditLog.${detail.result}` as any) }} ({{ detail.error_code }})
          </NDescriptionsItem>
          <NDescriptionsItem :label="$t('auditLog.resource')">
            {{ [detail.resource_type, detail.resource_id].filter(Boolean).join('/') || '-' }}
          </NDescriptionsItem>
          <NDescriptionsItem :label="$t('auditLog.requestId')">{{ detail.request_id || '-' }}</NDescriptionsItem>
          <NDescriptionsItem label="Trace ID">{{ detail.trace_id || '-' }}</NDescriptionsItem>
          <NDescriptionsItem label="Agent Run ID">{{ detail.agent_run_id || '-' }}</NDescriptionsItem>
          <NDescriptionsItem label="Tool Invocation ID">{{ detail.tool_invocation_id || '-' }}</NDescriptionsItem>
          <NDescriptionsItem :label="$t('auditLog.client')">{{ detail.client || '-' }}</NDescriptionsItem>
          <NDescriptionsItem :label="$t('auditLog.summary')">{{ detail.summary || '-' }}</NDescriptionsItem>
        </NDescriptions>
      </NDrawerContent>
    </NDrawer>
  </div>
</template>
