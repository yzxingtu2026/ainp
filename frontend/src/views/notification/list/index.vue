<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue';
import { NButton, NCard, NEmpty, NPagination, NSelect, NSpace, NSpin, NTag } from 'naive-ui';
import { useRouter } from 'vue-router';
import { $t } from '@/locales';
import {
  fetchArchiveNotification,
  fetchMarkAllRead,
  fetchMarkRead,
  fetchNotifications,
  fetchUnarchiveNotification,
  fetchUnreadCount
} from '@/service/api';
import { formatDateTime } from '@/utils/datetime';

defineOptions({ name: 'NotificationList' });

const router = useRouter();
const loading = ref(false);
const notifications = ref<Api.Notification.NotificationItem[]>([]);
const total = ref(0);
const unreadCount = ref(0);
const query = reactive({
  current: 1,
  size: 20,
  unread_only: false,
  exclude_archived: true
});

async function loadList() {
  loading.value = true;
  try {
    const [listRes, countRes] = await Promise.all([
      fetchNotifications(query),
      fetchUnreadCount()
    ]);
    if (!listRes.error && listRes.data) {
      notifications.value = listRes.data.items;
      total.value = listRes.data.total;
    }
    if (!countRes.error && countRes.data) {
      unreadCount.value = countRes.data.unread_count;
    }
  } finally {
    loading.value = false;
  }
}

async function handleMarkRead(id: string) {
  const res = await fetchMarkRead(id);
  if (!res.error) {
    const item = notifications.value.find(n => n.id === id);
    if (item) item.read_at = res.data?.read_at || new Date().toISOString();
    if (unreadCount.value > 0) unreadCount.value--;
  }
}

async function handleMarkAllRead() {
  const res = await fetchMarkAllRead();
  if (!res.error) {
    notifications.value.forEach(n => {
      if (!n.read_at) n.read_at = new Date().toISOString();
    });
    unreadCount.value = 0;
    window.$message?.success($t('notification.markAllReadSuccess'));
  }
}

async function handleArchive(id: string) {
  const res = await fetchArchiveNotification(id);
  if (!res.error) {
    window.$message?.success($t('notification.archiveSuccess'));
    await loadList();
  }
}

async function handleUnarchive(id: string) {
  const res = await fetchUnarchiveNotification(id);
  if (!res.error) {
    window.$message?.success($t('notification.unarchiveSuccess'));
    await loadList();
  }
}

function handleNavigate(item: Api.Notification.NotificationItem) {
  if (!item.read_at) {
    handleMarkRead(item.id);
  }
  if (item.action_json?.type === 'route' && item.action_json?.target) {
    router.push(item.action_json.target);
  }
}

function resetQuery() {
  query.current = 1;
  query.unread_only = false;
  query.exclude_archived = true;
  loadList();
}

const filterOptions = computed(() => [
  { label: $t('notification.filterAll'), value: 'all' },
  { label: $t('notification.filterUnread'), value: 'unread' },
  { label: $t('notification.filterArchived'), value: 'archived' }
]);

const currentFilter = computed({
  get() {
    if (query.unread_only) return 'unread';
    if (!query.exclude_archived) return 'archived';
    return 'all';
  },
  set(val: string) {
    query.current = 1;
    if (val === 'unread') {
      query.unread_only = true;
      query.exclude_archived = true;
    } else if (val === 'archived') {
      query.unread_only = false;
      query.exclude_archived = false;
    } else {
      query.unread_only = false;
      query.exclude_archived = true;
    }
    loadList();
  }
});

onMounted(loadList);
</script>

<template>
  <div class="min-h-500px flex-col-stretch gap-16px overflow-hidden lt-sm:overflow-auto">
    <NCard :title="$t('notification.pageTitle')" :bordered="false" class="card-wrapper">
      <template #header-extra>
        <NSpace align="center">
          <NTag v-if="unreadCount > 0" type="warning" round size="small">
            {{ $t('notification.unreadBadge', { count: unreadCount }) }}
          </NTag>
          <NButton
            v-if="unreadCount > 0"
            type="primary"
            size="small"
            ghost
            @click="handleMarkAllRead"
          >
            {{ $t('notification.markAllRead') }}
          </NButton>
        </NSpace>
      </template>

      <NSpace class="mb-16px" align="center">
        <NSelect
          v-model:value="currentFilter"
          class="w-160px"
          :options="filterOptions"
          :placeholder="$t('notification.filterPlaceholder')"
        />
        <NButton type="primary" @click="query.current = 1; loadList()">
          {{ $t('common.refresh') }}
        </NButton>
        <NButton @click="resetQuery">{{ $t('common.reset') }}</NButton>
      </NSpace>

      <NSpin :show="loading">
        <div v-if="notifications.length === 0" class="py-48px">
          <NEmpty :description="$t('notification.empty')" />
        </div>
        <div v-else class="notification-list">
          <div
            v-for="item in notifications"
            :key="item.id"
            class="notification-item"
            :class="{ unread: !item.read_at, archived: item.archived_at }"
          >
            <div class="item-icon" @click="handleNavigate(item)">
              <SvgIcon v-if="item.icon" :icon="item.icon" :style="{ fontSize: '20px' }" />
              <SvgIcon v-else icon="mdi:bell-outline" :style="{ fontSize: '20px' }" />
            </div>
            <div class="item-content" @click="handleNavigate(item)">
              <div class="item-title">
                <span v-if="!item.read_at" class="unread-dot"></span>
                {{ item.title }}
              </div>
              <div class="item-summary">{{ item.summary }}</div>
              <div class="item-meta">
                <span class="item-time">{{ formatDateTime(item.created_at) }}</span>
                <NTag v-if="item.archived_at" size="tiny" type="default" :bordered="false">
                  {{ $t('notification.archived') }}
                </NTag>
              </div>
            </div>
            <div class="item-actions">
              <NButton
                v-if="!item.read_at"
                text
                size="small"
                type="primary"
                @click="handleMarkRead(item.id)"
              >
                {{ $t('notification.markRead') }}
              </NButton>
              <NButton
                v-if="!item.archived_at"
                text
                size="small"
                type="warning"
                @click="handleArchive(item.id)"
              >
                {{ $t('notification.archive') }}
              </NButton>
              <NButton
                v-else
                text
                size="small"
                type="info"
                @click="handleUnarchive(item.id)"
              >
                {{ $t('notification.unarchive') }}
              </NButton>
            </div>
          </div>
        </div>
      </NSpin>

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

<style scoped>
.notification-list {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.notification-item {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  padding: 12px 16px;
  border-radius: 8px;
  transition: background-color 0.15s;
}

.notification-item:hover {
  background-color: var(--n-color-hover);
}

.notification-item.unread {
  background-color: rgba(24, 160, 88, 0.06);
}

.notification-item.archived {
  opacity: 0.65;
}

.item-icon {
  flex-shrink: 0;
  width: 40px;
  height: 40px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  background-color: rgba(24, 160, 88, 0.1);
  cursor: pointer;
}

.item-content {
  flex: 1;
  min-width: 0;
  cursor: pointer;
}

.item-title {
  font-size: 14px;
  font-weight: 500;
  line-height: 1.4;
  display: flex;
  align-items: center;
  gap: 6px;
}

.unread-dot {
  display: inline-block;
  width: 8px;
  height: 8px;
  min-width: 8px;
  border-radius: 50%;
  background-color: #18a058;
  flex-shrink: 0;
  box-shadow: 0 0 0 2px rgba(24, 160, 88, 0.2);
}

.item-summary {
  font-size: 13px;
  color: var(--n-text-color-3);
  line-height: 1.4;
  margin-top: 4px;
  overflow: hidden;
  text-overflow: ellipsis;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
}

.item-meta {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 6px;
}

.item-time {
  font-size: 12px;
  color: var(--n-text-color-3);
}

.item-actions {
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 4px;
}
</style>
