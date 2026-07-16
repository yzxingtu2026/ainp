<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue';
import { NBadge, NButton, NPopover, NScrollbar, NSpin, NEmpty, NDivider } from 'naive-ui';
import { useRouter } from 'vue-router';
import { fetchUnreadCount, fetchNotifications, fetchMarkRead, fetchMarkAllRead } from '@/service/api';
import { formatDateTime } from '@/utils/datetime';

defineOptions({ name: 'NotificationBell' });

const router = useRouter();
const unreadCount = ref(0);
const notifications = ref<Api.Notification.NotificationItem[]>([]);
const loading = ref(false);
const showPopover = ref(false);
let pollTimer: ReturnType<typeof setInterval> | null = null;

async function loadUnreadCount() {
  const res = await fetchUnreadCount();
  if (!res.error && res.data) {
    unreadCount.value = res.data.unread_count;
  }
}

async function loadRecentNotifications() {
  loading.value = true;
  try {
    const res = await fetchNotifications({ page: 1, page_size: 8, exclude_archived: true });
    if (!res.error && res.data) {
      notifications.value = res.data.items;
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
  }
}

function handleViewAll() {
  showPopover.value = false;
  router.push('/notification/list');
}

function handleNavigate(item: Api.Notification.NotificationItem) {
  if (!item.read_at) {
    handleMarkRead(item.id);
  }
  if (item.action_json?.type === 'route' && item.action_json?.target) {
    showPopover.value = false;
    router.push(item.action_json.target);
  }
}

function startPolling() {
  loadUnreadCount();
  pollTimer = setInterval(loadUnreadCount, 30_000); // 30s polling
}

function stopPolling() {
  if (pollTimer) {
    clearInterval(pollTimer);
    pollTimer = null;
  }
}

onMounted(() => {
  startPolling();
});

onUnmounted(() => {
  stopPolling();
});

function onPopoverShow(visible: boolean) {
  showPopover.value = visible;
  if (visible) {
    loadRecentNotifications();
  }
}
</script>

<template>
  <NPopover
    trigger="click"
    placement="bottom-end"
    :style="{ width: '380px' }"
    :show-arrow="false"
    @update:show="onPopoverShow"
  >
    <template #trigger>
      <NButton quaternary circle class="notification-bell-btn">
        <template #icon>
          <NBadge :value="unreadCount" :max="99" :show="unreadCount > 0" processing>
            <SvgIcon icon="mdi:bell-outline" :style="{ fontSize: '20px' }" />
          </NBadge>
        </template>
      </NButton>
    </template>

    <div class="notification-popover">
      <div class="popover-header">
        <span class="header-title">{{ $t('notification.title') }}</span>
        <NButton
          v-if="unreadCount > 0"
          text
          type="primary"
          size="small"
          @click="handleMarkAllRead"
        >
          {{ $t('notification.markAllRead') }}
        </NButton>
      </div>

      <NDivider style="margin: 8px 0" />

      <NScrollbar style="max-height: 360px">
        <NSpin :show="loading">
          <div v-if="notifications.length === 0" class="empty-state">
            <NEmpty :description="$t('notification.empty')" />
          </div>
          <div v-else class="notification-list">
            <div
              v-for="item in notifications"
              :key="item.id"
              class="notification-item"
              :class="{ unread: !item.read_at }"
              @click="handleNavigate(item)"
            >
              <span v-if="!item.read_at" class="unread-dot"></span>
              <div class="item-icon">
                <SvgIcon v-if="item.icon" :icon="item.icon" :style="{ fontSize: '18px' }" />
                <SvgIcon v-else icon="mdi:bell-outline" :style="{ fontSize: '18px' }" />
              </div>
              <div class="item-content">
                <div class="item-title">{{ item.title }}</div>
                <div class="item-summary">{{ item.summary }}</div>
                <div class="item-time">{{ formatDateTime(item.created_at) }}</div>
              </div>
              <NButton
                v-if="!item.read_at"
                text
                size="tiny"
                type="primary"
                @click.stop="handleMarkRead(item.id)"
              >
                {{ $t('notification.markRead') }}
              </NButton>
            </div>
          </div>
        </NSpin>
      </NScrollbar>

      <NDivider style="margin: 8px 0" />
      <div class="popover-footer">
        <NButton text type="primary" @click="handleViewAll">
          {{ $t('notification.viewAll') }}
        </NButton>
      </div>
    </div>
  </NPopover>
</template>

<style scoped>
.notification-bell-btn {
  margin-right: 4px;
}

.notification-popover {
  padding: 4px 0;
}

.popover-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 4px 12px;
}

.header-title {
  font-weight: 600;
  font-size: 14px;
}

.empty-state {
  padding: 32px 0;
}

.notification-list {
  padding: 0 4px;
}

.notification-item {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 8px 8px 8px 4px;
  border-radius: 6px;
  cursor: pointer;
  transition: background-color 0.15s;
  position: relative;
}

.notification-item:hover {
  background-color: rgba(0, 0, 0, 0.04);
}

.notification-item.unread {
  background-color: rgba(24, 160, 88, 0.06);
}

.unread-dot {
  display: inline-block;
  width: 8px;
  height: 8px;
  min-width: 8px;
  border-radius: 50%;
  background-color: #18a058;
  flex-shrink: 0;
  margin-top: 12px;
  box-shadow: 0 0 0 2px rgba(24, 160, 88, 0.2);
}

.item-icon {
  flex-shrink: 0;
  width: 32px;
  height: 32px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  background-color: var(--n-color);
}

.item-content {
  flex: 1;
  min-width: 0;
}

.item-title {
  font-size: 13px;
  font-weight: 500;
  line-height: 1.3;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.item-summary {
  font-size: 12px;
  color: var(--n-text-color-3);
  line-height: 1.3;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  margin-top: 2px;
}

.item-time {
  font-size: 11px;
  color: var(--n-text-color-3);
  margin-top: 4px;
}

.popover-footer {
  display: flex;
  justify-content: center;
  padding: 4px 0;
}
</style>
