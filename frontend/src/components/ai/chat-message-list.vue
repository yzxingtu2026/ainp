<script setup lang="ts">
import { computed, onBeforeUnmount, reactive, ref, toRef, watch } from 'vue';
import { $t } from '@/locales';
import { useAttachmentAvatar } from '@/hooks/business/agent-avatar';
import {
  attachmentTypeIcon,
  canPreviewAttachment,
  formatFileSize,
  isImageContentType,
  useAttachmentGridThumbs
} from '@/hooks/business/attachment';
import { useAttachmentThumbStatusPolling } from '@/hooks/business/attachment-thumb-poll';
import AttachmentPreview from '@/components/common/attachment-preview.vue';
import { fetchAttachment } from '@/service/api';
import { attachmentExtBadge, workspaceDocumentIcon } from '@/views/ai/workspace/modules/document-attachment';
import { copyPlainText } from './clipboard';
import { collapseRetriedToolOperations, type ToolOperation } from './tool-activity';

defineOptions({ name: 'ChatMessageList' });

interface ThinkingHint {
  icon: string;
  text: string;
  toolName?: string;
}

interface AttachmentRef {
  key: string;
  attachmentId: string;
  revision: string;
}

interface AttachmentMetaEntry {
  state: 'loading' | 'available' | 'unavailable';
  item: Api.Attachment.Item | null;
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
    agentName?: string;
    failedRunId?: string;
    canRetryFailedRun?: boolean;
    retrying?: boolean;
  }>(),
  {
    streamingText: '',
    thinking: false,
    thinkingHint: () => ({ icon: 'mdi:brain-outline', text: '' }),
    activeRunId: '',
    confirmations: () => [],
    error: '',
    userAvatarAttachmentId: null,
    userName: '',
    agentName: '',
    failedRunId: '',
    canRetryFailedRun: false,
    retrying: false
  }
);

const userAvatarAttachmentId = toRef(props, 'userAvatarAttachmentId');
const { url: userAvatarUrl } = useAttachmentAvatar(userAvatarAttachmentId);

const emit = defineEmits<{
  confirmationResolved: [status: 'approved' | 'rejected', result?: Api.AI.ToolResult];
  openRun: [runId: string];
  retryRun: [runId: string];
}>();

const resolvedAgentName = computed(() => props.agentName || $t('page.aiWorkspace.assistant'));

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
  images: AttachmentRef[];
  documents: AttachmentRef[];
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
  fullText: string;
  lastTextKey: string;
  hasCanonicalRun: boolean;
}
type RenderedItem = RenderedUserItem | RenderedSystemItem | RenderedRunItem;

const attachmentMeta = reactive<Record<string, AttachmentMetaEntry>>({});
const resolvedImageItems = ref<Api.Attachment.Item[]>([]);
const resolvedImageById = computed<Record<string, Api.Attachment.Item>>(() =>
  Object.fromEntries(resolvedImageItems.value.map(item => [item.id, item]))
);
const thumbsEnabled = computed(() => resolvedImageItems.value.length > 0);
const thumbs = useAttachmentGridThumbs(resolvedImageItems, { enabled: thumbsEnabled });
useAttachmentThumbStatusPolling(resolvedImageItems, { enabled: thumbsEnabled, maxItems: 48 });

const previewVisible = ref(false);
const previewTarget = ref<Api.Attachment.Item | null>(null);

const copiedKeys = reactive<Record<string, boolean>>({});
const copyTimers = new Map<string, number>();

function attachmentKey(attachmentId: string, revision: string) {
  return `${attachmentId}\u0000${revision}`;
}

function upsertResolvedImage(item: Api.Attachment.Item) {
  if (!isImageContentType(item.content_type)) return;
  const existing = resolvedImageItems.value.find(row => row.id === item.id);
  if (!existing) {
    resolvedImageItems.value = [...resolvedImageItems.value, item];
    return;
  }
  const mergedThumbStatus = existing.thumb_status === 'ready' ? 'ready' : item.thumb_status;
  resolvedImageItems.value = resolvedImageItems.value.map(row =>
    row.id === item.id ? { ...item, thumb_status: mergedThumbStatus } : row
  );
}

async function loadAttachmentMetadata(attachmentId: string, revision: string) {
  const key = attachmentKey(attachmentId, revision);
  if (attachmentMeta[key]) return;
  if (!attachmentId || !revision) {
    attachmentMeta[key] = { state: 'unavailable', item: null };
    return;
  }
  attachmentMeta[key] = { state: 'loading', item: null };
  const { data, error } = await fetchAttachment(attachmentId, { errorMode: 'inline' });
  if (!error && data?.status === 'ready' && data.sha256 === revision) {
    attachmentMeta[key] = { state: 'available', item: data };
    upsertResolvedImage(data);
    return;
  }
  attachmentMeta[key] = { state: 'unavailable', item: null };
}

watch(
  () =>
    props.messages.flatMap(message =>
      (message.content || []).flatMap(part => {
        if (part.type === 'input_image') {
          return [`${part.image_attachment_id || ''}\u0000${part.attachment_revision || ''}`];
        }
        if (part.type === 'input_file') {
          return [`${part.file_attachment_id || ''}\u0000${part.attachment_revision || ''}`];
        }
        return [];
      })
    ),
  keys => {
    const activeImageIds = new Set<string>();
    for (const key of new Set(keys)) {
      const separatorIndex = key.indexOf('\u0000');
      const attachmentId = separatorIndex >= 0 ? key.slice(0, separatorIndex) : key;
      const revision = separatorIndex >= 0 ? key.slice(separatorIndex + 1) : '';
      if (attachmentId) activeImageIds.add(attachmentId);
      void loadAttachmentMetadata(attachmentId, revision);
    }
    if (resolvedImageItems.value.some(item => !activeImageIds.has(item.id))) {
      resolvedImageItems.value = resolvedImageItems.value.filter(item => activeImageIds.has(item.id));
    }
  },
  { immediate: true }
);

function canOpenPreview(entry?: AttachmentMetaEntry): boolean {
  return Boolean(entry?.state === 'available' && entry.item && canPreviewAttachment(entry.item));
}

function openAttachmentPreview(entry?: AttachmentMetaEntry) {
  if (!entry?.item || entry.state !== 'available' || !canPreviewAttachment(entry.item)) return;
  previewTarget.value = entry.item;
  previewVisible.value = true;
}

function imageItemFor(entry?: AttachmentMetaEntry): Api.Attachment.Item | null {
  if (!entry?.item || entry.state !== 'available') return null;
  return resolvedImageById.value[entry.item.id] || entry.item;
}

function documentIconFor(entry?: AttachmentMetaEntry): string {
  if (!entry || entry.state !== 'available' || !entry.item) return 'mdi:file-alert-outline';
  return (
    workspaceDocumentIcon(entry.item.content_type, entry.item.original_name) ||
    attachmentTypeIcon(entry.item.content_type)
  );
}

async function copyTurnText(key: string, text: string) {
  if (!text) return;
  const ok = await copyPlainText(text);
  if (!ok) return;
  copiedKeys[key] = true;
  const existing = copyTimers.get(key);
  if (existing !== undefined) window.clearTimeout(existing);
  const timer = window.setTimeout(() => {
    copiedKeys[key] = false;
    copyTimers.delete(key);
  }, 1800);
  copyTimers.set(key, timer);
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

function finalizeRunGroupMeta(group: RenderedRunItem) {
  const texts: string[] = [];
  let lastTextKey = '';
  for (const segment of group.segments) {
    if (segment.kind === 'text') {
      texts.push(segment.text);
      lastTextKey = segment.key;
    }
  }
  group.fullText = texts.join('\n\n');
  group.lastTextKey = lastTextKey;
}

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
      const contentParts = message.content || [];
      items.push({
        kind: 'user',
        key: message.id,
        text: messageText(message),
        images: contentParts
          .filter(part => part.type === 'input_image')
          .map((part, index) => {
            const attachmentId = part.image_attachment_id || '';
            const revision = part.attachment_revision || '';
            return {
              key:
                attachmentId && revision ? attachmentKey(attachmentId, revision) : `missing-img-${message.id}-${index}`,
              attachmentId,
              revision
            };
          }),
        documents: contentParts
          .filter(part => part.type === 'input_file')
          .map((part, index) => {
            const attachmentId = part.file_attachment_id || '';
            const revision = part.attachment_revision || '';
            return {
              key:
                attachmentId && revision ? attachmentKey(attachmentId, revision) : `missing-doc-${message.id}-${index}`,
              attachmentId,
              revision
            };
          })
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
        live: runId === props.activeRunId,
        fullText: '',
        lastTextKey: '',
        hasCanonicalRun: Boolean(message.run_id)
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
        retryOfToolCallId: part.retry_of_tool_call_id,
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
    const operationSegments = group.segments.filter(segment => segment.kind === 'operations');
    const collapsedOperations = collapseRetriedToolOperations(operationSegments.flatMap(segment => segment.operations));
    const collapsedByCallId = new Map(collapsedOperations.map(operation => [operation.toolCallId, operation] as const));
    for (const segment of operationSegments) {
      segment.operations = segment.operations
        .filter(operation => collapsedByCallId.has(operation.toolCallId))
        .map(operation => collapsedByCallId.get(operation.toolCallId) || operation);
    }
    group.segments = group.segments.filter(segment => segment.kind !== 'operations' || segment.operations.length > 0);
    if (group.live) {
      if (props.streamingText) {
        const last = group.segments[group.segments.length - 1];
        if (last && last.kind === 'text') {
          last.text = `${last.text}\n\n${props.streamingText}`;
        } else {
          group.segments.push({ kind: 'text', key: 'streaming', text: props.streamingText });
        }
      }
      finalizeRunGroupMeta(group);
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
    finalizeRunGroupMeta(group);
  }

  // 活跃 run 尚无落库消息时，新建分组承载流式文本
  if (props.activeRunId && !runGroups.has(props.activeRunId)) {
    const activeGroup: RenderedRunItem = {
      kind: 'run',
      key: props.activeRunId,
      runId: props.activeRunId,
      segments: props.streamingText ? [{ kind: 'text', key: 'streaming', text: props.streamingText }] : [],
      live: true,
      fullText: props.streamingText || '',
      lastTextKey: props.streamingText ? 'streaming' : '',
      hasCanonicalRun: true
    };
    runGroups.set(props.activeRunId, activeGroup);
    items.push(activeGroup);
  }

  return items;
});

const renderedVisibleRunIds = computed(
  () =>
    new Set(
      renderedItems.value
        .filter((item): item is RenderedRunItem => item.kind === 'run' && item.segments.length > 0)
        .map(item => item.runId)
    )
);

const showBannerRetry = computed(() =>
  Boolean(props.canRetryFailedRun && props.failedRunId && !renderedVisibleRunIds.value.has(props.failedRunId))
);

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
  for (const timer of copyTimers.values()) {
    window.clearTimeout(timer);
  }
  copyTimers.clear();
});

function onConfirmationResolved(status: 'approved' | 'rejected', result?: Api.AI.ToolResult) {
  emit('confirmationResolved', status, result);
}
</script>

<template>
  <div class="chat-message-list">
    <template v-for="item in renderedItems" :key="item.key">
      <article v-if="item.kind === 'user'" class="message message--user">
        <div class="message__user-stack">
          <div class="message__user-bubble">
            <MarkdownRenderer v-if="item.text" :content="item.text" />

            <div v-if="item.images.length > 0" class="user-attachments user-attachments--images">
              <template v-for="(img, index) in item.images" :key="`${img.key}-${index}`">
                <button
                  v-if="canOpenPreview(attachmentMeta[img.key])"
                  type="button"
                  class="user-image-card"
                  :aria-label="`${$t('page.aiWorkspace.previewAttachment')}：${imageItemFor(attachmentMeta[img.key])?.original_name || $t('chat.imageAttachment')}`"
                  :title="imageItemFor(attachmentMeta[img.key])?.original_name || $t('chat.imageAttachment')"
                  @click="openAttachmentPreview(attachmentMeta[img.key])"
                >
                  <img
                    v-if="
                      imageItemFor(attachmentMeta[img.key]) && thumbs.urls[imageItemFor(attachmentMeta[img.key])!.id]
                    "
                    :src="thumbs.urls[imageItemFor(attachmentMeta[img.key])!.id]"
                    :alt="imageItemFor(attachmentMeta[img.key])?.original_name || $t('chat.imageAttachment')"
                    class="user-image-card__img"
                  />
                  <span
                    v-else-if="imageItemFor(attachmentMeta[img.key])?.thumb_status === 'pending'"
                    class="user-image-card__placeholder"
                  >
                    <NSpin :size="16" />
                  </span>
                  <span v-else class="user-image-card__placeholder">
                    <SvgIcon icon="mdi:image-outline" class="text-20px" />
                    <span class="user-image-card__name">
                      {{ imageItemFor(attachmentMeta[img.key])?.original_name || $t('chat.imageAttachment') }}
                    </span>
                  </span>
                  <span class="user-image-card__zoom" aria-hidden="true">
                    <SvgIcon icon="mdi:eye-outline" />
                  </span>
                </button>

                <div
                  v-else-if="!attachmentMeta[img.key] || attachmentMeta[img.key]?.state === 'loading'"
                  class="user-image-card user-image-card--loading"
                >
                  <NSpin :size="16" />
                </div>

                <div
                  v-else
                  class="user-image-card user-image-card--unavailable"
                  :title="$t('page.aiWorkspace.imageUnavailable')"
                >
                  <SvgIcon icon="mdi:image-off-outline" class="text-18px" />
                  <span class="user-image-card__unavailable-text">{{ $t('page.aiWorkspace.imageUnavailable') }}</span>
                </div>
              </template>
            </div>

            <div v-if="item.documents.length > 0" class="user-attachments user-attachments--docs">
              <template v-for="(doc, index) in item.documents" :key="`${doc.key}-${index}`">
                <button
                  v-if="canOpenPreview(attachmentMeta[doc.key])"
                  type="button"
                  class="user-doc-chip user-doc-chip--interactive"
                  :aria-label="`${$t('page.aiWorkspace.previewAttachment')}：${attachmentMeta[doc.key]?.item?.original_name || ''}`"
                  :title="attachmentMeta[doc.key]?.item?.original_name"
                  @click="openAttachmentPreview(attachmentMeta[doc.key])"
                >
                  <span class="user-doc-chip__icon">
                    <SvgIcon :icon="documentIconFor(attachmentMeta[doc.key])" />
                  </span>
                  <span class="user-doc-chip__meta">
                    <span class="user-doc-chip__name">{{ attachmentMeta[doc.key]?.item?.original_name }}</span>
                    <span class="user-doc-chip__sub">
                      <span>
                        {{
                          attachmentExtBadge(
                            attachmentMeta[doc.key]?.item?.original_name || '',
                            attachmentMeta[doc.key]?.item?.content_type
                          )
                        }}
                      </span>
                      <span>·</span>
                      <span>{{ formatFileSize(attachmentMeta[doc.key]?.item?.size || 0) }}</span>
                    </span>
                  </span>
                  <SvgIcon icon="mdi:eye-outline" class="user-doc-chip__preview" aria-hidden="true" />
                </button>

                <div
                  v-else-if="attachmentMeta[doc.key]?.state === 'available' && attachmentMeta[doc.key]?.item"
                  class="user-doc-chip"
                  :title="attachmentMeta[doc.key]?.item?.original_name"
                >
                  <span class="user-doc-chip__icon">
                    <SvgIcon :icon="documentIconFor(attachmentMeta[doc.key])" />
                  </span>
                  <span class="user-doc-chip__meta">
                    <span class="user-doc-chip__name">{{ attachmentMeta[doc.key]?.item?.original_name }}</span>
                    <span class="user-doc-chip__sub">
                      <span>
                        {{
                          attachmentExtBadge(
                            attachmentMeta[doc.key]?.item?.original_name || '',
                            attachmentMeta[doc.key]?.item?.content_type
                          )
                        }}
                      </span>
                      <span>·</span>
                      <span>{{ formatFileSize(attachmentMeta[doc.key]?.item?.size || 0) }}</span>
                    </span>
                  </span>
                </div>

                <div
                  v-else-if="!attachmentMeta[doc.key] || attachmentMeta[doc.key]?.state === 'loading'"
                  class="user-doc-chip user-doc-chip--loading"
                >
                  <NSpin :size="14" />
                  <span class="user-doc-chip__name">{{ $t('chat.documentAttachment') }}</span>
                </div>

                <div
                  v-else
                  class="user-doc-chip user-doc-chip--unavailable"
                  :title="$t('page.aiWorkspace.documentUnavailable')"
                >
                  <span class="user-doc-chip__icon user-doc-chip__icon--unavailable">
                    <SvgIcon icon="mdi:file-alert-outline" />
                  </span>
                  <span class="user-doc-chip__name">{{ $t('page.aiWorkspace.documentUnavailable') }}</span>
                </div>
              </template>
            </div>
          </div>

          <div v-if="item.text" class="message__turn-actions message__turn-actions--user">
            <button
              type="button"
              class="turn-action-btn"
              :class="{ 'turn-action-btn--active': copiedKeys[`user-${item.key}`] }"
              :aria-label="$t('chat.copyMessage')"
              :title="copiedKeys[`user-${item.key}`] ? $t('chat.copied') : $t('chat.copyMessage')"
              @click="copyTurnText(`user-${item.key}`, item.text)"
            >
              <SvgIcon :icon="copiedKeys[`user-${item.key}`] ? 'mdi:check' : 'mdi:content-copy'" />
              <span>{{ copiedKeys[`user-${item.key}`] ? $t('chat.copied') : $t('chat.copy') }}</span>
            </button>
          </div>
        </div>

        <span class="message__user-avatar" role="img" :aria-label="userName || '当前用户头像'">
          <img v-if="userAvatarUrl" :src="userAvatarUrl" :alt="userName" />
          <SvgIcon v-else icon="mdi:account" />
        </span>
      </article>

      <article v-else-if="item.kind === 'system'" class="message message--system">
        <div class="message__content message__system-banner">
          <MarkdownRenderer v-if="item.text" :content="item.text" />
        </div>
      </article>

      <article v-else-if="item.segments.length > 0" class="message message--assistant">
        <span class="message__assistant-avatar" :class="{ 'message__assistant-avatar--live': item.live }">
          <AiMascot :state="item.live ? 'working' : 'idle'" :size="34" :breathe="false" />
        </span>
        <div class="message__content assistant-panel" :class="{ 'assistant-panel--live': item.live }">
          <header class="assistant-panel__header">
            <span class="assistant-panel__role">{{ resolvedAgentName }}</span>
            <span v-if="item.live" class="assistant-panel__live">
              <i aria-hidden="true" />
              {{ $t('chat.streamingLive') }}
            </span>
          </header>

          <div class="assistant-panel__body">
            <template v-for="segment in item.segments" :key="segment.key">
              <MarkdownRenderer
                v-if="segment.kind === 'text'"
                :content="segment.text"
                :streaming="item.live && Boolean(streamingText) && segment.key === item.lastTextKey"
              />
              <ToolActivityGroup v-else :operations="segment.operations" :active="item.live" />
            </template>
          </div>

          <footer
            v-if="
              !item.live && (item.fullText || item.hasCanonicalRun || (canRetryFailedRun && failedRunId === item.runId))
            "
            class="message__turn-actions assistant-panel__footer"
          >
            <button
              v-if="item.fullText"
              type="button"
              class="turn-action-btn"
              :class="{ 'turn-action-btn--active': copiedKeys[`run-${item.key}`] }"
              :aria-label="$t('chat.copyResponse')"
              @click="copyTurnText(`run-${item.key}`, item.fullText)"
            >
              <SvgIcon :icon="copiedKeys[`run-${item.key}`] ? 'mdi:check' : 'mdi:content-copy'" />
              <span>{{ copiedKeys[`run-${item.key}`] ? $t('chat.copied') : $t('chat.copyResponse') }}</span>
            </button>

            <button
              v-if="item.hasCanonicalRun"
              type="button"
              class="turn-action-btn"
              :aria-label="$t('chat.viewRunDetails')"
              @click="emit('openRun', item.runId)"
            >
              <SvgIcon icon="mdi:timeline-clock-outline" />
              <span>{{ $t('chat.viewRunDetails') }}</span>
            </button>

            <button
              v-if="canRetryFailedRun && failedRunId === item.runId"
              type="button"
              class="turn-action-btn turn-action-btn--primary"
              :disabled="retrying"
              :aria-label="$t('chat.retryRun')"
              @click="emit('retryRun', item.runId)"
            >
              <NSpin v-if="retrying" :size="12" />
              <SvgIcon v-else icon="mdi:refresh" />
              <span>{{ $t('chat.retryRun') }}</span>
            </button>
          </footer>
        </div>
      </article>
    </template>

    <article v-if="thinking" class="message message--assistant">
      <span class="thinking-mascot message__assistant-avatar message__assistant-avatar--live">
        <AiMascot state="thinking" :size="34" :breathe="false" />
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
      <div class="chat-error-banner">
        <span>{{ error }}</span>
        <NButton
          v-if="showBannerRetry"
          size="tiny"
          type="error"
          secondary
          :loading="retrying"
          :disabled="retrying"
          @click="emit('retryRun', failedRunId)"
        >
          <template #icon><SvgIcon icon="mdi:refresh" /></template>
          {{ $t('chat.retryRun') }}
        </NButton>
      </div>
    </NAlert>

    <AttachmentPreview v-if="previewVisible" v-model:show="previewVisible" :attachment="previewTarget" />
  </div>
</template>

<style scoped>
.chat-message-list {
  width: min(920px, 100%);
  margin: 0 auto;
  padding: 24px 20px 76px;
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

.message--system {
  justify-content: center;
}

.message__user-stack {
  min-width: 0;
  max-width: min(700px, calc(100% - 46px));
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 4px;
}

.message__content {
  min-width: 0;
  max-width: min(760px, calc(100% - 46px));
  overflow-wrap: break-word;
}

/* Asymmetric precision user capsule bubble */
.message__user-bubble {
  width: fit-content;
  max-width: 100%;
  word-break: normal;
  overflow-wrap: break-word;
  border: 1px solid rgb(var(--primary-color) / 0.24);
  border-radius: 18px 18px 4px 18px;
  background: linear-gradient(135deg, rgb(var(--primary-color) / 0.12) 0%, rgba(8, 145, 178, 0.07) 100%);
  box-shadow:
    0 4px 14px rgb(var(--primary-color) / 0.06),
    inset 0 1px 0 rgba(255, 255, 255, 0.55);
  padding: 10px 15px;
}

:where(.dark) .message__user-bubble {
  border-color: rgb(var(--primary-color) / 0.36);
  background: linear-gradient(135deg, rgb(var(--primary-color) / 0.24) 0%, rgba(34, 211, 238, 0.12) 100%);
  box-shadow:
    0 6px 18px rgba(0, 0, 0, 0.22),
    inset 0 1px 0 rgba(255, 255, 255, 0.08);
}

.message__system-banner {
  border: 1px dashed rgba(127, 127, 127, 0.24);
  border-radius: 10px;
  background: rgba(127, 127, 127, 0.05);
  padding: 6px 12px;
  color: rgba(127, 127, 127, 0.95);
  font-size: 12.5px;
}

/* Multimodal user attachments */
.user-attachments {
  margin-top: 8px;
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.message__user-bubble > .user-attachments:first-child {
  margin-top: 0;
}

.user-image-card {
  position: relative;
  width: 96px;
  height: 76px;
  flex: none;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  overflow: hidden;
  border: 1px solid rgba(127, 127, 127, 0.24);
  border-radius: 10px;
  background: rgb(var(--container-bg-color) / 88%);
  padding: 0;
  color: inherit;
  cursor: pointer;
  transition:
    border-color 0.18s ease,
    transform 0.18s ease,
    box-shadow 0.18s ease;
}

.user-image-card:hover,
.user-image-card:focus-visible {
  border-color: rgb(var(--primary-color) / 0.55);
  box-shadow: 0 4px 12px rgb(var(--primary-color) / 0.16);
  transform: translateY(-1px);
  outline: none;
}

.user-image-card__img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.user-image-card__placeholder {
  width: 100%;
  height: 100%;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 4px;
  padding: 6px;
  color: rgba(127, 127, 127, 0.9);
}

.user-image-card__name {
  max-width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 10px;
}

.user-image-card__zoom {
  position: absolute;
  right: 5px;
  bottom: 5px;
  width: 22px;
  height: 22px;
  display: grid;
  place-items: center;
  border-radius: 6px;
  background: rgba(15, 23, 42, 0.66);
  color: #fff;
  font-size: 13px;
  opacity: 0;
  transition: opacity 0.16s ease;
}

.user-image-card:hover .user-image-card__zoom,
.user-image-card:focus-visible .user-image-card__zoom {
  opacity: 1;
}

.user-image-card--loading,
.user-image-card--unavailable {
  cursor: default;
  padding: 6px;
  text-align: center;
}

.user-image-card--unavailable {
  width: auto;
  min-width: 112px;
  max-width: 196px;
  height: auto;
  min-height: 44px;
  flex-direction: row;
  gap: 6px;
  padding: 6px 10px;
  border-style: dashed;
  color: rgba(127, 127, 127, 0.92);
}

.user-image-card--unavailable:hover {
  border-color: rgba(127, 127, 127, 0.24);
  box-shadow: none;
  transform: none;
}

.user-image-card__unavailable-text {
  font-size: 11.5px;
  line-height: 1.35;
  text-align: left;
}

.user-doc-chip {
  min-width: 0;
  max-width: min(240px, 100%);
  display: inline-flex;
  align-items: center;
  gap: 8px;
  border: 1px solid rgba(127, 127, 127, 0.22);
  border-radius: 10px;
  background: rgb(var(--container-bg-color) / 88%);
  padding: 6px 10px;
  color: inherit;
  font: inherit;
  text-align: left;
}

.user-doc-chip--interactive {
  cursor: pointer;
  transition:
    border-color 0.18s ease,
    box-shadow 0.18s ease;
}

.user-doc-chip--interactive:hover,
.user-doc-chip--interactive:focus-visible {
  border-color: rgb(var(--primary-color) / 0.52);
  box-shadow: 0 3px 10px rgb(var(--primary-color) / 0.12);
  outline: none;
}

.user-doc-chip__icon {
  width: 28px;
  height: 28px;
  flex: none;
  display: grid;
  place-items: center;
  border-radius: 7px;
  background: rgb(var(--primary-color) / 0.1);
  color: rgb(var(--primary-color));
  font-size: 17px;
}

.user-doc-chip__icon--unavailable {
  background: rgba(127, 127, 127, 0.12);
  color: rgba(127, 127, 127, 0.9);
}

.user-doc-chip__meta {
  min-width: 0;
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 1px;
}

.user-doc-chip__name {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 12px;
  font-weight: 580;
}

.user-doc-chip__sub {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  color: rgba(127, 127, 127, 0.9);
  font-size: 10.5px;
  font-variant-numeric: tabular-nums;
}

.user-doc-chip__preview {
  flex: none;
  color: rgb(var(--primary-color) / 0.85);
  font-size: 15px;
}

.user-doc-chip--unavailable {
  border-style: dashed;
  color: rgba(127, 127, 127, 0.92);
}

.user-doc-chip--loading {
  color: rgba(127, 127, 127, 0.9);
}

/* Assistant lightweight precision container */
.message__assistant-avatar {
  width: 36px;
  height: 36px;
  flex: none;
  display: grid;
  place-items: center;
  border: 1px solid rgba(127, 127, 127, 0.16);
  border-radius: 11px;
  background: rgb(var(--container-bg-color));
}

.message__assistant-avatar--live {
  border-color: rgb(var(--primary-color) / 0.38);
  box-shadow: 0 0 12px rgb(var(--primary-color) / 0.14);
}

.assistant-panel {
  flex: 1;
  border: 1px solid rgba(127, 127, 127, 0.15);
  border-radius: 14px;
  background: rgba(127, 127, 127, 0.032);
  padding: 10px 14px 11px;
  transition:
    border-color 0.2s ease,
    box-shadow 0.2s ease;
}

:where(.dark) .assistant-panel {
  border-color: rgba(148, 163, 184, 0.14);
  background: rgba(255, 255, 255, 0.024);
}

.assistant-panel:hover {
  border-color: rgba(127, 127, 127, 0.24);
}

.assistant-panel--live {
  border-color: rgb(var(--primary-color) / 0.32);
  box-shadow: 0 4px 20px rgb(var(--primary-color) / 0.06);
}

.assistant-panel__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 6px;
}

.assistant-panel__role {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: rgb(var(--base-text-color) / 0.72);
  font-size: 11.5px;
  font-weight: 620;
  letter-spacing: 0.01em;
}

.assistant-panel__live {
  flex: none;
  display: inline-flex;
  align-items: center;
  gap: 5px;
  border-radius: 999px;
  background: rgb(var(--primary-color) / 0.1);
  padding: 2px 8px;
  color: rgb(var(--primary-color));
  font-size: 10.5px;
  font-weight: 600;
}

.assistant-panel__live i {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: linear-gradient(135deg, #646cff, #22d3ee);
  animation: assistant-live-pulse 1.4s ease-in-out infinite;
}

@keyframes assistant-live-pulse {
  0%,
  100% {
    opacity: 1;
    transform: scale(1);
  }
  50% {
    opacity: 0.38;
    transform: scale(0.84);
  }
}

.assistant-panel__body {
  min-width: 0;
}

/* Turn action bar */
.message__turn-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px;
  opacity: 0.52;
  transition: opacity 0.18s ease;
}

.assistant-panel__footer {
  margin-top: 9px;
  padding-top: 7px;
  border-top: 1px solid rgba(127, 127, 127, 0.12);
}

.message__turn-actions--user {
  opacity: 0;
}

.message:hover .message__turn-actions,
.message:focus-within .message__turn-actions {
  opacity: 1;
}

.turn-action-btn {
  height: 24px;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  border: 1px solid transparent;
  border-radius: 6px;
  background: transparent;
  padding: 0 7px;
  color: rgba(127, 127, 127, 0.95);
  font-size: 11.5px;
  font-weight: 500;
  cursor: pointer;
  transition:
    border-color 0.16s ease,
    background 0.16s ease,
    color 0.16s ease;
}

.turn-action-btn:hover:not(:disabled),
.turn-action-btn:focus-visible {
  border-color: rgba(127, 127, 127, 0.2);
  background: rgba(127, 127, 127, 0.08);
  color: rgb(var(--base-text-color));
  outline: none;
}

.turn-action-btn--active {
  color: #10b981;
}

.turn-action-btn--primary {
  border-color: rgb(var(--primary-color) / 0.28);
  background: rgb(var(--primary-color) / 0.08);
  color: rgb(var(--primary-color));
}

.turn-action-btn--primary:hover:not(:disabled) {
  border-color: rgb(var(--primary-color) / 0.5);
  background: rgb(var(--primary-color) / 0.14);
  color: rgb(var(--primary-color));
}

.turn-action-btn:disabled {
  cursor: not-allowed;
  opacity: 0.55;
}

.chat-error-banner {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.message__user-avatar {
  width: 36px;
  height: 36px;
  flex: none;
  display: grid;
  place-items: center;
  overflow: hidden;
  border: 1px solid rgb(var(--primary-color) / 0.2);
  border-radius: 50%;
  background: rgba(127, 127, 127, 0.14);
  color: rgba(127, 127, 127, 0.95);
  font-size: 20px;
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

/* 步骤感知思考占位：与工具步进轨统一的微光胶囊 */
.thinking-status {
  position: relative;
  max-width: 100%;
  display: inline-flex;
  align-items: center;
  gap: 8px;
  border: 1px solid rgb(var(--primary-color) / 0.22);
  border-radius: 999px;
  background: rgb(var(--primary-color) / 0.06);
  padding: 6px 13px;
  overflow: hidden;
}

.thinking-status::after {
  content: '';
  position: absolute;
  inset: 0;
  background: linear-gradient(100deg, transparent 25%, rgb(var(--primary-color) / 0.1) 50%, transparent 75%);
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
  color: rgb(var(--primary-color) / 0.95);
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
  min-width: 0;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12.5px;
  font-weight: 500;
  color: rgb(var(--base-text-color) / 0.86);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.thinking-status__tool {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 11px;
  line-height: 1;
  padding: 2px 6px;
  border-radius: 5px;
  background: rgb(var(--primary-color) / 0.12);
  color: rgb(var(--primary-color));
  max-width: 180px;
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
  background: rgb(var(--primary-color) / 0.78);
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

@media (hover: none), (max-width: 640px) {
  .message__turn-actions {
    opacity: 0.92;
  }
}

@media (prefers-reduced-motion: reduce) {
  .thinking-mascot,
  .thinking-status__dot,
  .thinking-status__icon,
  .thinking-status::after,
  .assistant-panel__live i {
    animation: none;
  }

  .user-image-card,
  .user-image-card__zoom,
  .user-doc-chip--interactive,
  .assistant-panel,
  .message__turn-actions,
  .turn-action-btn,
  .thinking-swap-enter-active,
  .thinking-swap-leave-active,
  .thinking-slow-enter-active {
    transition: none;
  }

  .user-image-card:hover,
  .user-image-card:focus-visible {
    transform: none;
  }
}

@media (max-width: 640px) {
  .chat-message-list {
    padding: 16px 12px 60px;
  }
  .message__content {
    max-width: calc(100% - 44px);
  }
  .message__user-stack {
    max-width: calc(100% - 44px);
  }
}
</style>
