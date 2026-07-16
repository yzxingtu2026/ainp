<script setup lang="ts">
import { computed, h, onMounted, reactive, ref } from 'vue';
import type { DataTableColumns } from 'naive-ui';
import { NButton, NCard, NDataTable, NDescriptions, NDescriptionsItem, NDrawer, NDrawerContent, NInput, NPagination, NPopconfirm, NSelect, NSpace, NTag } from 'naive-ui';
import { useAuth } from '@/hooks/business/auth';
import { OPERATIONS_ACTIONS } from '@/constants/operations';
import { $t } from '@/locales';
import { fetchCancelJob, fetchJobDetail, fetchJobList, fetchRetryJob } from '@/service/api';
import { formatDateTime } from '@/utils/datetime';

defineOptions({ name: 'OperationsJobs' });
const { hasAuth } = useAuth();
const loading = ref(false);
const rows = ref<Api.Operations.JobRun[]>([]);
const total = ref(0);
const query = reactive({ current: 1, size: 20, job_type: '', status: '' });
const detail = ref<Api.Operations.JobRun | null>(null);
const drawer = ref(false);
const statuses = ['pending_enqueue', 'queued', 'running', 'retrying', 'succeeded', 'failed', 'dead', 'cancelled'];
const statusOptions = computed(() => statuses.map(value => ({ value, label: $t(`operations.status.${value}` as any) })));
const canCancel = (row: Api.Operations.JobRun) => hasAuth(OPERATIONS_ACTIONS.jobCancel) && ['pending_enqueue', 'queued', 'running', 'retrying'].includes(row.status);
const canRetry = (row: Api.Operations.JobRun) => hasAuth(OPERATIONS_ACTIONS.jobRetry) && ['failed', 'dead'].includes(row.status);
const columns = computed<DataTableColumns<Api.Operations.JobRun>>(() => [
  { title: $t('operations.jobType'), key: 'job_type', ellipsis: { tooltip: true } },
  { title: $t('operations.statusLabel'), key: 'status', width: 130, render: r => h(NTag, { size: 'small' }, { default: () => $t(`operations.status.${r.status}` as any) }) },
  { title: $t('operations.progress'), key: 'progress', width: 100, render: r => `${r.progress}%` },
  { title: $t('operations.attempts'), key: 'attempt', width: 100, render: r => `${r.attempt}/${r.max_attempts}` },
  { title: $t('operations.createdAt'), key: 'created_at', width: 180, render: r => formatDateTime(r.created_at) },
  { title: $t('common.action'), key: 'action', width: 220, render: r => h(NSpace, null, { default: () => [
    ...(hasAuth(OPERATIONS_ACTIONS.jobGet) ? [h(NButton, { size: 'small', onClick: () => openDetail(r.id) }, { default: () => $t('common.detail') })] : []),
    ...(canCancel(r) ? [h(NPopconfirm, { onPositiveClick: () => mutate(() => fetchCancelJob(r.id)) }, { trigger: () => h(NButton, { size: 'small', type: 'warning' }, { default: () => $t('operations.cancelJob') }), default: () => $t('operations.confirmAction') })] : []),
    ...(canRetry(r) ? [h(NPopconfirm, { onPositiveClick: () => mutate(() => fetchRetryJob(r.id)) }, { trigger: () => h(NButton, { size: 'small', type: 'primary' }, { default: () => $t('operations.retry') }), default: () => $t('operations.confirmAction') })] : [])
  ] }) }
]);
async function load() {
  loading.value = true;
  try {
    const res = await fetchJobList({ ...query, job_type: query.job_type || undefined, status: query.status || undefined });
    if (!res.error && res.data) { rows.value = res.data.items || []; total.value = res.data.total || 0; }
  } finally { loading.value = false; }
}
async function openDetail(id: string) {
  drawer.value = true; detail.value = null;
  const res = await fetchJobDetail(id);
  if (!res.error && res.data) detail.value = res.data.job;
}
async function mutate(call: () => ReturnType<typeof fetchCancelJob>) {
  const res = await call();
  if (!res.error) { window.$message?.success($t('operations.success')); await load(); }
}
function search() { query.current = 1; load(); }
onMounted(load);
</script>

<template>
  <div class="min-h-500px">
    <NCard :title="$t('operations.jobsTitle')" :bordered="false">
      <NSpace class="mb-12px">
        <NInput v-model:value="query.job_type" clearable :placeholder="$t('operations.jobType')" @keyup.enter="search" />
        <NSelect v-model:value="query.status" clearable class="w-180px" :options="statusOptions" :placeholder="$t('operations.statusLabel')" />
        <NButton type="primary" @click="search">{{ $t('common.search') }}</NButton>
      </NSpace>
      <NDataTable :columns="columns" :data="rows" :loading="loading" :row-key="row => row.id" :scroll-x="1000" />
      <div class="mt-16px flex justify-end"><NPagination v-model:page="query.current" v-model:page-size="query.size" :item-count="total" show-size-picker :page-sizes="[10,20,50]" @update:page="load" @update:page-size="search" /></div>
    </NCard>
    <NDrawer v-model:show="drawer" :width="560">
      <NDrawerContent :title="$t('operations.jobDetail')" closable>
        <NDescriptions v-if="detail" :column="1" bordered>
          <NDescriptionsItem :label="$t('operations.jobType')">{{ detail.job_type }}</NDescriptionsItem>
          <NDescriptionsItem :label="$t('operations.statusLabel')">{{ $t(`operations.status.${detail.status}` as any) }}</NDescriptionsItem>
          <NDescriptionsItem :label="$t('operations.progress')">{{ detail.progress }}% {{ detail.progress_message }}</NDescriptionsItem>
          <NDescriptionsItem :label="$t('operations.createdAt')">{{ formatDateTime(detail.created_at) }}</NDescriptionsItem>
          <NDescriptionsItem :label="$t('operations.updatedAt')">{{ formatDateTime(detail.updated_at) }}</NDescriptionsItem>
          <NDescriptionsItem :label="$t('operations.error')">{{ detail.last_error || '-' }}</NDescriptionsItem>
        </NDescriptions>
      </NDrawerContent>
    </NDrawer>
  </div>
</template>
