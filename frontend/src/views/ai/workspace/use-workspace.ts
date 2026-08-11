import { computed, nextTick, ref } from 'vue';
import {
  archiveAIConversation,
  cancelAIAgentRun,
  createAIConversationForSlot,
  createAIMessage,
  fetchAIAgentRun,
  fetchAIAgentRuns,
  fetchAIAgentSlot,
  fetchAIConversations,
  fetchAIMessages,
  fetchAIToolConfirmations,
  restoreAIConversation,
  startAIAgentRun,
  updateAIConversation
} from '@/service/api';
import { agentRunEventDelta, subscribeAgentRun } from '@/service/ai-run-stream';
import { $t } from '@/locales';
import { capturePageContext } from '@/composables/page-context';
import { getStandaloneErrorMessage, isRequestError } from '@/service/request';
import { createLatestRequestGate, loadAllCursorPages } from './modules/message-sync';

export type WorkspaceStatus =
  | 'idle'
  | 'loading'
  | 'running'
  | 'reconnecting'
  | 'waiting_confirmation'
  | 'completed'
  | 'failed'
  | 'cancelled';

const activeConversationStorageKey = 'ainp:ai-assistant:conversation-id';

function storedConversationId() {
  return window.sessionStorage.getItem(activeConversationStorageKey) || '';
}

function storeConversationId(id?: string) {
  if (id) window.sessionStorage.setItem(activeConversationStorageKey, id);
  else window.sessionStorage.removeItem(activeConversationStorageKey);
}

function idempotencyKey(prefix: string) {
  return `${prefix}-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`;
}

function isRunTerminal(status: Api.AI.AgentRunStatus) {
  return status === 'completed' || status === 'failed' || status === 'cancelled';
}

function diagnosticMessage(issue: Api.AI.AgentDiagnosticIssue) {
  if (issue.code === 'model.context_window_missing') return $t('page.aiWorkspace.diagnosticContextWindow');
  return issue.message;
}

function runFailureMessage(detail: Api.AI.AgentRunDetail) {
  const category = detail.run.error_category || 'unknown';
  if (category !== 'tool_failed') {
    return $t('page.aiWorkspace.runFailureSimple', { category });
  }
  const failedStep = [...detail.steps].reverse().find(step => step.status === 'failed');
  const reason = typeof failedStep?.output_summary?.reason === 'string' ? failedStep.output_summary.reason : '';
  return $t('page.aiWorkspace.runFailure', {
    category,
    reason: reason || failedStep?.error_category || 'unknown'
  });
}

function createAiConversationController() {
  const status = ref<WorkspaceStatus>('loading');
  const error = ref('');
  const slot = ref<Api.AI.AgentSlotResolution | null>(null);
  const activeConversations = ref<Api.AI.Conversation[]>([]);
  const archivedConversations = ref<Api.AI.Conversation[]>([]);
  const conversation = ref<Api.AI.Conversation | null>(null);
  const messages = ref<Api.AI.ConversationMessage[]>([]);
  const confirmations = ref<Api.AI.ToolConfirmation[]>([]);
  const runDetail = ref<Api.AI.AgentRunDetail | null>(null);
  const streamingText = ref('');
  const input = ref('');
  const images = ref<Api.Attachment.Item[]>([]);
  const busy = ref(false);
  const messageListEl = ref<HTMLElement | null>(null);

  let abort: AbortController | null = null;
  let eventCursor = 0;
  let pollTimer: number | null = null;
  let initialized = false;
  let initializePromise: Promise<void> | null = null;
  const messageRefreshGate = createLatestRequestGate();

  const canSend = computed(() =>
    Boolean(slot.value?.healthy && !busy.value && (input.value.trim() || images.value.length > 0))
  );

  const mascotState = computed<'idle' | 'thinking' | 'working' | 'error'>(() => {
    if (status.value === 'failed' || (slot.value && !slot.value.healthy)) return 'error';
    if (status.value === 'running' || status.value === 'reconnecting') return 'working';
    if (status.value === 'waiting_confirmation') return 'thinking';
    return 'idle';
  });

  /** 当前 run 已落库的 assistant 输出文本（按消息顺序拼接 output_text） */
  const persistedRunText = computed(() => {
    const runId = runDetail.value?.run.id;
    if (!runId) return '';
    return messages.value
      .filter(message => message.role === 'assistant' && message.run_id === runId)
      .map(message =>
        (message.content || [])
          .filter(part => part.type === 'output_text')
          .map(part => part.text || '')
          .join('')
      )
      .join('');
  });

  /**
   * 扣除已落库部分后的流式文本。
   * 模型每轮输出会一边发 run.delta（累积为 streamingText）一边在本轮结束后将同文本落库为 assistant 消息，
   * refreshMessages 拉回后若不清理会造成重复渲染；这里减去已落库前缀，
   * 只保留尚未落库的当前轮输出（工具执行等待期间为空，配合思考动画占位）。
   */
  const displayStreamingText = computed(() => {
    const persisted = persistedRunText.value;
    const raw = streamingText.value;
    if (persisted && raw.startsWith(persisted)) return raw.slice(persisted.length);
    return raw;
  });

  /** 当前正在运行的 run ID（未运行时为空串），供消息列表标记活跃 run 分组 */
  const activeRunId = computed(() => {
    const current = status.value;
    if (current !== 'running' && current !== 'reconnecting') return '';
    return runDetail.value?.run.id || '';
  });

  /** 步骤感知的思考占位提示：根据当前运行中的 step 动态映射图标与文案 */
  const thinkingHint = computed<{ icon: string; text: string; toolName?: string }>(() => {
    const steps = runDetail.value?.steps || [];
    const running = [...steps].reverse().find(step => step.status === 'running' || step.status === 'waiting');
    if (!running) return { icon: 'mdi:brain-outline', text: $t('chat.thinkingThinking') };
    if (running.type === 'context_assemble') return { icon: 'mdi:file-tree-outline', text: $t('chat.thinkingContext') };
    if (running.type === 'tool_search') return { icon: 'mdi:magnify', text: $t('chat.thinkingToolSearch') };
    if (running.type === 'tool_load') return { icon: 'mdi:package-variant-outline', text: $t('chat.thinkingToolLoad') };
    if (running.type === 'tool_execute') {
      const toolName = typeof running.input_summary?.tool_name === 'string' ? running.input_summary.tool_name : '';
      return { icon: 'mdi:api', text: $t('chat.thinkingToolExecute'), toolName };
    }
    if (running.type === 'finalize')
      return { icon: 'mdi:message-processing-outline', text: $t('chat.thinkingFinalize') };
    return { icon: 'mdi:brain-outline', text: $t('chat.thinkingThinking') };
  });

  async function scrollToBottom() {
    await nextTick();
    if (messageListEl.value) messageListEl.value.scrollTop = messageListEl.value.scrollHeight;
  }

  async function loadConversations() {
    const [active, archived] = await Promise.all([
      fetchAIConversations({ page: 1, page_size: 100, purpose: 'workspace', status: 'active' }),
      fetchAIConversations({ page: 1, page_size: 100, purpose: 'workspace', status: 'archived' })
    ]);
    activeConversations.value = active.data?.items || [];
    archivedConversations.value = archived.data?.items || [];
  }

  async function refreshMessages() {
    const currentConversation = conversation.value;
    if (!currentConversation) return false;
    const requestVersion = messageRefreshGate.begin();
    let nextMessages: Api.AI.ConversationMessage[];
    try {
      nextMessages = await loadAllCursorPages<Api.AI.ConversationMessage>(async afterSequence => {
        const { data, error: fetchError } = await fetchAIMessages(currentConversation.id, afterSequence, 100);
        if (fetchError) throw fetchError;
        return data || { items: [], next_sequence: afterSequence, has_more: false };
      });
    } catch {
      return false;
    }
    if (!messageRefreshGate.isCurrent(requestVersion) || conversation.value?.id !== currentConversation.id) return false;
    messages.value = nextMessages;
    await scrollToBottom();
    return true;
  }

  async function refreshConfirmations() {
    const { data } = await fetchAIToolConfirmations();
    confirmations.value = (data || []).filter(item => item.status === 'pending');
  }

  function applyRunStatus(run: Api.AI.AgentRun) {
    if (run.status === 'waiting_confirmation' || run.status === 'paused') status.value = 'waiting_confirmation';
    else if (run.status === 'completed') status.value = 'completed';
    else if (run.status === 'failed') status.value = 'failed';
    else if (run.status === 'cancelled') status.value = 'cancelled';
    else status.value = 'running';
  }

  async function refreshRun(runId: string, refreshContent = false) {
    const { data } = await fetchAIAgentRun(runId);
    if (!data) return null;
    runDetail.value = data;
    applyRunStatus(data.run);
    if (data.run.status === 'failed') error.value = runFailureMessage(data);
    if (data.run.status === 'waiting_confirmation' || data.run.status === 'paused') await refreshConfirmations();
    if (refreshContent || isRunTerminal(data.run.status)) {
      const refreshed = await refreshMessages();
      if (isRunTerminal(data.run.status) && refreshed) streamingText.value = '';
    }
    return data.run;
  }

  function stopPolling() {
    if (pollTimer != null) window.clearInterval(pollTimer);
    pollTimer = null;
  }

  function startPolling(runId: string) {
    stopPolling();
    pollTimer = window.setInterval(() => {
      void refreshRun(runId);
      void refreshMessages();
    }, 2500);
  }

  function handleEvent(runId: string, event: Api.AI.AgentRunEvent) {
    eventCursor = Math.max(eventCursor, event.sequence || 0);
    if (event.type === 'run.delta') {
      streamingText.value += agentRunEventDelta(event);
      void scrollToBottom();
      return;
    }
    if (event.type === 'run.waiting_confirmation' || event.type === 'run.paused') {
      void refreshRun(runId, true);
      return;
    }
    if (event.type === 'run.completed' || event.type === 'run.failed' || event.type === 'run.cancelled') {
      void refreshRun(runId, true);
      return;
    }
    if (event.type.startsWith('step.') || event.type.startsWith('run.')) {
      void refreshRun(runId);
      // 工具调用/结果消息随 step 事件实时写入后端，同步拉取以便对话区实时展示工具活动
      void refreshMessages();
    }
  }

  async function watchRun(runId: string, afterSequence = 0) {
    abort?.abort();
    abort = new AbortController();
    eventCursor = afterSequence;
    status.value = 'running';
    startPolling(runId);
    try {
      await subscribeAgentRun(runId, eventCursor, abort.signal, event => handleEvent(runId, event));
    } catch (cause) {
      if (cause instanceof DOMException && cause.name === 'AbortError') return;
      status.value = 'reconnecting';
      error.value = getStandaloneErrorMessage(cause, $t('page.aiWorkspace.reconnectFallback'));
    } finally {
      stopPolling();
    }
    const current = await refreshRun(runId, true);
    if (
      current &&
      !isRunTerminal(current.status) &&
      current.status !== 'waiting_confirmation' &&
      current.status !== 'paused'
    ) {
      window.setTimeout(() => void watchRun(runId, eventCursor), 1000);
    }
  }

  async function selectConversation(item: Api.AI.Conversation) {
    abort?.abort();
    stopPolling();
    messageRefreshGate.invalidate();
    conversation.value = item;
    storeConversationId(item.id);
    messages.value = [];
    runDetail.value = null;
    confirmations.value = [];
    streamingText.value = '';
    error.value = '';
    status.value = 'loading';
    await refreshMessages();
    const { data } = await fetchAIAgentRuns({
      page: 1,
      page_size: 1,
      conversation_id: item.id
    });
    const latest = data?.items?.[0];
    if (latest) {
      await refreshRun(latest.id);
      if (!isRunTerminal(latest.status) && latest.status !== 'waiting_confirmation' && latest.status !== 'paused') {
        void watchRun(latest.id);
      }
    } else {
      status.value = 'idle';
    }
  }

  function newConversation() {
    abort?.abort();
    stopPolling();
    messageRefreshGate.invalidate();
    conversation.value = null;
    storeConversationId();
    messages.value = [];
    confirmations.value = [];
    runDetail.value = null;
    streamingText.value = '';
    input.value = '';
    images.value = [];
    error.value = '';
    status.value = 'idle';
  }

  async function ensureConversation(title: string) {
    if (conversation.value) return conversation.value;
    const { data, error: createError } = await createAIConversationForSlot('workspace.default', {
      title: title.slice(0, 64) || $t('page.aiWorkspace.newConversation'),
      visibility: 'private'
    });
    if (createError || !data) throw createError || new Error($t('page.aiWorkspace.createFailed'));
    conversation.value = data;
    storeConversationId(data.id);
    activeConversations.value.unshift(data);
    return data;
  }

  async function send() {
    if (!canSend.value) return;
    const text = input.value.trim();
    const selectedImages = [...images.value];
    busy.value = true;
    error.value = '';
    streamingText.value = '';
    runDetail.value = null;
    try {
      const current = await ensureConversation(text);
      const content: Api.AI.ConversationContentPart[] = [];
      if (text) content.push({ type: 'input_text', text });
      for (const image of selectedImages) {
        content.push({
          type: 'input_image',
          image_attachment_id: image.id,
          attachment_revision: image.sha256,
          detail: 'auto'
        });
      }
      const { data: userMessage, error: messageError } = await createAIMessage(
        { conversation_id: current.id, role: 'user', content },
        idempotencyKey('workspace-message')
      );
      if (messageError || !userMessage) throw messageError || new Error($t('page.aiWorkspace.sendFailed'));
      input.value = '';
      images.value = [];
      messages.value.push(userMessage);
      await scrollToBottom();
      status.value = 'running';
      const { data: started, error: runError } = await startAIAgentRun(
        {
          conversation_id: current.id,
          trigger_message_id: userMessage.id,
          page_context: capturePageContext()
        },
        idempotencyKey('workspace-run')
      );
      if (runError || !started) throw runError || new Error($t('page.aiWorkspace.runFailed'));
      runDetail.value = { run: started, steps: [] };
      await watchRun(started.id);
    } catch (cause) {
      status.value = 'failed';
      if (!isRequestError(cause)) {
        error.value = getStandaloneErrorMessage(cause, $t('page.aiWorkspace.unavailableGeneric'));
      }
    } finally {
      busy.value = false;
    }
  }

  async function stop() {
    abort?.abort();
    stopPolling();
    const run = runDetail.value?.run;
    if (run && !isRunTerminal(run.status)) {
      const { data } = await cancelAIAgentRun(run.id, run.revision);
      if (data) runDetail.value = data;
    }
    status.value = 'cancelled';
    busy.value = false;
    streamingText.value = '';
  }

  async function renameConversation(item: Api.AI.Conversation, title: string) {
    const { data } = await updateAIConversation(item.id, title, item.revision);
    if (!data) return;
    Object.assign(item, data);
    if (conversation.value?.id === data.id) conversation.value = data;
  }

  async function archiveConversation(item: Api.AI.Conversation) {
    const { data } = await archiveAIConversation(item.id, item.revision);
    if (!data) return;
    activeConversations.value = activeConversations.value.filter(value => value.id !== item.id);
    archivedConversations.value.unshift(data);
    if (conversation.value?.id === item.id) newConversation();
  }

  async function restoreConversation(item: Api.AI.Conversation) {
    const { data } = await restoreAIConversation(item.id, item.revision);
    if (!data) return;
    archivedConversations.value = archivedConversations.value.filter(value => value.id !== item.id);
    activeConversations.value.unshift(data);
  }

  async function onConfirmationResolved(_status?: 'approved' | 'rejected', result?: Api.AI.ToolResult) {
    await refreshConfirmations();
    if (result?.status === 'failed') error.value = result.summary;
    const run = runDetail.value?.run;
    if (run) void watchRun(run.id, eventCursor);
  }

  async function initialize() {
    if (initialized) return;
    if (initializePromise) return initializePromise;
    initializePromise = (async () => {
      status.value = 'loading';
      error.value = '';
      const [slotResult] = await Promise.all([fetchAIAgentSlot('workspace.default'), loadConversations()]);
      slot.value = slotResult.data || null;
      if (slotResult.error || !slot.value) error.value = $t('page.aiWorkspace.slotResolveFailed');
      else if (!slot.value.healthy) error.value = slot.value.diagnostics.map(diagnosticMessage).join('；');
      initialized = true;
      const previousId = storedConversationId();
      const previous = activeConversations.value.find(item => item.id === previousId);
      if (previous) await selectConversation(previous);
      else {
        if (previousId) storeConversationId();
        status.value = slot.value?.healthy ? 'idle' : 'failed';
      }
    })();
    try {
      await initializePromise;
    } finally {
      initializePromise = null;
    }
  }

  function dispose() {
    abort?.abort();
    abort = null;
    stopPolling();
    messageRefreshGate.invalidate();
  }

  function reset() {
    dispose();
    initialized = false;
    initializePromise = null;
    eventCursor = 0;
    status.value = 'loading';
    error.value = '';
    slot.value = null;
    activeConversations.value = [];
    archivedConversations.value = [];
    conversation.value = null;
    messages.value = [];
    confirmations.value = [];
    runDetail.value = null;
    streamingText.value = '';
    input.value = '';
    images.value = [];
    busy.value = false;
    messageListEl.value = null;
    storeConversationId();
  }

  return {
    status,
    error,
    slot,
    activeConversations,
    archivedConversations,
    conversation,
    messages,
    confirmations,
    runDetail,
    displayStreamingText,
    activeRunId,
    thinkingHint,
    input,
    images,
    busy,
    canSend,
    mascotState,
    messageListEl,
    initialize,
    selectConversation,
    newConversation,
    send,
    stop,
    renameConversation,
    archiveConversation,
    restoreConversation,
    onConfirmationResolved,
    dispose,
    reset
  };
}

export type AiConversationController = ReturnType<typeof createAiConversationController>;

let sharedController: AiConversationController | null = null;

export function useAiConversationController() {
  sharedController ||= createAiConversationController();
  return sharedController;
}

export const useWorkspace = useAiConversationController;
