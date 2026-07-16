<script setup lang="ts">
import { h, onMounted, reactive, ref } from 'vue';
import type { DataTableColumns } from 'naive-ui';
import { NButton, NCard, NDataTable, NPopconfirm, NTag } from 'naive-ui';
import { $t } from '@/locales';
import { fetchDeliveryList, fetchRetryDelivery } from '@/service/api';
import { formatDateTime } from '@/utils/datetime';

defineOptions({ name: 'SystemNotificationDeliveries' });

const loading = ref(false);
const records = ref<Api.Notification.DeliveryItem[]>([]);
const total = ref(0);
const pagination = reactive({
  page: 1,
  pageSize: 20,
  showSizePicker: true,
  pageSizes: [10, 20, 50],
  onChange: (page: number) => {
    pagination.page = page;
    loadList();
  },
  onUpdatePageSize: (size: number) => {
    pagination.pageSize = size;
    pagination.page = 1;
    loadList();
  }
});

const statusTagType: Record<string, 'error' | 'warning' | 'success' | 'default'> = {
  failed: 'error',
  dead: 'error',
  pending: 'warning',
  sent: 'success',
  delivered: 'success'
};

const columns: DataTableColumns<Api.Notification.DeliveryItem> = [
  { title: $t('notificationAdmin.colId'), key: 'id', width: 120, ellipsis: { tooltip: true } },
  { title: $t('notificationAdmin.colChannel'), key: 'channel', width: 90 },
  { title: $t('notificationAdmin.colRecipient'), key: 'recipient_id', width: 120, ellipsis: { tooltip: true } },
  {
    title: $t('notificationAdmin.colStatus'),
    key: 'status',
    width: 100,
    render: row => h(NTag, { type: statusTagType[row.status] || 'default', size: 'small' }, { default: () => row.status })
  },
  {
    title: $t('notificationAdmin.colAttempts'),
    key: 'attempts',
    width: 90,
    render: row => `${row.attempts}/${row.max_attempts}`
  },
  { title: $t('notificationAdmin.colLastError'), key: 'last_error', minWidth: 200, ellipsis: { tooltip: true } },
  {
    title: $t('notificationAdmin.colFailedAt'),
    key: 'failed_at',
    width: 170,
    render: row => (row.failed_at ? formatDateTime(row.failed_at) : '-')
  },
  {
    title: $t('notificationAdmin.colCreatedAt'),
    key: 'created_at',
    width: 170,
    render: row => formatDateTime(row.created_at)
  },
  {
    title: $t('notificationAdmin.colActions'),
    key: 'actions',
    width: 100,
    render: row => {
      if (row.status !== 'failed' && row.status !== 'dead') return null;
      return h(
        NPopconfirm,
        { onPositiveClick: () => handleRetry(row) },
        {
          trigger: () => h(NButton, { text: true, type: 'primary', size: 'small' }, { default: () => $t('notificationAdmin.retry') }),
          default: () => $t('notificationAdmin.retryConfirm')
        }
      );
    }
  }
];

async function loadList() {
  loading.value = true;
  try {
    const res = await fetchDeliveryList({
      current: pagination.page,
      size: pagination.pageSize
    });
    if (!res.error && res.data) {
      records.value = res.data.items || [];
      total.value = res.data.total || 0;
    }
  } finally {
    loading.value = false;
  }
}

async function handleRetry(row: Api.Notification.DeliveryItem) {
  const res = await fetchRetryDelivery(row.id);
  if (!res.error) {
    window.$message?.success($t('notificationAdmin.retrySuccess'));
    await loadList();
  }
}

onMounted(loadList);
</script>

<template>
  <div class="min-h-500px flex-col-stretch gap-16px overflow-hidden lt-sm:overflow-auto">
    <NCard :title="$t('notificationAdmin.deliveryTitle')" :bordered="false" class="card-wrapper">
      <template #header-extra>
        <NButton type="primary" size="small" @click="loadList">{{ $t('common.refresh') }}</NButton>
      </template>
      <NDataTable
        :columns="columns"
        :data="records"
        :loading="loading"
        :bordered="false"
        :pagination="pagination"
        :row-key="(row: Api.Notification.DeliveryItem) => row.id"
        size="small"
      />
    </NCard>
  </div>
</template>
