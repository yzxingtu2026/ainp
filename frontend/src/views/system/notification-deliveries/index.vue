<script setup lang="ts">
import { computed, h, onMounted, reactive, ref } from 'vue';
import type { DataTableColumns } from 'naive-ui';
import { NButton, NCard, NDataTable, NPagination, NPopconfirm, NSpace, NTag } from 'naive-ui';
import { $t } from '@/locales';
import { fetchDeliveryList, fetchRetryDelivery } from '@/service/api';
import { formatDateTime } from '@/utils/datetime';

defineOptions({ name: 'SystemNotificationDeliveries' });

const loading = ref(false);
const records = ref<Api.Notification.DeliveryItem[]>([]);
const total = ref(0);
const query = reactive({ current: 1, size: 20 });

const statusTagType: Record<string, 'error' | 'warning' | 'success' | 'default'> = {
  failed: 'error',
  dead: 'error',
  pending: 'warning',
  queued: 'warning',
  sent: 'success',
  delivered: 'success'
};

const channelI18n: Record<string, string> = {
  in_app: 'notificationAdmin.channelInApp',
  inapp: 'notificationAdmin.channelInApp',
  email: 'notificationAdmin.channelEmail',
  sms: 'notificationAdmin.channelSms',
  webhook: 'notificationAdmin.channelWebhook',
  im: 'notificationAdmin.channelIm'
};

const statusI18n: Record<string, string> = {
  pending: 'notificationAdmin.statusPending',
  queued: 'notificationAdmin.statusQueued',
  sent: 'notificationAdmin.statusSent',
  delivered: 'notificationAdmin.statusDelivered',
  failed: 'notificationAdmin.statusFailed',
  dead: 'notificationAdmin.statusDead'
};

const columns = computed<DataTableColumns<Api.Notification.DeliveryItem>>(() => [
  {
    title: $t('notificationAdmin.colChannel'),
    key: 'channel',
    width: 100,
    render: row => $t((channelI18n[row.channel] || row.channel) as any) || row.channel
  },
  {
    title: $t('notificationAdmin.colRecipient'),
    key: 'recipient_name',
    width: 140,
    ellipsis: { tooltip: true },
    render: row => row.recipient_name || row.recipient_id
  },
  {
    title: $t('notificationAdmin.colStatus'),
    key: 'status',
    width: 100,
    render: row =>
      h(
        NTag,
        { type: statusTagType[row.status] || 'default', size: 'small' },
        { default: () => $t((statusI18n[row.status] || row.status) as any) || row.status }
      )
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
    title: $t('common.action'),
    key: 'actions',
    width: 120,
    fixed: 'right',
    render: row => {
      if (row.status !== 'failed' && row.status !== 'dead') return null;
      return h(NSpace, null, {
        default: () => [
          h(
            NPopconfirm,
            { onPositiveClick: () => handleRetry(row) },
            {
              trigger: () => h(NButton, { size: 'small', type: 'primary' }, { default: () => $t('notificationAdmin.retry') }),
              default: () => $t('notificationAdmin.retryConfirm')
            }
          )
        ]
      });
    }
  }
]);

async function loadList() {
  loading.value = true;
  try {
    const res = await fetchDeliveryList({
      current: query.current,
      size: query.size
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
      <NDataTable :columns="columns" :data="records" :loading="loading" :row-key="row => row.id" :scroll-x="1200" />
      <div class="mt-16px flex justify-end">
        <NPagination
          v-model:page="query.current"
          v-model:page-size="query.size"
          :item-count="total"
          show-size-picker
          :page-sizes="[10, 20, 50]"
          @update:page="loadList"
          @update:page-size="query.current = 1; loadList()"
        />
      </div>
    </NCard>
  </div>
</template>
