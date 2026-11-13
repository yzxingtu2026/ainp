<script setup lang="ts">
import { computed, ref } from 'vue';
import DateTimeText from '@/components/common/datetime-text.vue';
import { $t } from '@/locales';

defineOptions({ name: 'WorkspaceConversationPanel' });

const props = defineProps<{
  active: Api.AI.Conversation[];
  archived: Api.AI.Conversation[];
  currentId?: string;
}>();
const emit = defineEmits<{
  new: [];
  select: [conversation: Api.AI.Conversation];
  rename: [conversation: Api.AI.Conversation];
  archive: [conversation: Api.AI.Conversation];
  restore: [conversation: Api.AI.Conversation];
}>();

const search = ref('');
const showArchived = ref(false);
const visible = computed(() => {
  const source = showArchived.value ? props.archived : props.active;
  const keyword = search.value.trim().toLowerCase();
  return keyword ? source.filter(item => item.title.toLowerCase().includes(keyword)) : source;
});
</script>

<template>
  <div class="conversation-panel">
    <div class="conversation-panel__top">
      <NButton type="primary" block @click="emit('new')">
        <template #icon><SvgIcon icon="mdi:plus" /></template>
        {{ $t('page.aiWorkspace.newConversation') }}
      </NButton>
      <NInput v-model:value="search" clearable :placeholder="$t('page.aiWorkspace.searchConversations')">
        <template #prefix><SvgIcon icon="mdi:magnify" /></template>
      </NInput>
      <div class="conversation-panel__tabs">
        <button type="button" :class="{ active: !showArchived }" @click="showArchived = false">
          <span>{{ $t('page.aiWorkspace.recent') }}</span>
          <span v-if="active.length > 0" class="conversation-panel__count">{{ active.length }}</span>
        </button>
        <button type="button" :class="{ active: showArchived }" @click="showArchived = true">
          <span>{{ $t('page.aiWorkspace.archived') }}</span>
          <span v-if="archived.length > 0" class="conversation-panel__count">{{ archived.length }}</span>
        </button>
      </div>
    </div>

    <div class="conversation-panel__list">
      <NEmpty
        v-if="visible.length === 0"
        size="small"
        :description="showArchived ? $t('page.aiWorkspace.emptyArchived') : $t('page.aiWorkspace.emptyRecent')"
      />
      <button
        v-for="item in visible"
        :key="item.id"
        type="button"
        class="conversation-item"
        :class="{ 'conversation-item--active': currentId === item.id }"
        @click="emit('select', item)"
      >
        <span class="conversation-item__icon"><SvgIcon icon="mdi:message-text-outline" /></span>
        <span class="conversation-item__body">
          <span class="conversation-item__title" :title="item.title">{{ item.title }}</span>
          <DateTimeText :value="item.last_message_at || item.updated_at" class="conversation-item__time" />
        </span>
        <NDropdown
          trigger="click"
          :options="
            showArchived
              ? [{ label: $t('page.aiWorkspace.restore'), key: 'restore' }]
              : [
                { label: $t('page.aiWorkspace.rename'), key: 'rename' },
                { label: $t('page.aiWorkspace.archive'), key: 'archive' }
              ]
          "
          @select="
            key =>
              key === 'restore'
                ? emit('restore', item)
                : key === 'rename'
                  ? emit('rename', item)
                  : emit('archive', item)
          "
        >
          <span class="conversation-item__more" :title="$t('page.aiWorkspace.actions')" @click.stop>
            <SvgIcon icon="mdi:dots-horizontal" />
          </span>
        </NDropdown>
      </button>
    </div>
  </div>
</template>

<style scoped>
.conversation-panel {
  height: 100%;
  min-height: 0;
  display: flex;
  flex-direction: column;
  background: rgb(var(--container-bg-color));
}

.conversation-panel__top {
  flex: none;
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 14px 12px 10px;
}

.conversation-panel__tabs {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 4px;
  border: 1px solid rgba(127, 127, 127, 0.14);
  border-radius: 8px;
  background: rgba(127, 127, 127, 0.06);
  padding: 3px;
}

.conversation-panel__tabs button {
  height: 30px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 5px;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: rgba(var(--base-text-color), 0.78);
  font-size: 12.5px;
  cursor: pointer;
  transition:
    background 0.16s ease,
    color 0.16s ease;
}

.conversation-panel__tabs button.active {
  background: rgb(var(--container-bg-color));
  color: rgb(var(--base-text-color));
  box-shadow: 0 1px 4px rgba(25, 31, 46, 0.1);
  font-weight: 600;
}

.conversation-panel__count {
  min-width: 18px;
  height: 16px;
  display: inline-grid;
  place-items: center;
  border-radius: 999px;
  background: rgba(127, 127, 127, 0.14);
  padding: 0 5px;
  font-size: 10.5px;
  font-variant-numeric: tabular-nums;
}

.conversation-panel__tabs button.active .conversation-panel__count {
  background: rgba(var(--primary-color), 0.14);
  color: rgb(var(--primary-color));
}

.conversation-panel__list {
  min-height: 0;
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 2px;
  overflow-y: auto;
  padding: 4px 8px 16px;
}

.conversation-item {
  position: relative;
  width: 100%;
  min-height: 56px;
  display: flex;
  align-items: center;
  gap: 9px;
  border: 1px solid transparent;
  border-radius: 9px;
  background: transparent;
  padding: 7px 6px 7px 10px;
  color: inherit;
  text-align: left;
  cursor: pointer;
  transition:
    border-color 0.16s ease,
    background 0.16s ease;
}

.conversation-item:hover {
  background: rgba(127, 127, 127, 0.065);
}

.conversation-item--active {
  border-color: rgba(var(--primary-color), 0.28);
  background: linear-gradient(90deg, rgba(var(--primary-color), 0.12) 0%, rgba(var(--primary-color), 0.05) 100%);
  color: rgb(var(--primary-color));
}

.conversation-item__icon {
  width: 28px;
  height: 28px;
  flex: none;
  display: grid;
  place-items: center;
  border-radius: 7px;
  background: rgba(127, 127, 127, 0.08);
  font-size: 15px;
}

.conversation-item--active .conversation-item__icon {
  background: rgba(var(--primary-color), 0.15);
}

.conversation-item__body {
  min-width: 0;
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.conversation-item__title {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 13px;
  font-weight: 560;
}

.conversation-item__time {
  color: rgba(127, 127, 127, 0.82);
  font-size: 11px;
  font-variant-numeric: tabular-nums;
}

.conversation-item__more {
  width: 28px;
  height: 28px;
  flex: none;
  display: grid;
  place-items: center;
  border-radius: 6px;
  opacity: 0;
  transition:
    opacity 0.16s ease,
    background 0.16s ease;
}

.conversation-item__more:hover {
  background: rgba(127, 127, 127, 0.14);
}

.conversation-item:hover .conversation-item__more,
.conversation-item--active .conversation-item__more,
.conversation-item__more:focus {
  opacity: 1;
}

@media (hover: none) {
  .conversation-item__more {
    opacity: 0.85;
  }
}

@media (prefers-reduced-motion: reduce) {
  .conversation-panel__tabs button,
  .conversation-item,
  .conversation-item__more {
    transition: none;
  }
}
</style>
