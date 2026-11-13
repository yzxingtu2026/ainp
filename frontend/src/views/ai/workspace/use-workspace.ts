import { computed, nextTick, ref } from 'vue';
import {
  archiveAIConversation,
  cancelAIAgentRun,
  createAIConversationForSlot,
  createAIMessage,
  fetchAIAgentRun,
  fetchAIAgentRunEvents,
  fetchAIAgentRuns,
  fetchAIAgentSlot,
  fetchAIConversations,
  fetchAIMessages,
  fetchAIToolConfirmations,
  restoreAIConversation,
  resumeAIAgentRun,
  retryAIAgentRun,
  startAIAgentRun,
  submitAIPageCapture,
  updateAIConversation
} from '@/service/api';
import { agentRunEventDelta, subscribeAgentRun } from '@/service/ai-run-stream';
import { $t } from '@/locales';
import { capturePageContext } from '@/composables/page-context';
import { getStandaloneErrorMessage, isRequestError } from '@/service/request';
import { runFailurePresentation } from '@/components/ai/run-failure-message';
import { runTerminalNotice } from '@/components/ai/run-terminal-notice';
import { createLatestRequestGate, loadAllCursorPages } from './modules/message-sync';
import { isWorkspaceDocument } from './modules/document-attachment';
import {
  PAGE_CAPTURE_EVENT_REQUESTED,
  PAGE_CAPTURE_EVENT_SETTLED,
  parsePageCaptureRequestEvent
} from './modules/page-capture';
import { renderBusinessMainContentFile, uploadCapturedPageImage } from './modules/page-capture-render';

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
  if (issue.code === 'model.context_budget_invalid') return $t('page.aiWorkspace.diagnosticContextBudgetInvalid');
  if (issue.code === 'tool.vision_required') return $t('page.aiWorkspace.diagnosticPageCaptureVisionRequired');
  if (issue.code === 'tool.surface_not_projected') return $t('page.aiWorkspace.diagnosticToolSurfaceNotProjected');
  return issue.message;
}

function runFailureMessage(detail: Api.AI.AgentRunDetail) {
  if (detail.run.terminal_message_id) return '';
  const presentation = runFailurePresentation(detail);
  const notice = runTerminalNotice(detail.run) || 'unknown';
  let message = $t(`page.aiRun.terminal.${notice}` as App.I18n.I18nKey);
  if ((detail.run.completed_operation_count || 0) > 0 || presentation.completedToolOperation) {
    message += ` ${$t('page.aiWorkspace.completedToolsPreserved')}`;
  }
  return message;
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
  const attachments = ref<Api.Attachment.Item[]>([]);
  const busy = ref(false);
  const retrying = ref(false);
  const capturingPage = ref(false);
  const messageListEl = ref<HTMLElement | null>(null);

  let abort: AbortController | null = null;
  let eventCursor = 0;
  let pollTimer: number | null = null;
  let initialized = false;
  let initializePromise: Promise<void> | null = null;
  const handledCaptureRequestIds = new Set<string>();
  const activeCaptureRequestIds = new Set<string>();
  const messageRefreshGate = createLatestRequestGate();

  const canSend = computed(() =>
    Boolean(slot.value?.healthy && !busy.value && (input.value.trim() || attachments.value.length > 0))
  );

  const mascotState = computed<'idle' | 'thinking' | 'working' | 'error'>(() => {
    if (status.value === 'failed' || (slot.value && !slot.value.healthy)) return 'error';
    if (capturingPage.value || status.value === 'running' || status.value === 'reconnecting') return 'working';
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
    if (current !== 'running' && current !== 'reconnecting' && !capturingPage.value) return '';
    return runDetail.value?.run.id || '';
  });

  const failedRunId = computed(() => {
    const run = runDetail.value?.run;
    return run?.status === 'failed' ? run.id : '';
  });

  const canRetryFailedRun = computed(() => {
    const run = runDetail.value?.run;
    return Boolean(run && run.status === 'failed' && (run.retryable ?? true) && !busy.value && !retrying.value);
  });

  /** 步骤感知的思考占位提示：根据当前运行中的 step 动态映射图标与文案 */
  const thinkingHint = computed<{ icon: string; text: string; toolName?: string }>(() => {
    if (capturingPage.value) {
      return { icon: 'mdi:monitor-screenshot', text: $t('chat.thinkingPageCapture') };
    }
    const steps = runDetail.value?.steps || [];
    const running = [...steps].reverse().find(step => step.status === 'running' || step.status === 'waiting');
    if (!running) return { icon: 'mdi:brain-outline', text: $t('chat.thinkingThinking') };
    if (running.type === 'context_assemble') return { icon: 'mdi:file-tree-outline', text: $t('chat.thinkingContext') };
    if (running.type === 'tool_search') return { icon: 'mdi:magnify', text: $t('chat.thinkingToolSearch') };
    if (running.type === 'tool_load') return { icon: 'mdi:package-variant-outline', text: $t('chat.thinkingToolLoad') };
    if (running.type === 'tool_execute') {
      const toolName = typeof running.input_summary?.tool_name === 'string' ? running.input_summary.tool_name : '';
      if (toolName === 'builtin__page_capture') {
        return { icon: 'mdi:monitor-screenshot', text: $t('chat.thinkingPageCapture') };
      }
      return { icon: 'mdi:api', text: $t('chat.thinkingToolExecute'), toolName };
    }
    if (running.type === 'finalize')
      return { icon: 'mdi:message-processing-outline', text: $t('chat.thinkingFinalize') };
    return { icon: 'mdi:brain-outline', text: $t('chat.thinkingThinking') };
  });

  async function scrollToBottom(force = false) {
    const before = messageListEl.value;
    const nearBottom = !before || before.scrollHeight - before.scrollTop - before.clientHeight <= 120;
    await nextTick();
    const current = messageListEl.value;
    if (current && (force || nearBottom)) {
      current.scrollTop = current.scrollHeight;
    }
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
    if (!messageRefreshGate.isCurrent(requestVersion) || conversation.value?.id !== currentConversation.id)
      return false;
    messages.value = nextMessages;
    await scrollToBottom();
    return true;
  }

  async function refreshConfirmations() {
    const { data } = await fetchAIToolConfirmations();
    confirmations.value = (data || []).filter(item => item.status === 'pending');
  }

  function applyRunStatus(run: Api.AI.AgentRun) {
    if (run.status === 'paused' && capturingPage.value) status.value = 'running';
    else if (run.status === 'waiting_confirmation' || run.status === 'paused') status.value = 'waiting_confirmation';
    else if (run.status === 'completed') status.value = 'completed';
    else if (run.status === 'failed') status.value = 'failed';
    else if (run.status === 'cancelled') status.value = 'cancelled';
    else status.value = 'running';
  }

  async function fulfillPageCapture(runId: string, captureRequestId: string) {
    if (
      !captureRequestId ||
      handledCaptureRequestIds.has(captureRequestId) ||
      activeCaptureRequestIds.has(captureRequestId)
    ) {
      return;
    }
    activeCaptureRequestIds.add(captureRequestId);
    capturingPage.value = true;
    status.value = 'running';
    try {
      let file: File;
      try {
        file = await renderBusinessMainContentFile(captureRequestId);
      } catch {
        await submitAIPageCapture(
          {
            capture_request_id: captureRequestId,
            status: 'failed',
            failure_reason: 'render_failed'
          },
          { errorMode: 'silent' }
        );
        return;
      }
      let uploaded: Api.Attachment.Item;
      try {
        uploaded = await uploadCapturedPageImage(file);
      } catch {
        await submitAIPageCapture(
          {
            capture_request_id: captureRequestId,
            status: 'failed',
            failure_reason: 'upload_failed'
          },
          { errorMode: 'silent' }
        );
        return;
      }
      await submitAIPageCapture(
        {
          capture_request_id: captureRequestId,
          status: 'completed',
          attachment_id: uploaded.id,
          attachment_revision: uploaded.sha256
        },
        { errorMode: 'silent' }
      );
    } finally {
      handledCaptureRequestIds.add(captureRequestId);
      activeCaptureRequestIds.delete(captureRequestId);
      capturingPage.value = activeCaptureRequestIds.size > 0;
      const activeRun = runDetail.value?.run;
      if (activeRun && activeRun.id === runId && activeRun.status !== 'cancelled') {
        void watchRun(runId, eventCursor);
      }
    }
  }

  async function inspectPendingCaptureEvents(runId: string) {
    const { data } = await fetchAIAgentRunEvents(runId, 0, 200);
    if (!data?.items?.length) return false;
    const settledIds = new Set<string>();
    const requested: string[] = [];
    for (const item of data.items) {
      eventCursor = Math.max(eventCursor, item.sequence || 0);
      if (item.type === PAGE_CAPTURE_EVENT_SETTLED && typeof item.payload?.capture_request_id === 'string') {
        settledIds.add(item.payload.capture_request_id);
      }
      const parsed = parsePageCaptureRequestEvent(item);
      if (parsed) {
        if (
          parsed.expiresAt &&
          Number.isFinite(Date.parse(parsed.expiresAt)) &&
          Date.parse(parsed.expiresAt) <= Date.now()
        ) {
          settledIds.add(parsed.captureRequestId);
        } else {
          requested.push(parsed.captureRequestId);
        }
      }
    }
    const pendingId = [...requested].reverse().find(id => !settledIds.has(id) && !handledCaptureRequestIds.has(id));
    if (!pendingId) return false;
    void fulfillPageCapture(runId, pendingId);
    return true;
  }

  async function refreshRun(runId: string, refreshContent = false) {
    const { data } = await fetchAIAgentRun(runId);
    if (!data) return null;
    runDetail.value = data;
    applyRunStatus(data.run);
    if (isRunTerminal(data.run.status)) {
      activeCaptureRequestIds.clear();
      capturingPage.value = false;
    }
    if (data.run.status === 'failed' || data.run.status === 'cancelled') error.value = runFailureMessage(data);
    else if (isRunTerminal(data.run.status)) error.value = '';
    if (data.run.status === 'waiting_confirmation' || (data.run.status === 'paused' && !capturingPage.value)) {
      await refreshConfirmations();
    }
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
    if (event.type === PAGE_CAPTURE_EVENT_REQUESTED) {
      const parsed = parsePageCaptureRequestEvent(event);
      if (parsed) {
        void fulfillPageCapture(runId, parsed.captureRequestId);
      }
      void refreshRun(runId);
      void refreshMessages();
      return;
    }
    if (event.type === PAGE_CAPTURE_EVENT_SETTLED) {
      if (typeof event.payload?.capture_request_id === 'string') {
        handledCaptureRequestIds.add(event.payload.capture_request_id);
        activeCaptureRequestIds.delete(event.payload.capture_request_id);
        capturingPage.value = activeCaptureRequestIds.size > 0;
      }
      void refreshRun(runId, true);
      return;
    }
    if (event.type === 'run.waiting_confirmation' || event.type === 'run.paused') {
      if (event.type === 'run.paused' && event.payload?.reason === 'waiting_page_capture') {
        const requestId = typeof event.payload?.capture_request_id === 'string' ? event.payload.capture_request_id : '';
        if (requestId) void fulfillPageCapture(runId, requestId);
      }
      void refreshRun(runId, true);
      return;
    }
    if (event.type === 'run.completed' || event.type === 'run.failed' || event.type === 'run.cancelled') {
      activeCaptureRequestIds.clear();
      capturingPage.value = false;
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
    await scrollToBottom(true);
    const { data } = await fetchAIAgentRuns({
      page: 1,
      page_size: 1,
      conversation_id: item.id
    });
    const latest = data?.items?.[0];
    if (latest) {
      await refreshRun(latest.id);
      if (latest.status === 'paused') {
        await inspectPendingCaptureEvents(latest.id);
      } else if (!isRunTerminal(latest.status) && latest.status !== 'waiting_confirmation') {
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
    activeCaptureRequestIds.clear();
    capturingPage.value = false;
    conversation.value = null;
    storeConversationId();
    messages.value = [];
    confirmations.value = [];
    runDetail.value = null;
    streamingText.value = '';
    input.value = '';
    attachments.value = [];
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
    const selectedAttachments = [...attachments.value];
    busy.value = true;
    error.value = '';
    streamingText.value = '';
    runDetail.value = null;
    try {
      const current = await ensureConversation(text);
      const content: Api.AI.ConversationContentPart[] = [];
      if (text) content.push({ type: 'input_text', text });
      for (const attachment of selectedAttachments) {
        if (isWorkspaceDocument(attachment)) {
          content.push({
            type: 'input_file',
            file_attachment_id: attachment.id,
            attachment_revision: attachment.sha256
          });
        } else {
          content.push({
            type: 'input_image',
            image_attachment_id: attachment.id,
            attachment_revision: attachment.sha256,
            detail: 'auto'
          });
        }
      }
      const { data: userMessage, error: messageError } = await createAIMessage(
        { conversation_id: current.id, role: 'user', content },
        idempotencyKey('workspace-message')
      );
      if (messageError || !userMessage) throw messageError || new Error($t('page.aiWorkspace.sendFailed'));
      input.value = '';
      attachments.value = [];
      messages.value.push(userMessage);
      await scrollToBottom(true);
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
    activeCaptureRequestIds.clear();
    capturingPage.value = false;
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

  async function inspectRun(runId?: string) {
    if (!runId || runDetail.value?.run.id === runId) return runDetail.value;
    if (activeRunId.value && activeRunId.value !== runId) return runDetail.value;
    const { data } = await fetchAIAgentRun(runId);
    if (data) runDetail.value = data;
    return data || null;
  }

  async function retryRun(targetRunId?: string) {
    if (retrying.value || busy.value) return;
    let target = runDetail.value;
    if (targetRunId && target?.run.id !== targetRunId) {
      const { data } = await fetchAIAgentRun(targetRunId);
      if (data) target = data;
    }
    if (!target || target.run.status !== 'failed' || !(target.run.retryable ?? true)) return;
    retrying.value = true;
    error.value = '';
    try {
      const { data, error: retryError } = await retryAIAgentRun(target.run.id, target.run.revision);
      if (retryError || !data) return;
      runDetail.value = data;
      streamingText.value = '';
      eventCursor = 0;
      await scrollToBottom(true);
      void watchRun(data.run.id, 0);
    } finally {
      retrying.value = false;
    }
  }

  async function resumeRun() {
    const run = runDetail.value?.run;
    if (!run || (run.status !== 'waiting_confirmation' && run.status !== 'paused')) return;
    const { data, error: resumeError } = await resumeAIAgentRun(run.id, run.revision);
    if (resumeError || !data) return;
    runDetail.value = data;
    void watchRun(data.run.id, eventCursor);
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
    activeCaptureRequestIds.clear();
    capturingPage.value = false;
    messageRefreshGate.invalidate();
  }

  function reset() {
    dispose();
    handledCaptureRequestIds.clear();
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
    attachments.value = [];
    busy.value = false;
    retrying.value = false;
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
    failedRunId,
    canRetryFailedRun,
    retrying,
    thinkingHint,
    capturingPage,
    input,
    attachments,
    busy,
    canSend,
    mascotState,
    messageListEl,
    initialize,
    selectConversation,
    newConversation,
    send,
    stop,
    inspectRun,
    retryRun,
    resumeRun,
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
