<script setup lang="ts">
import { computed, h, onMounted, reactive, ref } from 'vue';
import type { DataTableColumns } from 'naive-ui';
import {
  NButton, NCard, NDataTable, NDescriptions, NDescriptionsItem, NDivider,
  NDrawer, NDrawerContent, NEmpty, NPagination, NPopconfirm, NSelect, NSpace, NTag
} from 'naive-ui';
import { $t } from '@/locales';
import { useAuth } from '@/hooks/business/auth';
import { OPERATIONS_ACTIONS } from '@/constants/operations';
import { fetchDeliveryDetail, fetchDeliveryList, fetchRetryDelivery } from '@/service/api';
import { formatDateTime } from '@/utils/datetime';

defineOptions({ name: 'OperationsNotificationDeliveries' });
const { hasAuth } = useAuth();

const loading = ref(false);
const records = ref<Api.Operations.DeliveryItem[]>([]);
const total = ref(0);
const query = reactive({ current: 1, size: 20, status: '', channel: '' });

// Drawer state
const drawerVisible = ref(false);
const drawerLoading = ref(false);
const detail = ref<Api.Operations.DeliveryDetail | null>(null);

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

const statusOptions = computed(() => [
  { label: $t('notificationAdmin.statusFailed'), value: 'failed' },
  { label: $t('notificationAdmin.statusDead'), value: 'dead' },
  { label: $t('notificationAdmin.statusPending'), value: 'pending' },
  { label: $t('notificationAdmin.statusQueued'), value: 'queued' },
  { label: $t('notificationAdmin.statusSent'), value: 'sent' },
  { label: $t('notificationAdmin.statusDelivered'), value: 'delivered' }
]);

const channelOptions = computed(() =>
  ['in_app', 'email', 'sms', 'webhook', 'im'].map(value => ({
    label: $t(channelI18n[value] as any),
    value
  }))
);

const columns = computed<DataTableColumns<Api.Operations.DeliveryItem>>(() => [
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
  {
    title: $t('notificationAdmin.colCreatedAt'),
    key: 'created_at',
    width: 170,
    render: row => formatDateTime(row.created_at)
  },
  {
    title: $t('common.action'),
    key: 'actions',
    width: 160,
    fixed: 'right',
    render: row =>
      h(NSpace, null, {
        default: () => [
          ...(hasAuth(OPERATIONS_ACTIONS.notificationDeliveryGet)
            ? [
                h(
                  NButton,
                  { size: 'small', type: 'info', ghost: true, onClick: () => openDetail(row) },
                  { default: () => $t('common.detail') }
                )
              ]
            : []),
          ...(hasAuth(OPERATIONS_ACTIONS.notificationDeliveryRetry) && (row.status === 'failed' || row.status === 'dead')
            ? [
                h(
                  NPopconfirm,
                  { onPositiveClick: () => handleRetry(row) },
                  {
                    trigger: () => h(NButton, { size: 'small', type: 'primary' }, { default: () => $t('notificationAdmin.retry') }),
                    default: () => $t('notificationAdmin.retryConfirm')
                  }
                )
              ]
            : [])
        ]
      })
  }
]);

async function loadList() {
  loading.value = true;
  try {
    const res = await fetchDeliveryList({
      current: query.current,
      size: query.size,
      status: query.status || undefined,
      channel: query.channel || undefined
    });
    if (!res.error && res.data) {
      records.value = res.data.items || [];
      total.value = res.data.total || 0;
    }
  } finally {
    loading.value = false;
  }
}

async function openDetail(row: Api.Operations.DeliveryItem) {
  drawerVisible.value = true;
  drawerLoading.value = true;
  detail.value = null;
  try {
    const res = await fetchDeliveryDetail(row.id);
    if (!res.error && res.data) {
      detail.value = res.data;
    }
  } finally {
    drawerLoading.value = false;
  }
}

async function handleRetry(row: Api.Operations.DeliveryItem) {
  const res = await fetchRetryDelivery(row.id);
  if (!res.error) {
    window.$message?.success($t('notificationAdmin.retrySuccess'));
    await loadList();
  }
}

onMounted(loadList);

function handleFilterChange() {
  query.current = 1;
  loadList();
}
</script>

<template>
  <div class="min-h-500px flex-col-stretch gap-16px overflow-hidden lt-sm:overflow-auto">
    <NCard :title="$t('notificationAdmin.deliveryTitle')" :bordered="false" class="card-wrapper">
      <NSpace class="mb-12px">
        <NSelect
          v-model:value="query.status"
          clearable
          class="w-160px"
          :options="statusOptions"
          :placeholder="$t('notificationAdmin.filterStatus')"
          @update:value="handleFilterChange"
        />
        <NSelect
          v-model:value="query.channel"
          clearable
          class="w-160px"
          :options="channelOptions"
          :placeholder="$t('notificationAdmin.filterChannel')"
          @update:value="handleFilterChange"
        />
      </NSpace>
      <NDataTable :columns="columns" :data="records" :loading="loading" :row-key="row => row.id" :scroll-x="900" />
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

    <NDrawer v-model:show="drawerVisible" :width="520" placement="right">
      <NDrawerContent :title="$t('notificationAdmin.detailTitle')" :loading="drawerLoading" closable>
        <template v-if="detail">
          <!-- Basic Info -->
          <NDescriptions :column="1" label-placement="left" bordered size="small">
            <NDescriptionsItem :label="$t('notificationAdmin.colStatus')">
              <NTag :type="statusTagType[detail.status] || 'default'" size="small">
                {{ $t((statusI18n[detail.status] || detail.status) as any) || detail.status }}
              </NTag>
            </NDescriptionsItem>
            <NDescriptionsItem :label="$t('notificationAdmin.colChannel')">
              {{ $t((channelI18n[detail.channel] || detail.channel) as any) || detail.channel }}
            </NDescriptionsItem>
            <NDescriptionsItem :label="$t('notificationAdmin.colRecipient')">
              {{ detail.recipient_name || detail.recipient_id }}
            </NDescriptionsItem>
            <NDescriptionsItem :label="$t('notificationAdmin.detailDefinitionKey')">
              {{ detail.definition_key }}
            </NDescriptionsItem>
            <NDescriptionsItem :label="$t('notificationAdmin.detailLocale')">
              {{ detail.locale }}
            </NDescriptionsItem>
            <NDescriptionsItem :label="$t('notificationAdmin.colAttempts')">
              {{ detail.attempts }}/{{ detail.max_attempts }}
            </NDescriptionsItem>
            <NDescriptionsItem :label="$t('notificationAdmin.detailProvider')">
              {{ detail.provider || '-' }}
            </NDescriptionsItem>
            <NDescriptionsItem :label="$t('notificationAdmin.detailExternalId')">
              {{ detail.external_id || '-' }}
            </NDescriptionsItem>
            <NDescriptionsItem :label="$t('notificationAdmin.detailSentAt')">
              {{ detail.sent_at ? formatDateTime(detail.sent_at) : '-' }}
            </NDescriptionsItem>
            <NDescriptionsItem :label="$t('notificationAdmin.detailDeliveredAt')">
              {{ detail.delivered_at ? formatDateTime(detail.delivered_at) : '-' }}
            </NDescriptionsItem>
            <NDescriptionsItem v-if="detail.failed_at" :label="$t('notificationAdmin.colFailedAt')">
              {{ formatDateTime(detail.failed_at) }}
            </NDescriptionsItem>
            <NDescriptionsItem v-if="detail.scheduled_at" :label="$t('notificationAdmin.detailScheduledAt')">
              {{ formatDateTime(detail.scheduled_at) }}
            </NDescriptionsItem>
            <NDescriptionsItem :label="$t('notificationAdmin.colCreatedAt')">
              {{ formatDateTime(detail.created_at) }}
            </NDescriptionsItem>
          </NDescriptions>

          <!-- Error Info (only if has error) -->
          <template v-if="detail.last_error">
            <NDivider />
            <h4 class="mb-8px font-bold">{{ $t('notificationAdmin.detailError') }}</h4>
            <NTag type="error" :bordered="false" style="white-space: pre-wrap; word-break: break-all;">
              {{ detail.last_error }}
            </NTag>
          </template>

          <!-- Message Content Preview -->
          <NDivider />
          <h4 class="mb-8px font-bold">{{ $t('notificationAdmin.detailContent') }}</h4>
          <template v-if="detail.content && (detail.content.title || detail.content.summary || detail.content.body)">
            <NCard size="small" :bordered="true" style="background: var(--n-color-modal, #fafafa);">
              <div v-if="detail.content.title" class="mb-4px">
                <strong>{{ detail.content.title }}</strong>
              </div>
              <div v-if="detail.content.summary" class="mb-4px text-gray-500 text-12px">
                {{ detail.content.summary }}
              </div>
              <div v-if="detail.content.body" class="whitespace-pre-wrap">
                {{ detail.content.body }}
              </div>
            </NCard>
          </template>
          <NEmpty v-else :description="$t('notificationAdmin.noContent')" />
        </template>
      </NDrawerContent>
    </NDrawer>
  </div>
</template>
