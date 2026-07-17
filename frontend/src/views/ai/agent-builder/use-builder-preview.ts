import { computed, onBeforeUnmount, ref, type Ref } from 'vue';
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

  let abort: AbortController | null = null;
  let eventCursor = 0;

  const canSend = computed(() => {
    // 预览走 materialize，不依赖已发布 current version 的 diagnostics；
    // 模型可用性由 materialize 校验，diagnostics 仅作发送前软提示。
    if (!options.agent.value) return false;
    if (busy.value) return false;
    if (!options.draft.model_alias.trim()) return false;
    if (options.agent.value.status === 'disabled') return false;
    if (!input.value.trim()) return false;
    return true;
  });

  const runSummary = computed(() => {
    if (error.value) return error.value;
    if (run.value) return `${run.value.status} · steps ${run.value.current_step}`;
    if (status.value === 'idle') return $t('page.agentBuilder.previewIdleSummary');
    return status.value;
  });

  async function refreshMessages() {
    if (!conversation.value) return;
    const { data } = await fetchAIMessages(conversation.value.id, 0, 100);
    messages.value = data?.items || [];
  }

  async function refreshConfirmations() {
    const { data } = await fetchAIToolConfirmations();
    confirmations.value = (data || []).filter(item => item.status === 'pending');
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
      // 配置变化产生新 preview version：换新调试会话
      try {
        await archiveAIConversation(conversation.value.id, conversation.value.revision);
      } catch {
        /* ignore */
      }
      conversation.value = null;
      messages.value = [];
    }
    previewVersionId.value = data.id;
    return data.id;
  }

  async function applyRunSnapshot(runId: string) {
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
    await refreshMessages();
    return data.run;
  }

  async function watchRun(runId: string) {
    abort?.abort();
    abort = new AbortController();
    eventCursor = 0;
    status.value = 'running';
    const poll = window.setInterval(() => {
      void applyRunSnapshot(runId);
    }, 1500);
    try {
      await subscribeAgentRun(runId, eventCursor, abort.signal, event => {
        if (typeof event.sequence === 'number') eventCursor = Math.max(eventCursor, event.sequence);
        void applyRunSnapshot(runId);
      });
    } catch (cause) {
      if (!(cause instanceof DOMException && cause.name === 'AbortError')) {
        throw cause;
      }
      return;
    } finally {
      window.clearInterval(poll);
    }
    await applyRunSnapshot(runId);
  }

  async function send() {
    if (!canSend.value) return;
    const text = input.value.trim();
    if (!text) return;
    busy.value = true;
    error.value = '';
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
      // confirmation pause: stop here; caller can resubscribe after approve
      if (status.value === 'waiting_confirmation') return;
      if (status.value === 'running') {
        // still running after stream end without terminal — poll once more
        await watchRun(started.id);
      }
    } catch (cause) {
      status.value = 'failed';
      error.value = cause instanceof Error ? cause.message : $t('page.agentBuilder.previewFailed');
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
    // keep previewVersionId for reuse until dirty
  }

  function resetPreviewBinding() {
    previewVersionId.value = '';
    void clear();
  }

  onBeforeUnmount(() => {
    abort?.abort();
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
    messageText,
    send,
    stop,
    clear,
    onConfirmationResolved,
    resetPreviewBinding,
    refreshMessages
  };
}
