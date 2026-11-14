<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useOnline } from '@vueuse/core';
import { useAuthStore } from '@/store/modules/auth';
import { $t } from '@/locales';
import { useAssistantPersonalization } from '@/composables/use-assistant-personalization';
import WorkspaceComposer from '@/views/ai/workspace/modules/workspace-composer.vue';
import { useAiConversationController } from '@/views/ai/workspace/use-workspace';

defineOptions({ name: 'AiConversationSurface' });

const props = withDefaults(defineProps<{ density?: 'workspace' | 'sidebar' }>(), { density: 'workspace' });
const emit = defineEmits<{ openRun: [runId: string] }>();

const authStore = useAuthStore();
const controller = useAiConversationController();
const online = useOnline();
const personalization = useAssistantPersonalization();
const runActive = computed(
  () =>
    controller.status.value === 'running' ||
    controller.status.value === 'reconnecting' ||
    controller.capturingPage.value
);
const thinking = computed(() => runActive.value && !controller.displayStreamingText.value);
const agentName = computed(() => controller.slot.value?.agent?.name || $t('page.aiWorkspace.assistant'));
const greetingName = computed(() => authStore.userInfo.userName || $t('page.aiWorkspace.assistant'));

interface BentoSuggestion {
  key: string;
  title?: string;
  prompt: string;
  icon: string;
}

const fallbackIcons = [
  'mdi:lightning-bolt-outline',
  'mdi:radar',
  'mdi:compass-outline',
  'mdi:star-four-points-outline'
] as const;

function suggestionIconFor(sourceType?: Api.AI.PersonalizationSuggestion['source_type'], index = 0) {
  if (sourceType === 'recent_work') return 'mdi:timeline-clock-outline';
  if (sourceType === 'recent_conversation') return 'mdi:message-text-outline';
  if (sourceType === 'memory_profile') return 'mdi:star-four-points-outline';
  return fallbackIcons[index % fallbackIcons.length];
}

const fallbackSuggestions = computed<BentoSuggestion[]>(() =>
  [
    $t('page.aiWorkspace.suggestionPriority'),
    $t('page.aiWorkspace.suggestionExceptions'),
    $t('page.aiWorkspace.suggestionFeature'),
    $t('page.aiWorkspace.suggestionNext')
  ]
    .slice(0, props.density === 'sidebar' ? 3 : 4)
    .map((prompt, index) => ({
      key: `fallback-${index}`,
      prompt,
      icon: fallbackIcons[index % fallbackIcons.length]
    }))
);

const suggestions = computed<BentoSuggestion[]>(() => {
  const limit = props.density === 'sidebar' ? 3 : 4;
  if (personalization.suggestions.value.length > 0) {
    return personalization.suggestions.value.slice(0, limit).map((item, index) => ({
      key: `personal-${index}-${item.prompt}`,
      title: item.title && item.title !== item.prompt ? item.title : undefined,
      prompt: item.prompt,
      icon: suggestionIconFor(item.source_type, index)
    }));
  }
  return fallbackSuggestions.value;
});

const welcomeGreeting = computed(
  () => personalization.greeting.value || $t('page.aiWorkspace.greeting', { name: greetingName.value })
);

const showScrollBottom = ref(false);
let surfaceElement: HTMLElement | null = null;

function updateScrollState() {
  if (!surfaceElement) {
    showScrollBottom.value = false;
    return;
  }
  const hasConversation = Boolean(controller.conversation.value || controller.messages.value.length > 0);
  if (!hasConversation) {
    showScrollBottom.value = false;
    return;
  }
  const distance = surfaceElement.scrollHeight - surfaceElement.scrollTop - surfaceElement.clientHeight;
  showScrollBottom.value = distance > 108;
}

function setMessageListElement(element: unknown) {
  surfaceElement = element instanceof HTMLElement ? element : null;
  controller.messageListEl.value = surfaceElement;
  updateScrollState();
}

function jumpToBottom() {
  if (!surfaceElement) return;
  const prefersReducedMotion =
    typeof window !== 'undefined' &&
    typeof window.matchMedia === 'function' &&
    window.matchMedia('(prefers-reduced-motion: reduce)').matches;
  if (typeof surfaceElement.scrollTo === 'function') {
    surfaceElement.scrollTo({
      top: surfaceElement.scrollHeight,
      behavior: prefersReducedMotion ? 'auto' : 'smooth'
    });
  } else {
    surfaceElement.scrollTop = surfaceElement.scrollHeight;
  }
  showScrollBottom.value = false;
}

function useSuggestion(value: string) {
  personalization.freeze();
  controller.input.value = value;
}

async function handleOpenRun(runId: string) {
  await controller.inspectRun(runId);
  emit('openRun', runId);
}

function handleRetryRun(runId: string) {
  void controller.retryRun(runId);
}

watch([controller.input, controller.conversation], ([input, conversation]) => {
  if (input || conversation) personalization.freeze();
});
watch(
  [() => controller.messages.value.length, controller.displayStreamingText, () => controller.conversation.value?.id],
  () => {
    void nextTick(updateScrollState);
  }
);
onMounted(() => {
  void controller.initialize();
  void personalization.load(`${authStore.userInfo.userId || ''}`);
  const pendingPrompt = personalization.consumePrompt();
  if (pendingPrompt && !controller.conversation.value) controller.input.value = pendingPrompt;
  void nextTick(updateScrollState);
});
onBeforeUnmount(() => {
  if (controller.messageListEl.value === surfaceElement) controller.messageListEl.value = null;
});
</script>

<template>
  <div class="ai-conversation-surface" :class="`ai-conversation-surface--${density}`">
    <div class="ai-conversation-surface__viewport">
      <section
        :ref="setMessageListElement"
        class="ai-conversation-surface__messages"
        @scroll.passive="updateScrollState"
      >
        <div v-if="controller.status.value === 'loading'" class="h-full flex-center">
          <NSpin size="large" />
        </div>
        <div
          v-else-if="!controller.conversation.value && controller.messages.value.length === 0"
          class="ai-conversation-surface__welcome"
        >
          <div class="ai-conversation-surface__mascot-halo">
            <AiMascot :state="controller.mascotState.value" :size="density === 'sidebar' ? 76 : 124" />
          </div>
          <h2>{{ welcomeGreeting }}</h2>
          <p v-if="controller.slot.value?.healthy" class="ai-conversation-surface__ready">
            <span class="ai-conversation-surface__ready-dot" aria-hidden="true" />
            <span>{{ $t('page.aiWorkspace.ready', { agent: agentName }) }}</span>
          </p>
          <NAlert v-else type="error" :show-icon="true">
            {{ controller.error.value || $t('page.aiWorkspace.unavailable') }}
          </NAlert>
          <div class="ai-conversation-surface__suggestions">
            <button
              v-for="item in suggestions"
              :key="item.key"
              type="button"
              class="ai-suggestion-card"
              @click="useSuggestion(item.prompt)"
            >
              <span class="ai-suggestion-card__icon" aria-hidden="true">
                <SvgIcon :icon="item.icon" />
              </span>
              <span class="ai-suggestion-card__content">
                <strong v-if="item.title" class="ai-suggestion-card__title">{{ item.title }}</strong>
                <span class="ai-suggestion-card__prompt">{{ item.prompt }}</span>
              </span>
              <SvgIcon icon="mdi:arrow-up-right" class="ai-suggestion-card__arrow" aria-hidden="true" />
            </button>
          </div>
        </div>
        <ChatMessageList
          v-else
          :messages="controller.messages.value"
          :user-avatar-attachment-id="authStore.userInfo.avatarAttachmentId"
          :user-name="authStore.userInfo.userName"
          :agent-name="agentName"
          :streaming-text="controller.displayStreamingText.value"
          :thinking="thinking"
          :thinking-hint="controller.thinkingHint.value"
          :active-run-id="controller.activeRunId.value"
          :failed-run-id="controller.failedRunId.value"
          :can-retry-failed-run="controller.canRetryFailedRun.value"
          :retrying="controller.retrying.value"
          :confirmations="controller.confirmations.value"
          :error="controller.error.value"
          @confirmation-resolved="controller.onConfirmationResolved"
          @open-run="handleOpenRun"
          @retry-run="handleRetryRun"
        />
      </section>

      <div
        v-if="controller.conversation.value || controller.messages.value.length > 0"
        class="ai-conversation-surface__fade"
        aria-hidden="true"
      />

      <Transition name="ai-scroll-pill">
        <button
          v-if="showScrollBottom"
          type="button"
          class="ai-conversation-surface__scroll-bottom"
          :class="{ 'ai-conversation-surface__scroll-bottom--live': runActive }"
          :aria-label="
            runActive ? $t('page.aiWorkspace.scrollToBottomStreaming') : $t('page.aiWorkspace.scrollToBottom')
          "
          @click="jumpToBottom"
        >
          <span v-if="runActive" class="ai-conversation-surface__scroll-dot" aria-hidden="true" />
          <span>
            {{ runActive ? $t('page.aiWorkspace.scrollToBottomStreaming') : $t('page.aiWorkspace.scrollToBottom') }}
          </span>
          <SvgIcon icon="mdi:arrow-down" class="text-14px" aria-hidden="true" />
        </button>
      </Transition>
    </div>

    <footer class="ai-conversation-surface__composer">
      <div v-if="!online" class="mb-6px text-center text-12px text-error">
        {{ $t('page.aiWorkspace.offline') }}
      </div>
      <div v-if="controller.status.value === 'reconnecting'" class="mb-6px text-center text-12px text-warning">
        {{ $t('page.aiWorkspace.reconnecting') }}
      </div>
      <WorkspaceComposer
        v-model="controller.input.value"
        v-model:attachments="controller.attachments.value"
        :disabled="!controller.slot.value?.healthy || !online"
        :offline="!online"
        :failed="Boolean(controller.error.value)"
        :can-send="controller.canSend.value"
        :active="runActive"
        :draft-key="controller.conversation.value?.id || 'new'"
        :density="density"
        @send="controller.send"
        @stop="controller.stop"
      />
      <div v-if="density === 'workspace'" class="ai-conversation-surface__meta">
        <span>{{ $t('page.aiWorkspace.disclaimer') }}</span>
        <span class="ai-conversation-surface__shortcut">{{ $t('page.aiWorkspace.shortcutHint') }}</span>
      </div>
    </footer>
  </div>
</template>

<style scoped>
.ai-conversation-surface {
  min-height: 0;
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.ai-conversation-surface__viewport {
  position: relative;
  min-height: 0;
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.ai-conversation-surface__messages {
  min-height: 0;
  flex: 1;
  overflow-x: hidden;
  overflow-y: auto;
  padding: 18px clamp(14px, 3vw, 42px) 28px;
}

.ai-conversation-surface__fade {
  position: absolute;
  right: 0;
  bottom: 0;
  left: 0;
  height: 36px;
  background: linear-gradient(180deg, rgb(var(--container-bg-color) / 0%) 0%, rgb(var(--container-bg-color)) 92%);
  pointer-events: none;
}

.ai-conversation-surface__scroll-bottom {
  position: absolute;
  bottom: 12px;
  left: 50%;
  z-index: 5;
  transform: translateX(-50%);
  height: 30px;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  border: 1px solid rgb(var(--primary-color) / 0.28);
  border-radius: 999px;
  background: rgb(var(--container-bg-color) / 92%);
  backdrop-filter: blur(8px);
  box-shadow: 0 6px 18px rgba(15, 23, 42, 0.12);
  padding: 0 12px;
  color: rgb(var(--base-text-color));
  font-size: 12px;
  font-weight: 550;
  white-space: nowrap;
  cursor: pointer;
  transition:
    border-color 0.18s ease,
    box-shadow 0.18s ease,
    color 0.18s ease;
}

.ai-conversation-surface__scroll-bottom:hover,
.ai-conversation-surface__scroll-bottom:focus-visible {
  border-color: rgb(var(--primary-color) / 0.58);
  color: rgb(var(--primary-color));
  box-shadow: 0 8px 22px rgb(var(--primary-color) / 0.18);
  outline: none;
}

.ai-conversation-surface__scroll-bottom--live {
  border-color: rgb(var(--primary-color) / 0.45);
}

.ai-conversation-surface__scroll-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: linear-gradient(135deg, #646cff, #22d3ee);
  animation: scroll-live-pulse 1.4s ease-in-out infinite;
}

@keyframes scroll-live-pulse {
  0%,
  100% {
    opacity: 1;
    transform: scale(1);
  }
  50% {
    opacity: 0.35;
    transform: scale(0.82);
  }
}

.ai-scroll-pill-enter-active,
.ai-scroll-pill-leave-active {
  transition:
    opacity 0.18s ease,
    transform 0.18s ease;
}

.ai-scroll-pill-enter-from,
.ai-scroll-pill-leave-to {
  opacity: 0;
  transform: translate(-50%, 8px);
}

.ai-conversation-surface__welcome {
  width: min(680px, 100%);
  min-height: 100%;
  margin: 0 auto;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  text-align: center;
}

.ai-conversation-surface__mascot-halo {
  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
}

.ai-conversation-surface__mascot-halo::before {
  content: '';
  position: absolute;
  inset: -10px;
  z-index: -1;
  border-radius: 50%;
  background: radial-gradient(circle, rgba(100, 108, 255, 0.14) 0%, rgba(8, 145, 178, 0.05) 45%, transparent 72%);
  pointer-events: none;
}

.ai-conversation-surface__welcome h2 {
  margin: 0;
  font-size: 24px;
  font-weight: 660;
  letter-spacing: -0.015em;
}

.ai-conversation-surface__ready {
  margin: 0;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  border: 1px solid rgba(127, 127, 127, 0.16);
  border-radius: 999px;
  background: rgba(127, 127, 127, 0.05);
  padding: 3px 11px;
  color: rgba(127, 127, 127, 0.95);
  font-size: 12.5px;
}

.ai-conversation-surface__ready-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: #10b981;
}

.ai-conversation-surface__suggestions {
  width: min(620px, 100%);
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
  margin-top: 10px;
}

.ai-suggestion-card {
  position: relative;
  min-width: 0;
  min-height: 56px;
  display: flex;
  align-items: flex-start;
  gap: 10px;
  border: 1px solid rgba(127, 127, 127, 0.17);
  border-radius: 12px;
  background: rgb(var(--container-bg-color));
  padding: 11px 13px;
  color: inherit;
  text-align: left;
  cursor: pointer;
  transition:
    border-color 0.2s ease,
    box-shadow 0.2s ease,
    background 0.2s ease;
}

.ai-suggestion-card:hover,
.ai-suggestion-card:focus-visible {
  border-color: rgb(var(--primary-color) / 0.48);
  background: linear-gradient(135deg, rgb(var(--primary-color) / 0.055) 0%, rgba(8, 145, 178, 0.03) 100%);
  box-shadow: 0 6px 18px rgb(var(--primary-color) / 0.09);
  outline: none;
}

.ai-suggestion-card__icon {
  width: 28px;
  height: 28px;
  margin-top: 1px;
  flex: none;
  display: grid;
  place-items: center;
  border-radius: 8px;
  background: rgb(var(--primary-color) / 0.1);
  color: rgb(var(--primary-color));
  font-size: 15px;
}

.ai-suggestion-card__content {
  min-width: 0;
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.ai-suggestion-card__title {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 12.5px;
  font-weight: 620;
}

.ai-suggestion-card__prompt {
  min-width: 0;
  overflow-wrap: break-word;
  color: rgb(var(--base-text-color) / 0.86);
  font-size: 12.5px;
  line-height: 1.48;
}

.ai-suggestion-card__arrow {
  flex: none;
  margin-top: 2px;
  color: rgba(127, 127, 127, 0.65);
  font-size: 15px;
  transition:
    transform 0.18s ease,
    color 0.18s ease;
}

.ai-suggestion-card:hover .ai-suggestion-card__arrow,
.ai-suggestion-card:focus-visible .ai-suggestion-card__arrow {
  color: rgb(var(--primary-color));
  transform: translate(2px, -2px);
}

.ai-conversation-surface__composer {
  flex: none;
  padding: 8px clamp(14px, 3vw, 42px) 14px;
  background: rgb(var(--container-bg-color));
}

.ai-conversation-surface__meta {
  width: min(820px, 100%);
  margin: 7px auto 0;
  padding: 0 12px;
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  color: rgba(127, 127, 127, 0.78);
  font-size: 11px;
}

.ai-conversation-surface__shortcut {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 10.5px;
  opacity: 0.85;
}

.ai-conversation-surface--sidebar .ai-conversation-surface__messages {
  padding: 12px 12px 22px;
}

.ai-conversation-surface--sidebar .ai-conversation-surface__welcome {
  justify-content: flex-start;
  gap: 9px;
  padding-top: 28px;
}

.ai-conversation-surface--sidebar .ai-conversation-surface__welcome h2 {
  font-size: 17px;
}

.ai-conversation-surface--sidebar .ai-conversation-surface__suggestions {
  grid-template-columns: 1fr;
  gap: 8px;
}

.ai-conversation-surface--sidebar .ai-suggestion-card {
  min-height: 48px;
  padding: 9px 11px;
}

.ai-conversation-surface--sidebar .ai-conversation-surface__composer {
  padding: 8px 12px 12px;
}

.ai-conversation-surface--sidebar :deep(.chat-message-list) {
  padding: 10px 0 48px;
}

.ai-conversation-surface--sidebar :deep(.message__content) {
  max-width: calc(100% - 44px);
}

.ai-conversation-surface--sidebar :deep(.message__user-stack) {
  max-width: calc(100% - 44px);
}

@media (max-width: 640px) {
  .ai-conversation-surface__suggestions {
    grid-template-columns: 1fr;
  }

  .ai-conversation-surface__meta {
    justify-content: center;
  }

  .ai-conversation-surface__shortcut {
    display: none;
  }
}

@media (prefers-reduced-motion: reduce) {
  .ai-conversation-surface *,
  .ai-conversation-surface *::before,
  .ai-conversation-surface *::after {
    scroll-behavior: auto !important;
    transition-duration: 0.01ms !important;
    animation-duration: 0.01ms !important;
  }
}
</style>
