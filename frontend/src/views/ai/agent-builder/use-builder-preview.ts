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
import { agentRunEventDelta, subscribeAgentRun } from '@/service/ai-run-stream';

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
      return '';
    })
    .filter(Boolean)
    .join('\n');
}

export function useBuilderPreview(options: {
  agent: Ref<Api.AI.AgentDetail | null>;
  draft: {
    model_alias: string;
    tenant_instructions: string;
    temperature: number;
    top_p: number;
    max_output_tokens: number;
    memory_inherit: boolean;
    memory_enabled: boolean;
    memory_extraction_alias: string;
    memory_consolidation_alias: string;
    memory_auto_save: boolean;
    memory_approval_mode: Api.AI.MemoryPolicy['write_approval_mode'];
    memory_agent_scope: boolean;
    revision: number;
  };
  diagnostics: Ref<Api.AI.AgentDiagnostics | null>;
  dirty: Ref<boolean>;
  supportsVision: Ref<boolean>;
}) {
  const status = ref<BuilderPreviewStatus>('idle');
  const error = ref('');
  const input = ref('');
  const images = ref<Api.Attachment.Item[]>([]);
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
    if (!input.value.trim() && images.value.length === 0) return false;
    if (images.value.length > 0 && !options.supportsVision.value) return false;
    if (images.value.some(item => !item.sha256)) return false;
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
    if (
      conversation.value &&
      conversation.value.agent_version_id === versionId &&
      conversation.value.status === 'active'
    ) {
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
      memory_policy: options.draft.memory_inherit
        ? undefined
        : {
            enabled: options.draft.memory_enabled,
            extraction_model_alias: options.draft.memory_extraction_alias,
            consolidation_model_alias: options.draft.memory_consolidation_alias,
            auto_save_low_sensitive: options.draft.memory_auto_save,
            write_approval_mode: options.draft.memory_approval_mode,
            agent_specific_scope: options.draft.memory_agent_scope,
            max_candidates_per_turn: 8,
            max_consolidation_items: 50,
            max_recall_items: 10,
            max_recall_tokens: 2000,
            policy_version: '1',
            prompt_version: '1',
            schema_version: '1'
          },
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
        } else {
          streaming.value = false;
          streamingText.value = '';
        }
        scheduleSnapshot(runId, stepType !== 'model' && stepType !== 'finalize');
        break;
      }
      case 'step.completed': {
        const stepType = typeof event.payload?.type === 'string' ? String(event.payload.type) : currentStepType.value;
        currentStepType.value = stepType;
        // 模型正文仍由 run.delta 展示；工具步骤完成后立即刷新配对响应。
        scheduleSnapshot(runId, stepType !== 'model' && stepType !== 'finalize');
        break;
      }
      case 'step.failed':
      case 'step.waiting':
        currentStepType.value =
          typeof event.payload?.type === 'string' ? String(event.payload.type) : currentStepType.value;
        scheduleSnapshot(runId, true);
        break;
      case 'run.delta':
        appendStreamDelta(agentRunEventDelta(event));
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
    const selectedImages = [...images.value];
    if (!text && selectedImages.length === 0) return;
    busy.value = true;
    error.value = '';
    clearStreaming();
    try {
      const versionId = await materializeIfNeeded();
      const conv = await ensureConversation(versionId);
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
      const { data: userMsg, error: msgError } = await createAIMessage(
        {
          conversation_id: conv.id,
          role: 'user',
          content
        },
        newIdempotencyKey('builder-msg')
      );
      if (msgError || !userMsg) throw msgError || new Error($t('page.agentBuilder.previewSendFailed'));
      input.value = '';
      images.value = [];
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
    if (
      run.value &&
      (run.value.status === 'queued' || run.value.status === 'running' || run.value.status === 'waiting_confirmation')
    ) {
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
    images.value = [];
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
    images,
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
