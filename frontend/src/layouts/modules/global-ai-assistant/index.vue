<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue';
import { useRouter } from 'vue-router';
import { useAiAssistantStore } from '@/store/modules/ai-assistant';
import { useRouteStore } from '@/store/modules/route';
import { useThemeStore } from '@/store/modules/theme';
import { useAiConversationController } from '@/views/ai/workspace/use-workspace';
import ConversationPanel from '@/views/ai/workspace/modules/conversation-panel.vue';
import { $t } from '@/locales';

defineOptions({ name: 'GlobalAiAssistant' });

const store = useAiAssistantStore();
const routeStore = useRouteStore();
const themeStore = useThemeStore();
const router = useRouter();
const controller = useAiConversationController();
const panel = ref<HTMLElement | null>(null);
const renameVisible = ref(false);
const renameTitle = ref('');
const renameTarget = ref<Api.AI.Conversation | null>(null);
const agentName = computed(() => controller.slot.value?.agent?.name || $t('page.aiWorkspace.assistant'));
const healthy = computed(() => Boolean(controller.slot.value?.healthy));

watch(
  () => store.visible,
  visible => {
    if (visible) void nextTick(() => panel.value?.focus());
  }
);

watch(
  () => routeStore.globalAiLauncherRoute,
  route => {
    if (!route) store.resetStore();
  }
);

function chooseConversation(item: Api.AI.Conversation) {
  void controller.selectConversation(item);
  store.show('conversation');
}

function newConversation() {
  controller.newConversation();
  store.show('conversation');
}

function openRename(item: Api.AI.Conversation) {
  renameTarget.value = item;
  renameTitle.value = item.title;
  renameVisible.value = true;
}

async function confirmRename() {
  if (!renameTarget.value || !renameTitle.value.trim()) return;
  await controller.renameConversation(renameTarget.value, renameTitle.value.trim());
  renameVisible.value = false;
}

function expandWorkspace() {
  const target = routeStore.globalAiLauncherRoute?.path || '/ai/workspace';
  store.close();
  void router.push(target);
}
</script>

<template>
  <Transition name="ai-assistant-panel">
    <aside
      v-if="store.visible"
      ref="panel"
      class="global-ai-assistant"
      tabindex="-1"
      role="complementary"
      :aria-label="$t('page.aiWorkspace.assistant')"
      :style="{ '--ai-header-height': `${themeStore.header.height}px` }"
      @keydown.esc.stop="store.close"
    >
      <header class="global-ai-assistant__header">
        <AiMascot :state="controller.mascotState.value" :size="42" />
        <div class="min-w-0 flex-1">
          <div class="truncate text-14px font-650">{{ agentName }}</div>
          <div class="flex-y-center gap-5px text-11px text-gray-500">
            <span class="global-ai-assistant__health" :class="{ 'global-ai-assistant__health--ok': healthy }"></span>
            {{ healthy ? $t('page.aiWorkspace.readyShort') : $t('page.aiWorkspace.unavailable') }}
          </div>
        </div>
        <NTooltip>
          <template #trigger>
            <button class="global-ai-assistant__tool" type="button" :aria-label="$t('page.aiWorkspace.newConversation')" @click="newConversation">
              <SvgIcon icon="mdi:plus" />
            </button>
          </template>
          {{ $t('page.aiWorkspace.newConversation') }}
        </NTooltip>
        <NTooltip>
          <template #trigger>
            <button class="global-ai-assistant__tool" type="button" :aria-label="$t('page.aiWorkspace.openConversations')" @click="store.show('history')">
              <SvgIcon icon="mdi:history" />
            </button>
          </template>
          {{ $t('page.aiWorkspace.openConversations') }}
        </NTooltip>
        <NTooltip>
          <template #trigger>
            <button class="global-ai-assistant__tool" type="button" :aria-label="$t('page.aiWorkspace.runDetails')" @click="store.show('run')">
              <SvgIcon icon="mdi:timeline-clock-outline" />
            </button>
          </template>
          {{ $t('page.aiWorkspace.runDetails') }}
        </NTooltip>
        <NTooltip>
          <template #trigger>
            <button class="global-ai-assistant__tool" type="button" :aria-label="$t('page.aiWorkspace.expandWorkspace')" @click="expandWorkspace">
              <SvgIcon icon="mdi:arrow-expand" />
            </button>
          </template>
          {{ $t('page.aiWorkspace.expandWorkspace') }}
        </NTooltip>
        <NTooltip>
          <template #trigger>
            <button class="global-ai-assistant__tool" type="button" :aria-label="$t('common.close')" @click="store.close">
              <SvgIcon icon="mdi:close" />
            </button>
          </template>
          {{ $t('common.close') }}
        </NTooltip>
      </header>

      <div v-if="store.view === 'conversation'" class="global-ai-assistant__body">
        <AiConversationSurface density="sidebar" />
      </div>

      <div v-else-if="store.view === 'history'" class="global-ai-assistant__secondary">
        <div class="global-ai-assistant__secondary-title">
          <button class="global-ai-assistant__tool" type="button" :aria-label="$t('common.back')" @click="store.show('conversation')">
            <SvgIcon icon="mdi:arrow-left" />
          </button>
          <strong>{{ $t('page.aiWorkspace.openConversations') }}</strong>
        </div>
        <ConversationPanel
          :active="controller.activeConversations.value"
          :archived="controller.archivedConversations.value"
          :current-id="controller.conversation.value?.id"
          @new="newConversation"
          @select="chooseConversation"
          @rename="openRename"
          @archive="controller.archiveConversation"
          @restore="controller.restoreConversation"
        />
      </div>

      <div v-else class="global-ai-assistant__secondary">
        <div class="global-ai-assistant__secondary-title">
          <button class="global-ai-assistant__tool" type="button" :aria-label="$t('common.back')" @click="store.show('conversation')">
            <SvgIcon icon="mdi:arrow-left" />
          </button>
          <strong>{{ $t('page.aiWorkspace.runDetails') }}</strong>
        </div>
        <div class="global-ai-assistant__run">
          <NEmpty v-if="!controller.runDetail.value" :description="$t('page.aiWorkspace.noRun')" />
          <template v-else>
            <NDescriptions :column="1" bordered size="small">
              <NDescriptionsItem :label="$t('page.aiWorkspace.status')">
                <AgentRunStatus :run="controller.runDetail.value.run" />
              </NDescriptionsItem>
              <NDescriptionsItem :label="$t('page.aiWorkspace.steps')">{{ controller.runDetail.value.run.current_step }}</NDescriptionsItem>
              <NDescriptionsItem :label="$t('page.aiWorkspace.modelCalls')">{{ controller.runDetail.value.run.model_calls }}</NDescriptionsItem>
              <NDescriptionsItem :label="$t('page.aiWorkspace.toolCalls')">{{ controller.runDetail.value.run.tool_calls }}</NDescriptionsItem>
            </NDescriptions>
            <NTimeline class="mt-18px">
              <NTimelineItem
                v-for="step in controller.runDetail.value.steps"
                :key="step.id"
                :type="step.status === 'failed' ? 'error' : step.status === 'completed' ? 'success' : 'info'"
                :title="step.type"
                :content="step.status"
                :time="step.started_at || step.created_at"
              />
            </NTimeline>
          </template>
        </div>
      </div>

      <NModal
        v-model:show="renameVisible"
        preset="dialog"
        :title="$t('page.aiWorkspace.renameTitle')"
        :positive-text="$t('page.aiWorkspace.save')"
        :negative-text="$t('common.cancel')"
        @positive-click="confirmRename"
      >
        <NInput v-model:value="renameTitle" maxlength="256" @keyup.enter="confirmRename" />
      </NModal>
    </aside>
  </Transition>
</template>

<style scoped>
.global-ai-assistant {
  position: fixed;
  z-index: 3000;
  top: var(--ai-header-height, 56px);
  right: 0;
  bottom: 0;
  width: clamp(400px, 36vw, min(640px, 48vw));
  min-width: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  border-left: 1px solid rgba(127, 127, 127, 0.18);
  background: rgb(var(--container-color));
  box-shadow: -10px 0 28px rgba(25, 31, 46, 0.14);
  outline: none;
}
.global-ai-assistant__header {
  min-height: 58px;
  flex: none;
  display: flex;
  align-items: center;
  gap: 6px;
  border-bottom: 1px solid rgba(127, 127, 127, 0.15);
  padding: 7px 10px;
}
.global-ai-assistant__health {
  width: 7px;
  height: 7px;
  flex: none;
  border-radius: 50%;
  background: #ef4444;
}
.global-ai-assistant__health--ok {
  background: #22a06b;
}
.global-ai-assistant__tool {
  width: 34px;
  height: 34px;
  flex: none;
  display: grid;
  place-items: center;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: inherit;
  cursor: pointer;
}
.global-ai-assistant__tool:hover,
.global-ai-assistant__tool:focus-visible {
  background: rgba(127, 127, 127, 0.1);
  outline: 2px solid rgba(var(--primary-color), 0.4);
}
.global-ai-assistant__body,
.global-ai-assistant__secondary {
  min-height: 0;
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
.global-ai-assistant__secondary-title {
  min-height: 48px;
  flex: none;
  display: flex;
  align-items: center;
  gap: 8px;
  border-bottom: 1px solid rgba(127, 127, 127, 0.13);
  padding: 6px 12px;
}
.global-ai-assistant__run {
  min-height: 0;
  flex: 1;
  overflow-x: hidden;
  overflow-y: auto;
  padding: 16px;
}
.ai-assistant-panel-enter-active,
.ai-assistant-panel-leave-active {
  transition: transform 180ms ease, opacity 180ms ease;
}
.ai-assistant-panel-enter-from,
.ai-assistant-panel-leave-to {
  transform: translateX(24px);
  opacity: 0;
}
@media (max-width: 840px) {
  .global-ai-assistant {
    top: 0;
    width: 100vw;
    max-width: none;
  }
}
@media (prefers-reduced-motion: reduce) {
  .ai-assistant-panel-enter-active,
  .ai-assistant-panel-leave-active {
    transition: none;
  }
}
</style>
