import { getAuthorization } from '@/service/request/shared';
import { getServiceBaseURL } from '@/utils/service';
import { localStg } from '@/utils/storage';

export function agentRunEventDelta(event: Api.AI.AgentRunEvent): string {
  const payload = event.payload || {};
  if (typeof payload.delta === 'string') return payload.delta;
  if (typeof payload.text === 'string') return payload.text;
  return '';
}

export async function subscribeAgentRun(
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
    let message = 'Agent Run stream unavailable';
    try {
      const envelope = await response.json();
      if (typeof envelope?.message === 'string' && envelope.message) message = envelope.message;
    } catch {
      // Keep the transport-level fallback message.
    }
    throw new Error(message);
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
        onEvent(JSON.parse(line.slice(6)) as Api.AI.AgentRunEvent);
      } catch {
        // A malformed frame must not terminate a recoverable stream.
      }
    }
  }
}
