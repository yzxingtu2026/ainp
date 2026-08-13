<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, watch } from 'vue';
import { useOnline } from '@vueuse/core';
import { useAuthStore } from '@/store/modules/auth';
import { $t } from '@/locales';
import { useAssistantPersonalization } from '@/composables/use-assistant-personalization';
import WorkspaceComposer from '@/views/ai/workspace/modules/workspace-composer.vue';
import { useAiConversationController } from '@/views/ai/workspace/use-workspace';

defineOptions({ name: 'AiConversationSurface' });

const props = withDefaults(defineProps<{ density?: 'workspace' | 'sidebar' }>(), { density: 'workspace' });
const authStore = useAuthStore();
const controller = useAiConversationController();
const online = useOnline();
const personalization = useAssistantPersonalization();
const runActive = computed(() => controller.status.value === 'running' || controller.status.value === 'reconnecting');
const thinking = computed(() => runActive.value && !controller.displayStreamingText.value);
const agentName = computed(() => controller.slot.value?.agent?.name || $t('page.aiWorkspace.assistant'));
const greetingName = computed(() => authStore.userInfo.userName || $t('page.aiWorkspace.assistant'));
const fallbackSuggestions = computed(() =>
  [
    $t('page.aiWorkspace.suggestionPriority'),
    $t('page.aiWorkspace.suggestionExceptions'),
    $t('page.aiWorkspace.suggestionFeature'),
    $t('page.aiWorkspace.suggestionNext')
  ].slice(0, props.density === 'sidebar' ? 3 : 4)
);
const suggestions = computed(() =>
  (personalization.suggestions.value.length
    ? personalization.suggestions.value.map(item => item.prompt)
    : fallbackSuggestions.value
  ).slice(0, props.density === 'sidebar' ? 3 : 4)
);
const welcomeGreeting = computed(
  () => personalization.greeting.value || $t('page.aiWorkspace.greeting', { name: greetingName.value })
);

let surfaceElement: HTMLElement | null = null;

function setMessageListElement(element: unknown) {
  surfaceElement = element instanceof HTMLElement ? element : null;
  controller.messageListEl.value = surfaceElement;
}

function useSuggestion(value: string) {
  personalization.freeze();
  controller.input.value = value;
}

watch([controller.input, controller.conversation], ([input, conversation]) => {
  if (input || conversation) personalization.freeze();
});
onMounted(() => {
  void controller.initialize();
  void personalization.load(`${authStore.userInfo.userId || ''}`);
  const pendingPrompt = personalization.consumePrompt();
  if (pendingPrompt && !controller.conversation.value) controller.input.value = pendingPrompt;
});
onBeforeUnmount(() => {
  if (controller.messageListEl.value === surfaceElement) controller.messageListEl.value = null;
});
</script>

<template>
  <div class="ai-conversation-surface" :class="`ai-conversation-surface--${density}`">
    <section :ref="setMessageListElement" class="ai-conversation-surface__messages">
      <div v-if="controller.status.value === 'loading'" class="h-full flex-center">
        <NSpin size="large" />
      </div>
      <div
        v-else-if="!controller.conversation.value && controller.messages.value.length === 0"
        class="ai-conversation-surface__welcome"
      >
        <AiMascot :state="controller.mascotState.value" :size="density === 'sidebar' ? 76 : 132" />
        <h2>{{ welcomeGreeting }}</h2>
        <p v-if="controller.slot.value?.healthy">{{ $t('page.aiWorkspace.ready', { agent: agentName }) }}</p>
        <NAlert v-else type="error" :show-icon="true">
          {{ controller.error.value || $t('page.aiWorkspace.unavailable') }}
        </NAlert>
        <div class="ai-conversation-surface__suggestions">
          <button v-for="suggestion in suggestions" :key="suggestion" type="button" @click="useSuggestion(suggestion)">
            <span>{{ suggestion }}</span>
            <SvgIcon icon="mdi:arrow-up-right" />
          </button>
        </div>
      </div>
      <ChatMessageList
        v-else
        :messages="controller.messages.value"
        :user-avatar-attachment-id="authStore.userInfo.avatarAttachmentId"
        :user-name="authStore.userInfo.userName"
        :streaming-text="controller.displayStreamingText.value"
        :thinking="thinking"
        :thinking-hint="controller.thinkingHint.value"
        :active-run-id="controller.activeRunId.value"
        :confirmations="controller.confirmations.value"
        :error="controller.error.value"
        @confirmation-resolved="controller.onConfirmationResolved"
      />
    </section>
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
        :can-send="controller.canSend.value"
        :active="runActive"
        :draft-key="controller.conversation.value?.id || 'new'"
        @send="controller.send"
        @stop="controller.stop"
      />
      <div v-if="density === 'workspace'" class="mt-7px text-center text-11px text-gray-400">
        {{ $t('page.aiWorkspace.disclaimer') }}
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
.ai-conversation-surface__messages {
  min-height: 0;
  flex: 1;
  overflow-x: hidden;
  overflow-y: auto;
  padding: 18px clamp(14px, 3vw, 42px) 24px;
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
.ai-conversation-surface__welcome h2 {
  margin: 0;
  font-size: 24px;
  font-weight: 650;
}
.ai-conversation-surface__welcome p {
  margin: 0;
  color: rgba(127, 127, 127, 0.9);
}
.ai-conversation-surface__suggestions {
  width: min(600px, 100%);
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
  margin-top: 8px;
}
.ai-conversation-surface__suggestions button {
  min-width: 0;
  min-height: 44px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  border: 1px solid rgba(127, 127, 127, 0.16);
  border-radius: 6px;
  background: rgb(var(--container-bg-color));
  padding: 9px 12px;
  color: inherit;
  text-align: left;
  cursor: pointer;
}
.ai-conversation-surface__suggestions span {
  min-width: 0;
  overflow-wrap: anywhere;
}
.ai-conversation-surface__composer {
  flex: none;
  padding: 10px clamp(14px, 3vw, 42px) 16px;
  background: rgb(var(--container-bg-color));
}
.ai-conversation-surface--sidebar .ai-conversation-surface__messages {
  padding: 12px 14px 18px;
}
.ai-conversation-surface--sidebar .ai-conversation-surface__welcome {
  justify-content: flex-start;
  gap: 9px;
  padding-top: 36px;
}
.ai-conversation-surface--sidebar .ai-conversation-surface__welcome h2 {
  font-size: 17px;
}
.ai-conversation-surface--sidebar .ai-conversation-surface__suggestions {
  grid-template-columns: 1fr;
}
.ai-conversation-surface--sidebar .ai-conversation-surface__composer {
  padding: 8px 12px 12px;
}
.ai-conversation-surface--sidebar :deep(.chat-message-list) {
  padding: 12px 0 48px;
}
.ai-conversation-surface--sidebar :deep(.message__content) {
  max-width: calc(100% - 46px);
}
@media (max-width: 640px) {
  .ai-conversation-surface__suggestions {
    grid-template-columns: 1fr;
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
