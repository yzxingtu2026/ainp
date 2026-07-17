import { computed, nextTick, onBeforeUnmount, ref, type Ref } from 'vue';
import { $t } from '@/locales';
import {
  archiveAIConversation,
  cancelAIAgentRun,
  createAIConversation,
  createAIMessage,
  fetchAIAgentRun,
  fetchAIMessages,
  fetchAIToolConfirmations,
  materializeAIAgentPreview,
  startAIAgentRun
} from '@/service/api';
import { getAuthorization } from '@/service/request/shared';
import { getServiceBaseURL } from '@/utils/service';
import { localStg } from '@/utils/storage';

export type BuilderPreviewStatus =
  | 'idle'
  | 'materializing'
  | 'running'
  | 'waiting_confirmation'
  | 'completed'
  | 'failed'
  | 'cancelled'
  | 'stopped';

function newIdempotencyKey(prefix: string) {
  return `${prefix}-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`;
}

function messageText(msg: Api.AI.ConversationMessage): string {
  return (msg.content || [])
    .map(part => {
      if (part.type === 'input_text' || part.type === 'output_text') return part.text || '';
      if (part.type === 'tool_call') return `[tool:${part.tool_name || ''}]`;
      if (part.type === 'tool_result') return `[tool_result:${part.tool_call_id || ''}]`;
      return '';
    })
    .filter(Boolean)
    .join('\n');
}

function eventDeltaText(event: Api.AI.AgentRunEvent): string {
  const payload = event.payload || {};
  if (typeof payload.delta === 'string') return payload.delta;
  if (typeof payload.text === 'string') return payload.text;
  return '';
}

async function subscribeAgentRun(
  runId: string,
  afterSequence: number,
  signal: AbortSignal,
  onEvent: (event: Api.AI.AgentRunEvent) => void
) {
  const isProxy = import.meta.env.DEV && import.meta.env.VITE_HTTP_PROXY === 'Y';
  const { baseURL } = getServiceBaseURL(import.meta.env, isProxy);
  const response = await fetch(`${baseURL}/api/v1/ai/agent-runs/subscribe`, {
    method: 'POST',
    signal,
    headers: {
      'Content-Type': 'application/json',
      Authorization: getAuthorization() || '',
      'Accept-Language': localStg.get('lang') || 'zh-CN'
    },
    body: JSON.stringify({ id: runId, after_sequence: afterSequence, limit: 200 })
  });
  const contentType = response.headers.get('content-type') || '';
  if (!response.ok || !response.body || !contentType.includes('text/event-stream')) {
    let msg = $t('page.agentBuilder.previewSubscribeFailed');
    try {
      const envelope = await response.json();
      if (typeof envelope?.message === 'string' && envelope.message) msg = envelope.message;
    } catch {
      /* ignore */
    }
    throw new Error(msg);
  }
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });
    const frames = buffer.split('\n\n');
    buffer = frames.pop() || '';
    for (const frame of frames) {
      const line = frame.split('\n').find(item => item.startsWith('data: '));
      if (!line) continue;
      try {
        const payload = JSON.parse(line.slice(6)) as Api.AI.AgentRunEvent;
        onEvent(payload);
      } catch {
        /* ignore malformed frame */
      }
    }
  }
}

export function useBuilderPreview(options: {
  agent: Ref<Api.AI.AgentDetail | null>;
  draft: {
    model_alias: string;
    tenant_instructions: string;
    temperature: number;
    top_p: number;
    max_output_tokens: number;
    revision: number;
  };
  diagnostics: Ref<Api.AI.AgentDiagnostics | null>;
  dirty: Ref<boolean>;
}) {
  const status = ref<BuilderPreviewStatus>('idle');
  const error = ref('');
  const input = ref('');
  const messages = ref<Api.AI.ConversationMessage[]>([]);
  const confirmations = ref<Api.AI.ToolConfirmation[]>([]);
  const conversation = ref<Api.AI.Conversation | null>(null);
  const run = ref<Api.AI.AgentRun | null>(null);
  const previewVersionId = ref('');
  const busy = ref(false);
  /** 流式助手正文（来自 run.delta，尚未落库消息时展示） */
  const streamingText = ref('');
  const streaming = ref(false);
  const currentStepType = ref('');
  const messageListEl = ref<HTMLElement | null>(null);

  let abort: AbortController | null = null;
  let eventCursor = 0;
  let snapshotTimer: number | null = null;

  const canSend = computed(() => {
    if (!options.agent.value) return false;
    if (busy.value) return false;
    if (!options.draft.model_alias.trim()) return false;
    if (options.agent.value.status === 'disabled') return false;
    if (!input.value.trim()) return false;
    return true;
  });

  const runSummary = computed(() => {
    if (error.value) return error.value;
    if (streaming.value && streamingText.value) {
      return `${run.value?.status || 'running'} · streaming`;
    }
    if (currentStepType.value && status.value === 'running') {
      return `${run.value?.status || 'running'} · ${currentStepType.value}`;
    }
    if (run.value) return `${run.value.status} · steps ${run.value.current_step}`;
    if (status.value === 'idle') return $t('page.agentBuilder.previewIdleSummary');
    return status.value;
  });

  async function scrollMessagesToBottom() {
    await nextTick();
    const el = messageListEl.value;
    if (el) el.scrollTop = el.scrollHeight;
  }

  async function refreshMessages() {
    if (!conversation.value) return;
    const { data } = await fetchAIMessages(conversation.value.id, 0, 100);
    messages.value = data?.items || [];
    await scrollMessagesToBottom();
  }

  async function refreshConfirmations() {
    const { data } = await fetchAIToolConfirmations();
    confirmations.value = (data || []).filter(item => item.status === 'pending');
  }

  function clearStreaming() {
    streamingText.value = '';
    streaming.value = false;
    currentStepType.value = '';
  }

  function appendStreamDelta(delta: string) {
    if (!delta) return;
    streaming.value = true;
    streamingText.value += delta;
    void scrollMessagesToBottom();
  }

  async function ensureConversation(versionId: string) {
    if (conversation.value && conversation.value.agent_version_id === versionId && conversation.value.status === 'active') {
      return conversation.value;
    }
    if (conversation.value) {
      try {
        await archiveAIConversation(conversation.value.id, conversation.value.revision);
      } catch {
        /* best-effort */
      }
      conversation.value = null;
      messages.value = [];
      clearStreaming();
    }
    const { data, error: createError } = await createAIConversation({
      agent_id: options.agent.value!.id,
      agent_version_id: versionId,
      purpose: 'builder_preview',
      visibility: 'private',
      title: $t('page.agentBuilder.previewConversationTitle')
    });
    if (createError || !data) throw createError || new Error($t('page.agentBuilder.previewConversationFailed'));
    conversation.value = data;
    previewVersionId.value = versionId;
    return data;
  }

  async function materializeIfNeeded() {
    const agent = options.agent.value!;
    status.value = 'materializing';
    const { data, error: matError } = await materializeAIAgentPreview({
      agent_id: agent.id,
      model_alias: options.draft.model_alias,
      tenant_instructions: options.draft.tenant_instructions,
      generation_config: {
        temperature: options.draft.temperature,
        top_p: options.draft.top_p,
        max_output_tokens: options.draft.max_output_tokens
      },
      tool_policy: {},
      revision: options.draft.revision
    });
    if (matError || !data) throw matError || new Error($t('page.agentBuilder.previewMaterializeFailed'));
    if (previewVersionId.value && previewVersionId.value !== data.id && conversation.value) {
      try {
        await archiveAIConversation(conversation.value.id, conversation.value.revision);
      } catch {
        /* ignore */
      }
      conversation.value = null;
      messages.value = [];
      clearStreaming();
    }
    previewVersionId.value = data.id;
    return data.id;
  }

  async function applyRunSnapshot(runId: string, opts: { refreshMsgs?: boolean } = {}) {
    const { data } = await fetchAIAgentRun(runId);
    if (!data?.run) return null;
    run.value = data.run;
    if (data.run.status === 'waiting_confirmation' || data.run.status === 'paused') {
      status.value = 'waiting_confirmation';
      await refreshConfirmations();
    } else if (data.run.status === 'completed') status.value = 'completed';
    else if (data.run.status === 'failed') status.value = 'failed';
    else if (data.run.status === 'cancelled') status.value = 'cancelled';
    else if (data.run.status === 'queued') status.value = 'running';
    else status.value = 'running';

    const terminal =
      data.run.status === 'completed' ||
      data.run.status === 'failed' ||
      data.run.status === 'cancelled' ||
      data.run.status === 'waiting_confirmation' ||
      data.run.status === 'paused';
    if (opts.refreshMsgs || terminal) {
      await refreshMessages();
      if (terminal && data.run.status !== 'waiting_confirmation' && data.run.status !== 'paused') {
        clearStreaming();
      }
    }
    return data.run;
  }

  function scheduleSnapshot(runId: string, refreshMsgs = false) {
    if (snapshotTimer != null) window.clearTimeout(snapshotTimer);
    snapshotTimer = window.setTimeout(() => {
      snapshotTimer = null;
      void applyRunSnapshot(runId, { refreshMsgs });
    }, 200);
  }

  function handleRunEvent(runId: string, event: Api.AI.AgentRunEvent) {
    if (typeof event.sequence === 'number') {
      eventCursor = Math.max(eventCursor, event.sequence);
    }
    switch (event.type) {
      case 'run.queued':
      case 'run.started':
        status.value = 'running';
        if (run.value) run.value = { ...run.value, status: event.type === 'run.queued' ? 'queued' : 'running' };
        scheduleSnapshot(runId);
        break;
      case 'step.started': {
        status.value = 'running';
        const stepType = typeof event.payload?.type === 'string' ? event.payload.type : '';
        currentStepType.value = stepType;
        if (stepType === 'model' || stepType === 'finalize') {
          streaming.value = true;
        }
        scheduleSnapshot(runId);
        break;
      }
      case 'step.completed':
        currentStepType.value = typeof event.payload?.type === 'string' ? String(event.payload.type) : currentStepType.value;
        // 不在 finalize 时提前拉消息，避免与 streaming 气泡重复；终态再刷新。
        scheduleSnapshot(runId, false);
        break;
      case 'step.failed':
      case 'step.waiting':
        currentStepType.value = typeof event.payload?.type === 'string' ? String(event.payload.type) : currentStepType.value;
        scheduleSnapshot(runId, true);
        break;
      case 'run.delta':
        appendStreamDelta(eventDeltaText(event));
        break;
      case 'run.waiting_confirmation':
      case 'run.paused':
        status.value = 'waiting_confirmation';
        void applyRunSnapshot(runId, { refreshMsgs: true });
        break;
      case 'run.completed':
        status.value = 'completed';
        void applyRunSnapshot(runId, { refreshMsgs: true }).then(() => clearStreaming());
        break;
      case 'run.failed':
        status.value = 'failed';
        void applyRunSnapshot(runId, { refreshMsgs: true }).then(() => clearStreaming());
        break;
      case 'run.cancelled':
        status.value = 'cancelled';
        void applyRunSnapshot(runId, { refreshMsgs: true }).then(() => clearStreaming());
        break;
      default:
        scheduleSnapshot(runId);
    }
  }

  async function watchRun(runId: string) {
    abort?.abort();
    abort = new AbortController();
    eventCursor = 0;
    status.value = 'running';
    clearStreaming();
    const poll = window.setInterval(() => {
      void applyRunSnapshot(runId);
    }, 2500);
    try {
      await subscribeAgentRun(runId, eventCursor, abort.signal, event => {
        handleRunEvent(runId, event);
      });
    } catch (cause) {
      if (!(cause instanceof DOMException && cause.name === 'AbortError')) {
        throw cause;
      }
      return;
    } finally {
      window.clearInterval(poll);
      if (snapshotTimer != null) {
        window.clearTimeout(snapshotTimer);
        snapshotTimer = null;
      }
    }
    await applyRunSnapshot(runId, { refreshMsgs: true });
  }

  async function send() {
    if (!canSend.value) return;
    const text = input.value.trim();
    if (!text) return;
    busy.value = true;
    error.value = '';
    clearStreaming();
    try {
      const versionId = await materializeIfNeeded();
      const conv = await ensureConversation(versionId);
      const { data: userMsg, error: msgError } = await createAIMessage(
        {
          conversation_id: conv.id,
          role: 'user',
          content: [{ type: 'input_text', text }]
        },
        newIdempotencyKey('builder-msg')
      );
      if (msgError || !userMsg) throw msgError || new Error($t('page.agentBuilder.previewSendFailed'));
      input.value = '';
      await refreshMessages();
      const { data: started, error: runError } = await startAIAgentRun(
        { conversation_id: conv.id, trigger_message_id: userMsg.id, budget: {} },
        newIdempotencyKey('builder-run')
      );
      if (runError || !started) throw runError || new Error($t('page.agentBuilder.previewRunFailed'));
      run.value = started;
      await watchRun(started.id);
      if (status.value === 'waiting_confirmation') return;
      if (status.value === 'running') {
        await watchRun(started.id);
      }
    } catch (cause) {
      status.value = 'failed';
      error.value = cause instanceof Error ? cause.message : $t('page.agentBuilder.previewFailed');
      clearStreaming();
    } finally {
      busy.value = false;
    }
  }

  async function onConfirmationResolved() {
    confirmations.value = [];
    if (!run.value) return;
    busy.value = true;
    try {
      await watchRun(run.value.id);
    } catch (cause) {
      status.value = 'failed';
      error.value = cause instanceof Error ? cause.message : $t('page.agentBuilder.previewFailed');
    } finally {
      busy.value = false;
    }
  }

  async function stop() {
    abort?.abort();
    abort = null;
    if (run.value && (run.value.status === 'queued' || run.value.status === 'running' || run.value.status === 'waiting_confirmation')) {
      const { data } = await cancelAIAgentRun(run.value.id, run.value.revision);
      if (data?.run) run.value = data.run;
    }
    status.value = 'stopped';
    busy.value = false;
    clearStreaming();
  }

  async function clear() {
    abort?.abort();
    abort = null;
    if (conversation.value) {
      try {
        await archiveAIConversation(conversation.value.id, conversation.value.revision);
      } catch {
        /* ignore */
      }
    }
    conversation.value = null;
    run.value = null;
    messages.value = [];
    confirmations.value = [];
    input.value = '';
    error.value = '';
    status.value = 'idle';
    clearStreaming();
  }

  function resetPreviewBinding() {
    previewVersionId.value = '';
    void clear();
  }

  onBeforeUnmount(() => {
    abort?.abort();
    if (snapshotTimer != null) window.clearTimeout(snapshotTimer);
  });

  return {
    status,
    error,
    input,
    messages,
    confirmations,
    conversation,
    run,
    busy,
    canSend,
    runSummary,
    streamingText,
    streaming,
    currentStepType,
    messageListEl,
    messageText,
    send,
    stop,
    clear,
    onConfirmationResolved,
    resetPreviewBinding,
    refreshMessages
  };
}
