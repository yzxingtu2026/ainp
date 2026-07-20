<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import { useRouter } from 'vue-router';
import { useAppStore } from '@/store/modules/app';
import { useAuthStore } from '@/store/modules/auth';
import { $t } from '@/locales';
import ConversationPanel from './modules/conversation-panel.vue';
import WorkspaceComposer from './modules/workspace-composer.vue';
import { useWorkspace } from './use-workspace';

defineOptions({ name: 'AIWorkspace' });

const appStore = useAppStore();
const authStore = useAuthStore();
const router = useRouter();
const workspace = useWorkspace();

const conversationsVisible = ref(false);
const conversationPanelCollapsed = ref(true);
const detailsVisible = ref(false);
const renameVisible = ref(false);
const renameTitle = ref('');
const renameTarget = ref<Api.AI.Conversation | null>(null);

const runActive = computed(() => workspace.status.value === 'running' || workspace.status.value === 'reconnecting');
const aiThinking = computed(() => runActive.value && !workspace.streamingText.value);
const agentName = computed(() => workspace.slot.value?.agent?.name || $t('page.aiWorkspace.assistant'));
const greetingName = computed(() => authStore.userInfo.userName || $t('page.aiWorkspace.assistant'));
const suggestions = computed(() => [
  $t('page.aiWorkspace.suggestionPriority'),
  $t('page.aiWorkspace.suggestionExceptions'),
  $t('page.aiWorkspace.suggestionFeature'),
  $t('page.aiWorkspace.suggestionNext')
]);

function chooseConversation(item: Api.AI.Conversation) {
  void workspace.selectConversation(item);
  conversationsVisible.value = false;
}

function openRename(item: Api.AI.Conversation) {
  renameTarget.value = item;
  renameTitle.value = item.title;
  renameVisible.value = true;
}

async function confirmRename() {
  if (!renameTarget.value || !renameTitle.value.trim()) return;
  await workspace.renameConversation(renameTarget.value, renameTitle.value.trim());
  renameVisible.value = false;
}

function useSuggestion(value: string) {
  workspace.input.value = value;
}

function setMessageListElement(element: unknown) {
  workspace.messageListEl.value = element instanceof HTMLElement ? element : null;
}

onMounted(() => void workspace.initialize());
</script>

<template>
  <div class="ai-workspace">
    <aside
      v-if="!appStore.isMobile"
      class="ai-workspace__sidebar"
      :class="{ 'ai-workspace__sidebar--collapsed': conversationPanelCollapsed }"
    >
      <div class="ai-workspace__sidebar-body">
        <ConversationPanel
          :active="workspace.activeConversations.value"
          :archived="workspace.archivedConversations.value"
          :current-id="workspace.conversation.value?.id"
          @new="workspace.newConversation"
          @select="chooseConversation"
          @rename="openRename"
          @archive="workspace.archiveConversation"
          @restore="workspace.restoreConversation"
        />
      </div>
    </aside>

    <main class="ai-workspace__main">
      <button
        v-if="!appStore.isMobile"
        type="button"
        class="ai-workspace__collapse-handle"
        :title="conversationPanelCollapsed ? $t('page.aiWorkspace.openConversations') : $t('page.aiWorkspace.collapseConversations')"
        @click="conversationPanelCollapsed = !conversationPanelCollapsed"
      >
        <SvgIcon :icon="conversationPanelCollapsed ? 'mdi:menu-close' : 'mdi:menu-open'" />
      </button>
      <header class="ai-workspace__header">
        <button
          v-if="appStore.isMobile"
          type="button"
          class="ai-workspace__header-button"
          :title="$t('page.aiWorkspace.openConversations')"
          @click="conversationsVisible = true"
        >
          <SvgIcon icon="mdi:menu" />
        </button>
        <div class="min-w-0 flex-1">
          <div class="truncate text-14px font-600">
            {{ workspace.conversation.value?.title || $t('page.aiWorkspace.newChat') }}
          </div>
          <div class="truncate text-11px text-gray-500">
            {{ agentName }} · {{ workspace.slot.value?.source || 'default' }}
          </div>
        </div>
        <NTooltip>
          <template #trigger>
            <button
              type="button"
              class="ai-workspace__header-button"
              :title="$t('page.aiWorkspace.runDetails')"
              @click="detailsVisible = true"
            >
              <SvgIcon icon="mdi:timeline-clock-outline" />
            </button>
          </template>
          {{ $t('page.aiWorkspace.runDetails') }}
        </NTooltip>
        <NTooltip>
          <template #trigger>
            <button
              type="button"
              class="ai-workspace__header-button"
              :title="$t('page.aiWorkspace.memory')"
              @click="router.push('/profile/memory')"
            >
              <SvgIcon icon="mdi:brain" />
            </button>
          </template>
          {{ $t('page.aiWorkspace.memory') }}
        </NTooltip>
      </header>

      <section :ref="setMessageListElement" class="ai-workspace__messages">
        <div v-if="workspace.status.value === 'loading'" class="h-full flex-center">
          <NSpin size="large" />
        </div>

        <div v-else-if="!workspace.conversation.value && workspace.messages.value.length === 0" class="welcome">
          <AiMascot :state="workspace.mascotState.value" :size="132" />
          <h1>{{ $t('page.aiWorkspace.greeting', { name: greetingName }) }}</h1>
          <p v-if="workspace.slot.value?.healthy">
            {{ $t('page.aiWorkspace.ready', { agent: agentName }) }}
          </p>
          <NAlert v-else type="error" class="max-w-560px" :show-icon="true">
            {{ workspace.error.value || $t('page.aiWorkspace.unavailable') }}
          </NAlert>
          <div class="welcome__suggestions">
            <button
              v-for="suggestion in suggestions"
              :key="suggestion"
              type="button"
              @click="useSuggestion(suggestion)"
            >
              {{ suggestion }}
              <SvgIcon icon="mdi:arrow-up-right" />
            </button>
          </div>
        </div>

        <ChatMessageList
          v-else
          :messages="workspace.messages.value"
          :streaming-text="workspace.streamingText.value"
          :thinking="aiThinking"
          :run-active="runActive"
          :confirmations="workspace.confirmations.value"
          :error="workspace.error.value"
          @confirmation-resolved="workspace.onConfirmationResolved"
        />
      </section>

      <footer class="ai-workspace__composer">
        <div v-if="workspace.status.value === 'reconnecting'" class="mb-6px text-center text-12px text-warning">
          {{ $t('page.aiWorkspace.reconnecting') }}
        </div>
        <WorkspaceComposer
          v-model="workspace.input.value"
          v-model:images="workspace.images.value"
          :disabled="!workspace.slot.value?.healthy"
          :can-send="workspace.canSend.value"
          :active="runActive"
          @send="workspace.send"
          @stop="workspace.stop"
        />
        <div class="mt-7px text-center text-11px text-gray-400">
          {{ $t('page.aiWorkspace.disclaimer') }}
        </div>
      </footer>
    </main>

    <NDrawer v-model:show="conversationsVisible" placement="left" width="min(88vw, 320px)">
      <NDrawerContent body-content-style="padding: 0;">
        <ConversationPanel
          :active="workspace.activeConversations.value"
          :archived="workspace.archivedConversations.value"
          :current-id="workspace.conversation.value?.id"
          @new="
            workspace.newConversation();
            conversationsVisible = false;
          "
          @select="chooseConversation"
          @rename="openRename"
          @archive="workspace.archiveConversation"
          @restore="workspace.restoreConversation"
        />
      </NDrawerContent>
    </NDrawer>

    <NDrawer v-model:show="detailsVisible" placement="right" width="min(92vw, 440px)">
      <NDrawerContent :title="$t('page.aiWorkspace.runDetails')" closable>
        <NEmpty v-if="!workspace.runDetail.value" :description="$t('page.aiWorkspace.noRun')" />
        <template v-else>
          <NDescriptions :column="1" bordered size="small">
            <NDescriptionsItem :label="$t('page.aiWorkspace.status')">
              <AgentRunStatus :run="workspace.runDetail.value.run" />
            </NDescriptionsItem>
            <NDescriptionsItem :label="$t('page.aiWorkspace.steps')">
              {{ workspace.runDetail.value.run.current_step }}
            </NDescriptionsItem>
            <NDescriptionsItem :label="$t('page.aiWorkspace.modelCalls')">
              {{ workspace.runDetail.value.run.model_calls }}
            </NDescriptionsItem>
            <NDescriptionsItem :label="$t('page.aiWorkspace.toolCalls')">
              {{ workspace.runDetail.value.run.tool_calls }}
            </NDescriptionsItem>
          </NDescriptions>
          <NTimeline class="mt-20px">
            <NTimelineItem
              v-for="step in workspace.runDetail.value.steps"
              :key="step.id"
              :type="step.status === 'failed' ? 'error' : step.status === 'completed' ? 'success' : 'info'"
              :title="step.type"
              :content="step.status"
              :time="step.started_at || step.created_at"
            />
          </NTimeline>
        </template>
      </NDrawerContent>
    </NDrawer>

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
  </div>
</template>

<style scoped>
.ai-workspace {
  height: 100%;
  min-height: 0;
  display: flex;
  overflow: hidden;
  background: rgb(var(--layout-bg-color));
}

.ai-workspace__sidebar {
  width: clamp(260px, 20vw, 300px);
  flex: none;
  overflow: hidden;
  border-right: 1px solid rgba(127, 127, 127, 0.15);
  background: rgb(var(--container-color));
  visibility: visible;
  transition:
    width 0.3s cubic-bezier(0.4, 0, 0.2, 1),
    visibility 0s linear 0s;
}

.ai-workspace__sidebar--collapsed {
  width: 0;
  visibility: hidden;
  transition:
    width 0.3s cubic-bezier(0.4, 0, 0.2, 1),
    visibility 0s linear 0.3s;
}

.ai-workspace__sidebar-body {
  width: clamp(260px, 20vw, 300px);
  height: 100%;
}

.ai-workspace__main {
  position: relative;
  min-width: 0;
  min-height: 0;
  flex: 1;
  display: flex;
  flex-direction: column;
}

.ai-workspace__header {
  height: 58px;
  flex: none;
  display: flex;
  align-items: center;
  gap: 8px;
  border-bottom: 1px solid rgba(127, 127, 127, 0.13);
  padding: 0 16px;
  background: rgba(var(--container-color), 0.94);
}

.ai-workspace__header-button {
  width: 38px;
  height: 38px;
  flex: none;
  display: grid;
  place-items: center;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: inherit;
  font-size: 20px;
  cursor: pointer;
}

.ai-workspace__header-button:hover {
  background: rgba(127, 127, 127, 0.1);
}

.ai-workspace__collapse-handle {
  position: absolute;
  top: 50%;
  left: 0;
  z-index: 20;
  width: 24px;
  height: 72px;
  display: grid;
  place-items: center;
  border: 1px solid rgba(127, 127, 127, 0.18);
  border-left: 0;
  border-radius: 0 14px 14px 0;
  background: rgb(var(--container-color));
  color: rgba(127, 127, 127, 0.85);
  font-size: 18px;
  cursor: pointer;
  transform: translateY(-50%);
  box-shadow: 2px 0 12px rgba(25, 31, 46, 0.1);
  transition:
    color 0.2s ease,
    border-color 0.2s ease,
    background 0.2s ease,
    width 0.2s ease;
}
.ai-workspace__collapse-handle:hover {
  width: 28px;
  border-color: rgba(var(--primary-color), 0.5);
  background: rgba(var(--primary-color), 0.07);
  color: rgb(var(--primary-color));
}
.ai-workspace__messages {
  min-height: 0;
  flex: 1;
  overflow-y: auto;
  overscroll-behavior: contain;
}
.ai-workspace__composer {
  flex: none;
  padding: 12px 20px 10px;
}

.welcome {
  min-height: 100%;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 10px;
  padding: 36px 24px;
}

.welcome h1 {
  margin: 0;
  font-size: clamp(22px, 3vw, 32px);
  font-weight: 650;
  letter-spacing: 0;
  text-align: center;
}
.welcome p {
  margin: 0;
  color: rgba(127, 127, 127, 0.9);
}
.welcome__suggestions {
  width: min(680px, 100%);
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
  margin-top: 22px;
}
.welcome__suggestions button {
  min-height: 54px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  border: 1px solid rgba(127, 127, 127, 0.18);
  border-radius: 6px;
  background: rgba(var(--container-color), 0.72);
  padding: 10px 14px;
  color: inherit;
  text-align: left;
  cursor: pointer;
}
.welcome__suggestions button:hover {
  border-color: rgba(var(--primary-color), 0.45);
  background: rgba(var(--primary-color), 0.06);
}

@media (max-width: 640px) {
  .ai-workspace__composer {
    padding: 8px 10px calc(8px + env(safe-area-inset-bottom));
  }
  .welcome {
    justify-content: flex-start;
    padding-top: 54px;
  }
  .welcome__suggestions {
    grid-template-columns: 1fr;
  }
}
</style>
