<script setup lang="ts">
import { computed, h, onMounted, reactive, ref } from 'vue';
import type { DataTableColumns } from 'naive-ui';
import { NButton, NCard, NDataTable, NDescriptions, NDescriptionsItem, NDrawer, NDrawerContent, NInput, NPagination, NPopconfirm, NSelect, NSpace, NTag } from 'naive-ui';
import { useAuth } from '@/hooks/business/auth';
import { OPERATIONS_ACTIONS } from '@/constants/operations';
import { $t } from '@/locales';
import { fetchOutboxDetail, fetchOutboxList, fetchRetryOutbox } from '@/service/api';
import { formatDateTime } from '@/utils/datetime';

defineOptions({ name: 'OperationsOutbox' });
const { hasAuth } = useAuth();
const loading = ref(false);
const rows = ref<Api.Operations.OutboxEvent[]>([]);
const total = ref(0);
const query = reactive({ current: 1, size: 20, event_type: '', status: '' });
const detail = ref<Api.Operations.OutboxDetail | null>(null);
const drawer = ref(false);
const statuses = ['pending', 'processing', 'published', 'failed', 'dead'];
const statusOptions = computed(() => statuses.map(value => ({ value, label: $t(`operations.status.${value}` as any) })));
const columns = computed<DataTableColumns<Api.Operations.OutboxEvent>>(() => [
  { title: $t('operations.eventType'), key: 'event_type', ellipsis: { tooltip: true } },
  { title: $t('operations.aggregate'), key: 'aggregate_id', ellipsis: { tooltip: true }, render: r => `${r.aggregate_type || '-'}/${r.aggregate_id || '-'}` },
  { title: $t('operations.statusLabel'), key: 'status', width: 120, render: r => h(NTag, { size: 'small' }, { default: () => $t(`operations.status.${r.status}` as any) }) },
  { title: $t('operations.attempts'), key: 'attempts', width: 90 },
  { title: $t('operations.occurredAt'), key: 'occurred_at', width: 180, render: r => formatDateTime(r.occurred_at) },
  { title: $t('common.action'), key: 'action', width: 170, render: r => h(NSpace, null, { default: () => [
    ...(hasAuth(OPERATIONS_ACTIONS.outboxListDeliveries) ? [h(NButton, { size: 'small', onClick: () => openDetail(r.id) }, { default: () => $t('common.detail') })] : []),
    ...(hasAuth(OPERATIONS_ACTIONS.outboxRetry) && ['failed', 'dead'].includes(r.status) ? [h(NPopconfirm, { onPositiveClick: () => retry(r.id) }, { trigger: () => h(NButton, { size: 'small', type: 'primary' }, { default: () => $t('operations.retry') }), default: () => $t('operations.confirmAction') })] : [])
  ] }) }
]);
async function load() {
  loading.value = true;
  try {
    const res = await fetchOutboxList({ ...query, event_type: query.event_type || undefined, status: query.status || undefined });
    if (!res.error && res.data) { rows.value = res.data.items || []; total.value = res.data.total || 0; }
  } finally { loading.value = false; }
}
async function openDetail(id: string) { drawer.value = true; detail.value = null; const res = await fetchOutboxDetail(id); if (!res.error && res.data) detail.value = res.data; }
async function retry(id: string) { const res = await fetchRetryOutbox(id); if (!res.error) { window.$message?.success($t('operations.success')); await load(); } }
function search() { query.current = 1; load(); }
onMounted(load);
</script>

<template>
  <NCard :title="$t('operations.outboxTitle')" :bordered="false">
    <NSpace class="mb-12px">
      <NInput v-model:value="query.event_type" clearable :placeholder="$t('operations.eventType')" @keyup.enter="search" />
      <NSelect v-model:value="query.status" clearable class="w-180px" :options="statusOptions" :placeholder="$t('operations.statusLabel')" />
      <NButton type="primary" @click="search">{{ $t('common.search') }}</NButton>
    </NSpace>
    <NDataTable :columns="columns" :data="rows" :loading="loading" :row-key="row => row.id" :scroll-x="1000" />
    <div class="mt-16px flex justify-end"><NPagination v-model:page="query.current" v-model:page-size="query.size" :item-count="total" show-size-picker :page-sizes="[10,20,50]" @update:page="load" @update:page-size="search" /></div>
  </NCard>
  <NDrawer v-model:show="drawer" :width="620">
    <NDrawerContent :title="$t('operations.outboxDetail')" closable>
      <template v-if="detail">
        <NDescriptions :column="1" bordered>
          <NDescriptionsItem :label="$t('operations.eventType')">{{ detail.event.event_type }}</NDescriptionsItem>
          <NDescriptionsItem :label="$t('operations.statusLabel')">{{ $t(`operations.status.${detail.event.status}` as any) }}</NDescriptionsItem>
          <NDescriptionsItem :label="$t('operations.payload')"><pre class="m-0 whitespace-pre-wrap break-all">{{ JSON.stringify(detail.event.payload, null, 2) }}</pre></NDescriptionsItem>
        </NDescriptions>
        <NDataTable
          class="mt-16px" :data="detail.deliveries" :columns="[
            { title: $t('operations.handler'), key: 'handler_name' },
            { title: $t('operations.statusLabel'), key: 'status' },
            { title: $t('operations.attempts'), key: 'attempts' },
            { title: $t('operations.error'), key: 'last_error' }
          ]"
        />
      </template>
    </NDrawerContent>
  </NDrawer>
</template>
