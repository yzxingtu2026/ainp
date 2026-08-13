<script setup lang="ts">
import { computed, onBeforeUnmount, reactive, ref, toRef, watch } from 'vue';
import { $t } from '@/locales';
import { useAttachmentAvatar } from '@/hooks/business/agent-avatar';
import { attachmentTypeIcon } from '@/hooks/business/attachment';
import { fetchAttachment } from '@/service/api';
import type { ToolOperation } from './tool-activity';

defineOptions({ name: 'ChatMessageList' });

interface ThinkingHint {
  icon: string;
  text: string;
  toolName?: string;
}

const props = withDefaults(
  defineProps<{
    messages: Api.AI.ConversationMessage[];
    /** 流式输出中的增量文本（会被追加进活跃 run 分组） */
    streamingText?: string;
    /** 运行中且尚无输出时展示思考占位 */
    thinking?: boolean;
    /** 步骤感知的思考占位提示（图标 + 文案 + 可选工具名） */
    thinkingHint?: ThinkingHint;
    /** 当前活跃 run ID；对应 run 分组被标记为实时（工具实时展示 + 追加流式文本） */
    activeRunId?: string;
    confirmations?: Api.AI.ToolConfirmation[];
    error?: string;
    userAvatarAttachmentId?: string | null;
    userName?: string;
  }>(),
  {
    streamingText: '',
    thinking: false,
    thinkingHint: () => ({ icon: 'mdi:brain-outline', text: '' }),
    activeRunId: '',
    confirmations: () => [],
    error: '',
    userAvatarAttachmentId: null,
    userName: ''
  }
);

const userAvatarAttachmentId = toRef(props, 'userAvatarAttachmentId');
const { url: userAvatarUrl } = useAttachmentAvatar(userAvatarAttachmentId);

const emit = defineEmits<{
  confirmationResolved: [status: 'approved' | 'rejected', result?: Api.AI.ToolResult];
}>();

/** thinkingHint 未提供或文案为空时的兜底提示 */
const hint = computed<ThinkingHint>(() => {
  const provided = props.thinkingHint;
  if (provided && provided.text) return provided;
  return { icon: 'mdi:brain-outline', text: $t('chat.thinkingThinking') };
});

interface RenderedUserItem {
  kind: 'user';
  key: string;
  text: string;
  imageCount: number;
  documents: Array<{ key: string; attachmentId: string; revision: string }>;
}
interface RenderedSystemItem {
  kind: 'system';
  key: string;
  text: string;
}
/** run 分组内的有序段：文本段或工具操作段，用于按会话位置穿插渲染 */
type RunSegment =
  | { kind: 'text'; key: string; text: string }
  | { kind: 'operations'; key: string; operations: ToolOperation[] };
interface RenderedRunItem {
  kind: 'run';
  key: string;
  runId: string;
  segments: RunSegment[];
  live: boolean;
}
type RenderedItem = RenderedUserItem | RenderedSystemItem | RenderedRunItem;

const documentMetadata = reactive<Record<string, { name: string; contentType: string; available: boolean }>>({});

function documentKey(attachmentId: string, revision: string) {
  return `${attachmentId}\u0000${revision}`;
}

async function loadDocumentMetadata(attachmentId: string, revision: string) {
  const key = documentKey(attachmentId, revision);
  if (documentMetadata[key]) return;
  documentMetadata[key] = { name: '', contentType: '', available: false };
  const { data, error } = await fetchAttachment(attachmentId, { errorMode: 'inline' });
  if (!error && data?.status === 'ready' && data.sha256 === revision) {
    documentMetadata[key] = { name: data.original_name, contentType: data.content_type, available: true };
  }
}

watch(
  () =>
    props.messages.flatMap(message =>
      (message.content || [])
        .filter(part => part.type === 'input_file' && part.file_attachment_id && part.attachment_revision)
        .map(part => `${part.file_attachment_id}\u0000${part.attachment_revision}`)
    ),
  keys => {
    for (const key of new Set(keys)) {
      const [attachmentId, revision] = key.split('\u0000');
      void loadDocumentMetadata(attachmentId, revision);
    }
  },
  { immediate: true }
);

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

/**
 * 按 run 分组渲染：同一次 run 的 assistant 消息归为一个消息块（共用一个头像）。
 * 运行中（live）按消息顺序穿插「文本段 / 工具操作段」，工具调用随会话位置实时展示；
 * 结束后（settled）合并为一个文本段 + 一个工具汇总组（「已处理 N 个操作」）。
 * user / system 消息单独渲染。
 */
const renderedItems = computed<RenderedItem[]>(() => {
  const items: RenderedItem[] = [];
  const runGroups = new Map<string, RenderedRunItem>();
  const resultIndex = toolResultIndex.value;

  for (const message of props.messages) {
    if (message.role === 'tool') continue;
    if (message.role === 'user') {
      items.push({
        kind: 'user',
        key: message.id,
        text: messageText(message),
        imageCount: (message.content || []).filter(part => part.type === 'input_image').length,
        documents: (message.content || [])
          .filter(part => part.type === 'input_file' && part.file_attachment_id && part.attachment_revision)
          .map(part => ({
            key: documentKey(part.file_attachment_id || '', part.attachment_revision || ''),
            attachmentId: part.file_attachment_id || '',
            revision: part.attachment_revision || ''
          }))
      });
      continue;
    }
    if (message.role === 'system') {
      items.push({ kind: 'system', key: message.id, text: messageText(message) });
      continue;
    }
    const runId = message.run_id || `message-${message.id}`;
    let group = runGroups.get(runId);
    if (!group) {
      group = {
        kind: 'run',
        key: runId,
        runId,
        segments: [],
        live: runId === props.activeRunId
      };
      runGroups.set(runId, group);
      items.push(group);
    }
    // 按消息内顺序追加段：先文本段，再该消息的工具操作段
    const text = messageText(message);
    if (text) group.segments.push({ kind: 'text', key: `${message.id}-text`, text });
    const operations: ToolOperation[] = [];
    for (const part of message.content || []) {
      if (part.type !== 'tool_call') continue;
      const callId = part.tool_call_id || '';
      operations.push({
        toolCallId: callId,
        toolName: part.tool_name || '',
        presentation: part.presentation,
        request: part.arguments,
        result: callId ? resultIndex.get(callId) : undefined,
        hasResult: Boolean(callId && resultIndex.has(callId))
      });
    }
    if (operations.length > 0) {
      group.segments.push({ kind: 'operations', key: `${message.id}-ops`, operations });
    }
  }

  // 逐个 run 分组收尾：活跃 run 保持穿插并追加流式文本；已结束 run 合并为文本段 + 工具汇总段
  for (const group of runGroups.values()) {
    if (group.live) {
      if (props.streamingText) {
        const last = group.segments[group.segments.length - 1];
        if (last && last.kind === 'text') {
          last.text = `${last.text}\n\n${props.streamingText}`;
        } else {
          group.segments.push({ kind: 'text', key: 'streaming', text: props.streamingText });
        }
      }
      continue;
    }
    const texts: string[] = [];
    const operations: ToolOperation[] = [];
    for (const segment of group.segments) {
      if (segment.kind === 'text') texts.push(segment.text);
      else operations.push(...segment.operations);
    }
    const merged: RunSegment[] = [];
    if (texts.length > 0) merged.push({ kind: 'text', key: `${group.runId}-text`, text: texts.join('\n\n') });
    if (operations.length > 0) merged.push({ kind: 'operations', key: `${group.runId}-ops`, operations });
    group.segments = merged;
  }

  // 活跃 run 尚无落库消息时，新建分组承载流式文本
  if (props.activeRunId && !runGroups.has(props.activeRunId)) {
    const activeGroup: RenderedRunItem = {
      kind: 'run',
      key: props.activeRunId,
      runId: props.activeRunId,
      segments: props.streamingText ? [{ kind: 'text', key: 'streaming', text: props.streamingText }] : [],
      live: true
    };
    runGroups.set(props.activeRunId, activeGroup);
    items.push(activeGroup);
  }

  return items;
});

/** 思考持续约 10 秒无步骤进展时，追加「耗时比预期稍久」提示 */
const slowHint = ref(false);
let slowTimer: number | null = null;
function resetSlowTimer() {
  slowHint.value = false;
  if (slowTimer !== null) {
    window.clearTimeout(slowTimer);
    slowTimer = null;
  }
  if (props.thinking) {
    slowTimer = window.setTimeout(() => {
      slowHint.value = true;
    }, 10000);
  }
}
watch(() => props.thinking, resetSlowTimer, { immediate: true });
watch(
  () => `${hint.value.text}|${hint.value.toolName || ''}`,
  () => {
    if (props.thinking) resetSlowTimer();
  }
);
onBeforeUnmount(() => {
  if (slowTimer !== null) window.clearTimeout(slowTimer);
});

function onConfirmationResolved(status: 'approved' | 'rejected', result?: Api.AI.ToolResult) {
  emit('confirmationResolved', status, result);
}
</script>

<template>
  <div class="chat-message-list">
    <template v-for="item in renderedItems" :key="item.key">
      <article v-if="item.kind === 'user'" class="message message--user">
        <div class="message__content">
          <MarkdownRenderer v-if="item.text" :content="item.text" />
          <NTag v-for="index in item.imageCount" :key="`image-${index}`" size="small" class="mt-6px">
            <template #icon><SvgIcon icon="mdi:image-outline" /></template>
            {{ $t('chat.imageAttachment') }}
          </NTag>
          <NTag v-for="document in item.documents" :key="document.key" size="small" class="mt-6px mr-6px">
            <template #icon>
              <SvgIcon
                :icon="
                  documentMetadata[document.key]?.available
                    ? attachmentTypeIcon(documentMetadata[document.key].contentType)
                    : 'mdi:file-alert-outline'
                "
              />
            </template>
            {{
              documentMetadata[document.key]?.available
                ? documentMetadata[document.key].name
                : $t('page.aiWorkspace.documentUnavailable')
            }}
          </NTag>
        </div>
        <span class="message__user-avatar" role="img" :aria-label="userName || '当前用户头像'">
          <img v-if="userAvatarUrl" :src="userAvatarUrl" :alt="userName" />
          <SvgIcon v-else icon="mdi:account" />
        </span>
      </article>

      <article v-else-if="item.kind === 'system'" class="message message--system">
        <div class="message__content">
          <MarkdownRenderer v-if="item.text" :content="item.text" />
        </div>
      </article>

      <article v-else-if="item.segments.length > 0" class="message message--assistant">
        <AiMascot :state="item.live ? 'working' : 'idle'" :size="36" :breathe="false" />
        <div class="message__content">
          <template v-for="segment in item.segments" :key="segment.key">
            <MarkdownRenderer v-if="segment.kind === 'text'" :content="segment.text" />
            <ToolActivityGroup v-else :operations="segment.operations" :active="item.live" />
          </template>
        </div>
      </article>
    </template>

    <article v-if="thinking" class="message message--assistant">
      <span class="thinking-mascot">
        <AiMascot state="thinking" :size="36" :breathe="false" />
      </span>
      <div class="message__content">
        <div class="thinking-status" role="status" :aria-label="hint.text">
          <span class="thinking-status__icon" aria-hidden="true">
            <SvgIcon :icon="hint.icon" />
          </span>
          <Transition name="thinking-swap" mode="out-in">
            <span :key="hint.text + (hint.toolName || '')" class="thinking-status__label">
              {{ hint.text }}
              <code v-if="hint.toolName" class="thinking-status__tool">{{ hint.toolName }}</code>
            </span>
          </Transition>
          <span class="thinking-status__dots" aria-hidden="true">
            <i class="thinking-status__dot"></i>
            <i class="thinking-status__dot"></i>
            <i class="thinking-status__dot"></i>
          </span>
        </div>
        <Transition name="thinking-slow">
          <p v-if="slowHint" class="thinking-slow-hint">{{ $t('chat.thinkingSlow') }}</p>
        </Transition>
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
  overflow: hidden;
  border-radius: 50%;
  background: rgba(127, 127, 127, 0.16);
  color: rgba(127, 127, 127, 0.95);
  font-size: 22px;
}

.message__user-avatar img {
  width: 100%;
  height: 100%;
  object-fit: cover;
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

/* 步骤感知思考占位：图标 + 动态文案 + 跳动圆点，胶囊背景叠加缓慢流动微光 */
.thinking-status {
  position: relative;
  display: inline-flex;
  align-items: center;
  gap: 8px;
  border-radius: 999px;
  background: rgba(127, 127, 127, 0.1);
  padding: 6px 12px;
  overflow: hidden;
}

.thinking-status::after {
  content: '';
  position: absolute;
  inset: 0;
  background: linear-gradient(100deg, transparent 25%, rgba(var(--primary-color), 0.09) 50%, transparent 75%);
  background-size: 220% 100%;
  animation: thinking-shimmer 2.4s linear infinite;
  pointer-events: none;
}

@keyframes thinking-shimmer {
  0% {
    background-position: 130% 0;
  }
  100% {
    background-position: -130% 0;
  }
}

.thinking-status__icon {
  display: inline-flex;
  flex: none;
  color: rgba(var(--primary-color), 0.9);
  font-size: 14px;
  animation: thinking-pulse 1.6s ease-in-out infinite;
}

@keyframes thinking-pulse {
  0%,
  100% {
    opacity: 0.55;
    transform: scale(1);
  }
  50% {
    opacity: 1;
    transform: scale(1.12);
  }
}

.thinking-status__label {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12.5px;
  font-weight: 500;
  color: rgba(var(--base-text-color), 0.85);
  white-space: nowrap;
}

.thinking-status__tool {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 11px;
  line-height: 1;
  padding: 2px 6px;
  border-radius: 5px;
  background: rgba(var(--primary-color), 0.12);
  color: rgba(var(--primary-color), 1);
  max-width: 220px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.thinking-status__dots {
  display: inline-flex;
  align-items: center;
  gap: 3px;
  flex: none;
}

.thinking-status__dot {
  width: 5px;
  height: 5px;
  border-radius: 50%;
  background: rgba(var(--primary-color), 0.75);
  animation: thinking-bounce 1.2s ease-in-out infinite;
}

.thinking-status__dot:nth-child(2) {
  animation-delay: 0.15s;
}

.thinking-status__dot:nth-child(3) {
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
    transform: translateY(-3px);
    opacity: 1;
  }
}

/* 步骤切换时文案淡入上移过渡 */
.thinking-swap-enter-active,
.thinking-swap-leave-active {
  transition:
    opacity 0.22s ease,
    transform 0.22s ease;
}

.thinking-swap-enter-from {
  opacity: 0;
  transform: translateY(5px);
}

.thinking-swap-leave-to {
  opacity: 0;
  transform: translateY(-5px);
}

.thinking-slow-hint {
  margin: 8px 2px 0;
  font-size: 12px;
  color: rgba(127, 127, 127, 0.85);
}

.thinking-slow-enter-active {
  transition: opacity 0.45s ease;
}

.thinking-slow-enter-from {
  opacity: 0;
}

@media (prefers-reduced-motion: reduce) {
  .thinking-mascot,
  .thinking-status__dot,
  .thinking-status__icon,
  .thinking-status::after {
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
