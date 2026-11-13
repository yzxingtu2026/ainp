<script setup lang="ts">
import { computed, ref } from 'vue';
import { useRouter } from 'vue-router';
import CodeTag from '@/components/common/code-tag.vue';
import DateTimeText from '@/components/common/datetime-text.vue';
import { CODESET_KEYS } from '@/constants/codeset';
import { useAppStore } from '@/store/modules/app';
import { $t } from '@/locales';
import ConversationPanel from './modules/conversation-panel.vue';
import { useWorkspace } from './use-workspace';

defineOptions({ name: 'AIWorkspace' });

const appStore = useAppStore();
const router = useRouter();
const workspace = useWorkspace();

const conversationsVisible = ref(false);
const conversationPanelCollapsed = ref(true);
const detailsVisible = ref(false);
const renameVisible = ref(false);
const renameTitle = ref('');
const renameTarget = ref<Api.AI.Conversation | null>(null);

const agentName = computed(() => workspace.slot.value?.agent?.name || $t('page.aiWorkspace.assistant'));
const healthy = computed(() => Boolean(workspace.slot.value?.healthy));

function slotSourceLabel(source?: Api.AI.AgentSlotResolution['source']) {
  if (source === 'user') return $t('page.aiAgents.source.personal');
  if (source === 'tenant') return $t('page.aiAgents.source.tenant');
  if (source === 'builtin') return $t('page.aiAgents.source.builtin');
  return '';
}

function stepTitle(type: string) {
  if (type === 'model') return $t('chat.stepModel');
  if (type === 'tool_search') return $t('chat.stepToolSearch');
  if (type === 'tool_load') return $t('chat.stepToolLoad');
  if (type === 'tool_execute') return $t('chat.stepToolExecute');
  if (type === 'context_assemble') return $t('chat.stepContextAssemble');
  if (type === 'finalize') return $t('chat.stepFinalize');
  return type;
}

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
        :title="
          conversationPanelCollapsed
            ? $t('page.aiWorkspace.openConversations')
            : $t('page.aiWorkspace.collapseConversations')
        "
        :aria-label="
          conversationPanelCollapsed
            ? $t('page.aiWorkspace.openConversations')
            : $t('page.aiWorkspace.collapseConversations')
        "
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
          :aria-label="$t('page.aiWorkspace.openConversations')"
          @click="conversationsVisible = true"
        >
          <SvgIcon icon="mdi:menu" />
        </button>

        <div class="ai-workspace__title-group">
          <div class="truncate text-14px font-620">
            {{ workspace.conversation.value?.title || $t('page.aiWorkspace.newChat') }}
          </div>
          <div class="ai-workspace__subtitle">
            <span class="ai-workspace__health" :class="{ 'ai-workspace__health--ok': healthy }" aria-hidden="true" />
            <span class="truncate">{{ agentName }}</span>
            <span v-if="workspace.slot.value?.source" class="ai-workspace__source-tag">
              {{ slotSourceLabel(workspace.slot.value.source) }}
            </span>
          </div>
        </div>

        <NTooltip>
          <template #trigger>
            <button
              type="button"
              class="ai-workspace__header-button"
              :title="$t('page.aiWorkspace.newConversation')"
              :aria-label="$t('page.aiWorkspace.newConversation')"
              @click="workspace.newConversation"
            >
              <SvgIcon icon="mdi:plus" />
            </button>
          </template>
          {{ $t('page.aiWorkspace.newConversation') }}
        </NTooltip>

        <NTooltip>
          <template #trigger>
            <button
              type="button"
              class="ai-workspace__header-button"
              :class="{ 'ai-workspace__header-button--active': detailsVisible }"
              :title="$t('page.aiWorkspace.runDetails')"
              :aria-label="$t('page.aiWorkspace.runDetails')"
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
              :aria-label="$t('page.aiWorkspace.memory')"
              @click="router.push('/profile/memory')"
            >
              <SvgIcon icon="mdi:brain" />
            </button>
          </template>
          {{ $t('page.aiWorkspace.memory') }}
        </NTooltip>
      </header>

      <AiConversationSurface density="workspace" @open-run="detailsVisible = true" />
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
          <div class="workspace-run-card">
            <AgentRunStatus
              :run="workspace.runDetail.value.run"
              @cancel="workspace.stop"
              @resume="workspace.resumeRun"
              @retry="workspace.retryRun(workspace.runDetail.value.run.id)"
            />
          </div>
          <div class="workspace-run-metrics">
            <div class="workspace-run-metric">
              <span>{{ $t('page.aiWorkspace.steps') }}</span>
              <strong>{{ workspace.runDetail.value.run.current_step }}</strong>
            </div>
            <div class="workspace-run-metric">
              <span>{{ $t('page.aiWorkspace.modelCalls') }}</span>
              <strong>{{ workspace.runDetail.value.run.model_calls }}</strong>
            </div>
            <div class="workspace-run-metric">
              <span>{{ $t('page.aiWorkspace.toolCalls') }}</span>
              <strong>{{ workspace.runDetail.value.run.tool_calls }}</strong>
            </div>
          </div>
          <NTimeline class="mt-20px">
            <NTimelineItem
              v-for="step in workspace.runDetail.value.steps"
              :key="step.id"
              :type="step.status === 'failed' ? 'error' : step.status === 'completed' ? 'success' : 'info'"
            >
              <template #header>
                <div class="workspace-step-header">
                  <span class="workspace-step-title">{{ stepTitle(step.type) }}</span>
                  <CodeTag :code-set="CODESET_KEYS.agentStepStatus" :value="step.status" />
                </div>
              </template>
              <template #footer>
                <DateTimeText :value="step.started_at || step.created_at" class="text-11px text-gray-500" />
              </template>
            </NTimelineItem>
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
  background: rgb(var(--container-bg-color));
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
  background: rgb(var(--container-bg-color));
}

.ai-workspace__header {
  height: 56px;
  flex: none;
  display: flex;
  align-items: center;
  gap: 6px;
  border-bottom: 1px solid rgba(127, 127, 127, 0.14);
  padding: 0 16px;
  background: linear-gradient(180deg, rgba(100, 108, 255, 0.03) 0%, rgb(var(--container-bg-color) / 96%) 100%);
}

.ai-workspace__title-group {
  min-width: 0;
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.ai-workspace__subtitle {
  min-width: 0;
  display: flex;
  align-items: center;
  gap: 6px;
  color: rgba(127, 127, 127, 0.9);
  font-size: 11.5px;
}

.ai-workspace__health {
  width: 6px;
  height: 6px;
  flex: none;
  border-radius: 50%;
  background: #ef4444;
}

.ai-workspace__health--ok {
  background: #22a06b;
  box-shadow: 0 0 6px rgba(34, 160, 107, 0.45);
}

.ai-workspace__source-tag {
  flex: none;
  border: 1px solid rgba(127, 127, 127, 0.18);
  border-radius: 999px;
  background: rgba(127, 127, 127, 0.06);
  padding: 0 6px;
  font-size: 10px;
  line-height: 16px;
}

.ai-workspace__header-button {
  width: 34px;
  height: 34px;
  flex: none;
  display: grid;
  place-items: center;
  border: 1px solid transparent;
  border-radius: 8px;
  background: transparent;
  color: inherit;
  font-size: 18px;
  cursor: pointer;
  transition:
    border-color 0.16s ease,
    background 0.16s ease,
    color 0.16s ease;
}

.ai-workspace__header-button:hover,
.ai-workspace__header-button:focus-visible {
  border-color: rgba(127, 127, 127, 0.18);
  background: rgba(127, 127, 127, 0.08);
  outline: 2px solid rgba(var(--primary-color), 0.4);
  outline-offset: 1px;
}

.ai-workspace__header-button--active {
  border-color: rgba(var(--primary-color), 0.34);
  background: rgba(var(--primary-color), 0.1);
  color: rgb(var(--primary-color));
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
  background: rgb(var(--container-bg-color));
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

.ai-workspace__collapse-handle:hover,
.ai-workspace__collapse-handle:focus-visible {
  width: 28px;
  border-color: rgba(var(--primary-color), 0.5);
  background: rgba(var(--primary-color), 0.07);
  color: rgb(var(--primary-color));
  outline: none;
}

.workspace-run-card {
  border: 1px solid rgba(127, 127, 127, 0.16);
  border-radius: 10px;
  background: rgba(127, 127, 127, 0.04);
  padding: 10px 12px;
}

.workspace-run-metrics {
  margin-top: 10px;
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 8px;
}

.workspace-run-metric {
  display: flex;
  flex-direction: column;
  gap: 2px;
  border: 1px solid rgba(127, 127, 127, 0.15);
  border-radius: 9px;
  background: rgba(127, 127, 127, 0.035);
  padding: 8px 10px;
}

.workspace-run-metric span {
  color: rgba(127, 127, 127, 0.9);
  font-size: 11px;
}

.workspace-run-metric strong {
  font-size: 16px;
  font-weight: 660;
  font-variant-numeric: tabular-nums;
}

.workspace-step-header {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.workspace-step-title {
  font-size: 13px;
  font-weight: 600;
}

@media (prefers-reduced-motion: reduce) {
  .ai-workspace__sidebar,
  .ai-workspace__sidebar--collapsed,
  .ai-workspace__header-button,
  .ai-workspace__collapse-handle {
    transition: none;
  }
}
</style>
