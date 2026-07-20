<script setup lang="ts">
import { computed, ref } from 'vue';
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
          {{ $t('page.aiWorkspace.recent') }}
        </button>
        <button type="button" :class="{ active: showArchived }" @click="showArchived = true">
          {{ $t('page.aiWorkspace.archived') }}
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
          <span class="conversation-item__time">{{ item.updated_at.slice(0, 10) }}</span>
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
  background: rgb(var(--container-color));
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
  border-radius: 6px;
  background: rgba(127, 127, 127, 0.08);
  padding: 3px;
}
.conversation-panel__tabs button {
  height: 30px;
  border: 0;
  border-radius: 4px;
  background: transparent;
  color: inherit;
  cursor: pointer;
}
.conversation-panel__tabs button.active {
  background: rgb(var(--container-color));
  box-shadow: 0 1px 4px rgba(25, 31, 46, 0.1);
  font-weight: 600;
}
.conversation-panel__list {
  min-height: 0;
  flex: 1;
  overflow-y: auto;
  padding: 4px 8px 16px;
}
.conversation-item {
  width: 100%;
  min-height: 58px;
  display: flex;
  align-items: center;
  gap: 8px;
  border: 0;
  border-radius: 6px;
  background: transparent;
  padding: 7px 6px 7px 10px;
  color: inherit;
  text-align: left;
  cursor: pointer;
}
.conversation-item:hover {
  background: rgba(127, 127, 127, 0.08);
}
.conversation-item--active {
  background: rgba(var(--primary-color), 0.11);
  color: rgb(var(--primary-color));
}
.conversation-item__icon {
  flex: none;
  font-size: 18px;
}
.conversation-item__body {
  min-width: 0;
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 3px;
}
.conversation-item__title {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 13px;
  font-weight: 550;
}
.conversation-item__time {
  color: rgba(127, 127, 127, 0.82);
  font-size: 10px;
}
.conversation-item__more {
  width: 30px;
  height: 30px;
  flex: none;
  display: grid;
  place-items: center;
  border-radius: 4px;
  opacity: 0;
}
.conversation-item:hover .conversation-item__more,
.conversation-item__more:focus {
  opacity: 1;
}
</style>
