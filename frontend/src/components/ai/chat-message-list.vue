<script setup lang="ts">
import { computed } from 'vue';
import { $t } from '@/locales';
import type { ToolOperation } from './tool-activity';

defineOptions({ name: 'ChatMessageList' });

const props = withDefaults(
  defineProps<{
    messages: Api.AI.ConversationMessage[];
    /** 流式输出中的增量文本 */
    streamingText?: string;
    /** 运行中且尚无输出时展示思考占位 */
    thinking?: boolean;
    /** 所属运行是否仍在进行（决定工具组实时展示或折叠） */
    runActive?: boolean;
    confirmations?: Api.AI.ToolConfirmation[];
    error?: string;
  }>(),
  {
    streamingText: '',
    thinking: false,
    runActive: false,
    confirmations: () => [],
    error: ''
  }
);

const emit = defineEmits<{
  confirmationResolved: [status: 'approved' | 'rejected', result?: Api.AI.ToolResult];
}>();

interface RenderedMessage {
  message: Api.AI.ConversationMessage;
  text: string;
  imageCount: number;
  operations: ToolOperation[];
}

function messageText(message: Api.AI.ConversationMessage): string {
  return (message.content || [])
    .filter(part => part.type === 'input_text' || part.type === 'output_text')
    .map(part => part.text || '')
    .filter(Boolean)
    .join('\n');
}

/** tool 消息的 tool_result 按 tool_call_id 建索引，供 assistant 消息的 tool_call 配对 */
const toolResultIndex = computed(() => {
  const index = new Map<string, unknown>();
  for (const message of props.messages) {
    if (message.role !== 'tool') continue;
    for (const part of message.content || []) {
      if (part.type === 'tool_result' && part.tool_call_id) index.set(part.tool_call_id, part.result);
    }
  }
  return index;
});

const renderedMessages = computed<RenderedMessage[]>(() =>
  props.messages
    .filter(message => message.role !== 'tool')
    .map(message => {
      const operations: ToolOperation[] = [];
      let imageCount = 0;
      for (const part of message.content || []) {
        if (part.type === 'tool_call') {
          const callId = part.tool_call_id || '';
          operations.push({
            toolCallId: callId,
            toolName: part.tool_name || '',
            request: part.arguments,
            result: callId ? toolResultIndex.value.get(callId) : undefined,
            hasResult: Boolean(callId && toolResultIndex.value.has(callId))
          });
        } else if (part.type === 'input_image') {
          imageCount += 1;
        }
      }
      return { message, text: messageText(message), imageCount, operations };
    })
);

function onConfirmationResolved(status: 'approved' | 'rejected', result?: Api.AI.ToolResult) {
  emit('confirmationResolved', status, result);
}
</script>

<template>
  <div class="chat-message-list">
    <article
      v-for="item in renderedMessages"
      :key="item.message.id"
      class="message"
      :class="`message--${item.message.role}`"
    >
      <AiMascot v-if="item.message.role === 'assistant'" :size="36" :breathe="false" />
      <div class="message__content">
        <MarkdownRenderer v-if="item.text" :content="item.text" />
        <NTag v-for="index in item.imageCount" :key="`image-${index}`" size="small" class="mt-6px">
          <template #icon><SvgIcon icon="mdi:image-outline" /></template>
          {{ $t('chat.imageAttachment') }}
        </NTag>
        <ToolActivityGroup v-if="item.operations.length > 0" :operations="item.operations" :active="runActive" />
      </div>
      <span v-if="item.message.role === 'user'" class="message__user-avatar" aria-hidden="true">
        <SvgIcon icon="mdi:account" />
      </span>
    </article>

    <article v-if="thinking" class="message message--assistant">
      <span class="thinking-mascot">
        <AiMascot state="thinking" :size="36" :breathe="false" />
      </span>
      <div class="message__content">
        <span class="thinking-bubble" role="status" :aria-label="$t('chat.thinking')">
          <i class="thinking-bubble__dot"></i>
          <i class="thinking-bubble__dot"></i>
          <i class="thinking-bubble__dot"></i>
        </span>
      </div>
    </article>

    <article v-else-if="streamingText" class="message message--assistant">
      <AiMascot state="working" :size="36" :breathe="false" />
      <div class="message__content">
        <MarkdownRenderer :content="streamingText" />
      </div>
    </article>

    <ToolConfirmationCard
      v-for="confirmation in confirmations"
      :key="confirmation.id"
      :confirmation="confirmation"
      class="mx-auto my-12px"
      @resolved="onConfirmationResolved"
    />

    <NAlert v-if="error" type="error" class="mx-auto mt-12px max-w-760px" closable>
      {{ error }}
    </NAlert>
  </div>
</template>

<style scoped>
.chat-message-list {
  width: min(920px, 100%);
  margin: 0 auto;
  padding: 28px 24px 80px;
}

.message {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  margin: 18px 0;
}

.message--user {
  justify-content: flex-end;
}

.message__content {
  min-width: 0;
  max-width: min(720px, 85%);
  overflow-wrap: anywhere;
}

.message--user .message__content {
  border-radius: 8px;
  background: rgba(var(--primary-color), 0.12);
  padding: 10px 14px;
}

.message--assistant .message__content {
  padding-top: 6px;
}

.message__user-avatar {
  width: 36px;
  height: 36px;
  flex: none;
  display: grid;
  place-items: center;
  border-radius: 50%;
  background: rgba(127, 127, 127, 0.16);
  color: rgba(127, 127, 127, 0.95);
  font-size: 22px;
}

.thinking-mascot {
  display: inline-flex;
  flex: none;
  animation: thinking-float 1.8s ease-in-out infinite;
}

@keyframes thinking-float {
  0%,
  100% {
    transform: translateY(0);
  }
  50% {
    transform: translateY(-3px);
  }
}

.thinking-bubble {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  border-radius: 999px;
  background: rgba(127, 127, 127, 0.1);
  padding: 13px 16px;
}

.thinking-bubble__dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: rgba(var(--primary-color), 0.75);
  animation: thinking-bounce 1.2s ease-in-out infinite;
}

.thinking-bubble__dot:nth-child(2) {
  animation-delay: 0.15s;
}

.thinking-bubble__dot:nth-child(3) {
  animation-delay: 0.3s;
}

@keyframes thinking-bounce {
  0%,
  60%,
  100% {
    transform: translateY(0);
    opacity: 0.45;
  }
  30% {
    transform: translateY(-5px);
    opacity: 1;
  }
}

@media (prefers-reduced-motion: reduce) {
  .thinking-mascot,
  .thinking-bubble__dot {
    animation: none;
  }
}

@media (max-width: 640px) {
  .chat-message-list {
    padding: 18px 14px 60px;
  }
  .message__content {
    max-width: calc(100% - 44px);
  }
}
</style>
