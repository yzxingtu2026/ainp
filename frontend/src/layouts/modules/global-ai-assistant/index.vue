<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue';
import { useRouter } from 'vue-router';
import CodeTag from '@/components/common/code-tag.vue';
import DateTimeText from '@/components/common/datetime-text.vue';
import { CODESET_KEYS } from '@/constants/codeset';
import { useAppStore } from '@/store/modules/app';
import { useAiAssistantStore } from '@/store/modules/ai-assistant';
import { useRouteStore } from '@/store/modules/route';
import { useThemeStore } from '@/store/modules/theme';
import { useAiConversationController } from '@/views/ai/workspace/use-workspace';
import ConversationPanel from '@/views/ai/workspace/modules/conversation-panel.vue';
import { $t } from '@/locales';

defineOptions({ name: 'GlobalAiAssistant' });

const appStore = useAppStore();
const store = useAiAssistantStore();
const routeStore = useRouteStore();
const themeStore = useThemeStore();
const router = useRouter();
const controller = useAiConversationController();
const panel = ref<HTMLElement | null>(null);
const renameVisible = ref(false);
const renameTitle = ref('');
const renameTarget = ref<Api.AI.Conversation | null>(null);
const docked = computed(() => !appStore.isMobile);
const agentName = computed(() => controller.slot.value?.agent?.name || $t('page.aiWorkspace.assistant'));
const healthy = computed(() => Boolean(controller.slot.value?.healthy));

function stepTitle(type: string) {
  if (type === 'model') return $t('chat.stepModel');
  if (type === 'tool_search') return $t('chat.stepToolSearch');
  if (type === 'tool_load') return $t('chat.stepToolLoad');
  if (type === 'tool_execute') return $t('chat.stepToolExecute');
  if (type === 'context_assemble') return $t('chat.stepContextAssemble');
  if (type === 'finalize') return $t('chat.stepFinalize');
  return type;
}

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
  <Teleport to="body" :disabled="docked">
    <Transition name="ai-assistant-panel">
      <aside
        v-if="store.visible"
        ref="panel"
        data-ainp-ai-assistant="true"
        class="global-ai-assistant"
        :class="{ 'global-ai-assistant--docked': docked }"
        tabindex="-1"
        role="complementary"
        :aria-label="$t('page.aiWorkspace.assistant')"
        :style="{ '--ai-header-height': `${themeStore.header.height}px` }"
        @keydown.esc.stop="store.close"
      >
        <header class="global-ai-assistant__header">
          <span class="global-ai-assistant__avatar">
            <AiMascot :state="controller.mascotState.value" :size="36" />
          </span>
          <div class="min-w-0 flex-1">
            <div class="truncate text-13.5px font-650">{{ agentName }}</div>
            <div class="flex-y-center gap-5px text-11px text-gray-500">
              <span class="global-ai-assistant__health" :class="{ 'global-ai-assistant__health--ok': healthy }"></span>
              {{ healthy ? $t('page.aiWorkspace.readyShort') : $t('page.aiWorkspace.unavailable') }}
            </div>
          </div>
          <NTooltip>
            <template #trigger>
              <button
                class="global-ai-assistant__tool"
                type="button"
                :aria-label="$t('page.aiWorkspace.newConversation')"
                @click="newConversation"
              >
                <SvgIcon icon="mdi:plus" />
              </button>
            </template>
            {{ $t('page.aiWorkspace.newConversation') }}
          </NTooltip>
          <NTooltip>
            <template #trigger>
              <button
                class="global-ai-assistant__tool"
                :class="{ 'global-ai-assistant__tool--active': store.view === 'history' }"
                type="button"
                :aria-label="$t('page.aiWorkspace.openConversations')"
                @click="store.show(store.view === 'history' ? 'conversation' : 'history')"
              >
                <SvgIcon icon="mdi:history" />
              </button>
            </template>
            {{ $t('page.aiWorkspace.openConversations') }}
          </NTooltip>
          <NTooltip>
            <template #trigger>
              <button
                class="global-ai-assistant__tool"
                :class="{ 'global-ai-assistant__tool--active': store.view === 'run' }"
                type="button"
                :aria-label="$t('page.aiWorkspace.runDetails')"
                @click="store.show(store.view === 'run' ? 'conversation' : 'run')"
              >
                <SvgIcon icon="mdi:timeline-clock-outline" />
              </button>
            </template>
            {{ $t('page.aiWorkspace.runDetails') }}
          </NTooltip>
          <NTooltip>
            <template #trigger>
              <button
                class="global-ai-assistant__tool"
                type="button"
                :aria-label="$t('page.aiWorkspace.expandWorkspace')"
                @click="expandWorkspace"
              >
                <SvgIcon icon="mdi:arrow-expand" />
              </button>
            </template>
            {{ $t('page.aiWorkspace.expandWorkspace') }}
          </NTooltip>
          <NTooltip>
            <template #trigger>
              <button
                class="global-ai-assistant__tool"
                type="button"
                :aria-label="$t('common.close')"
                @click="store.close"
              >
                <SvgIcon icon="mdi:close" />
              </button>
            </template>
            {{ $t('common.close') }}
          </NTooltip>
        </header>

        <div v-if="store.view === 'conversation'" class="global-ai-assistant__body">
          <AiConversationSurface density="sidebar" @open-run="store.show('run')" />
        </div>

        <div v-else-if="store.view === 'history'" class="global-ai-assistant__secondary">
          <div class="global-ai-assistant__secondary-title">
            <button
              class="global-ai-assistant__tool"
              type="button"
              :aria-label="$t('common.back')"
              @click="store.show('conversation')"
            >
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
            <button
              class="global-ai-assistant__tool"
              type="button"
              :aria-label="$t('common.back')"
              @click="store.show('conversation')"
            >
              <SvgIcon icon="mdi:arrow-left" />
            </button>
            <strong>{{ $t('page.aiWorkspace.runDetails') }}</strong>
          </div>
          <div class="global-ai-assistant__run">
            <NEmpty v-if="!controller.runDetail.value" :description="$t('page.aiWorkspace.noRun')" />
            <template v-else>
              <div class="run-status-card">
                <AgentRunStatus
                  :run="controller.runDetail.value.run"
                  @cancel="controller.stop"
                  @resume="controller.resumeRun"
                  @retry="controller.retryRun(controller.runDetail.value.run.id)"
                />
              </div>
              <div class="run-metrics-grid">
                <div class="run-metric-item">
                  <span>{{ $t('page.aiWorkspace.steps') }}</span>
                  <strong>{{ controller.runDetail.value.run.current_step }}</strong>
                </div>
                <div class="run-metric-item">
                  <span>{{ $t('page.aiWorkspace.modelCalls') }}</span>
                  <strong>{{ controller.runDetail.value.run.model_calls }}</strong>
                </div>
                <div class="run-metric-item">
                  <span>{{ $t('page.aiWorkspace.toolCalls') }}</span>
                  <strong>{{ controller.runDetail.value.run.tool_calls }}</strong>
                </div>
              </div>
              <NTimeline class="mt-16px">
                <NTimelineItem
                  v-for="step in controller.runDetail.value.steps"
                  :key="step.id"
                  :type="step.status === 'failed' ? 'error' : step.status === 'completed' ? 'success' : 'info'"
                >
                  <template #header>
                    <div class="run-step-header">
                      <span class="run-step-title">{{ stepTitle(step.type) }}</span>
                      <CodeTag :code-set="CODESET_KEYS.agentStepStatus" :value="step.status" />
                    </div>
                  </template>
                  <template #footer>
                    <DateTimeText :value="step.started_at || step.created_at" class="text-11px text-gray-500" />
                  </template>
                </NTimelineItem>
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
  </Teleport>
</template>

<style scoped>
.global-ai-assistant {
  position: fixed;
  z-index: 1100;
  top: var(--ai-header-height, 56px);
  right: 0;
  bottom: 0;
  width: clamp(400px, 36vw, min(640px, 48vw));
  min-width: 0;
  display: flex;
  flex-direction: column;
  isolation: isolate;
  overflow: hidden;
  border-left: 1px solid rgba(127, 127, 127, 0.18);
  background: rgb(var(--container-bg-color));
  box-shadow: -10px 0 28px rgba(25, 31, 46, 0.14);
  outline: none;
  pointer-events: auto;
}

.global-ai-assistant--docked {
  position: relative;
  top: auto;
  right: auto;
  bottom: auto;
  z-index: auto;
  width: 100%;
  height: 100%;
  box-shadow: none;
}

.global-ai-assistant__header {
  min-height: 56px;
  flex: none;
  display: flex;
  align-items: center;
  gap: 6px;
  border-bottom: 1px solid rgba(127, 127, 127, 0.14);
  background: linear-gradient(180deg, rgba(100, 108, 255, 0.035) 0%, transparent 100%);
  padding: 6px 10px;
}

.global-ai-assistant__avatar {
  width: 38px;
  height: 38px;
  flex: none;
  display: grid;
  place-items: center;
  border: 1px solid rgba(127, 127, 127, 0.16);
  border-radius: 11px;
  background: rgb(var(--container-bg-color));
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
  box-shadow: 0 0 6px rgba(34, 160, 107, 0.45);
}

.global-ai-assistant__tool {
  width: 32px;
  height: 32px;
  flex: none;
  display: grid;
  place-items: center;
  border: 1px solid transparent;
  border-radius: 8px;
  background: transparent;
  color: inherit;
  cursor: pointer;
  transition:
    border-color 0.16s ease,
    background 0.16s ease,
    color 0.16s ease;
}

.global-ai-assistant__tool:hover,
.global-ai-assistant__tool:focus-visible {
  border-color: rgba(127, 127, 127, 0.18);
  background: rgba(127, 127, 127, 0.08);
  outline: 2px solid rgba(var(--primary-color), 0.4);
  outline-offset: 1px;
}

.global-ai-assistant__tool--active {
  border-color: rgba(var(--primary-color), 0.34);
  background: rgba(var(--primary-color), 0.1);
  color: rgb(var(--primary-color));
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
  min-height: 46px;
  flex: none;
  display: flex;
  align-items: center;
  gap: 8px;
  border-bottom: 1px solid rgba(127, 127, 127, 0.13);
  padding: 6px 12px;
  font-size: 13.5px;
}

.global-ai-assistant__run {
  min-height: 0;
  flex: 1;
  overflow-x: hidden;
  overflow-y: auto;
  padding: 14px;
}

.run-status-card {
  border: 1px solid rgba(127, 127, 127, 0.16);
  border-radius: 10px;
  background: rgba(127, 127, 127, 0.04);
  padding: 10px 12px;
}

.run-metrics-grid {
  margin-top: 10px;
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 8px;
}

.run-metric-item {
  display: flex;
  flex-direction: column;
  gap: 2px;
  border: 1px solid rgba(127, 127, 127, 0.15);
  border-radius: 9px;
  background: rgba(127, 127, 127, 0.035);
  padding: 8px 10px;
}

.run-metric-item span {
  color: rgba(127, 127, 127, 0.9);
  font-size: 11px;
}

.run-metric-item strong {
  font-size: 16px;
  font-weight: 660;
  font-variant-numeric: tabular-nums;
}

.run-step-header {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.run-step-title {
  font-size: 13px;
  font-weight: 600;
}

.ai-assistant-panel-enter-active,
.ai-assistant-panel-leave-active {
  transition:
    transform 180ms ease,
    opacity 180ms ease;
}

.ai-assistant-panel-enter-from,
.ai-assistant-panel-leave-to {
  transform: translateX(24px);
  opacity: 0;
}

@media (max-width: 840px) {
  .global-ai-assistant:not(.global-ai-assistant--docked) {
    top: 0;
    width: 100vw;
    max-width: none;
  }
}

@media (prefers-reduced-motion: reduce) {
  .global-ai-assistant__tool,
  .ai-assistant-panel-enter-active,
  .ai-assistant-panel-leave-active {
    transition: none;
  }
}
</style>
