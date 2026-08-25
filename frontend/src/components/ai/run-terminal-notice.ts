export type TerminalNotice =
  | 'providerUnavailable'
  | 'modelUnavailable'
  | 'authorizationChanged'
  | 'safetyRefusal'
  | 'contextUnavailable'
  | 'persistenceUnavailable'
  | 'cancelled'
  | 'budgetLimit'
  | 'operationFailed'
  | 'unknown';

const noticeByMessageKey: Record<string, TerminalNotice> = {
  'ai.run.terminal.provider_unavailable': 'providerUnavailable',
  'ai.run.terminal.model_unavailable': 'modelUnavailable',
  'ai.run.terminal.authorization_changed': 'authorizationChanged',
  'ai.run.terminal.safety_refusal': 'safetyRefusal',
  'ai.run.terminal.context_unavailable': 'contextUnavailable',
  'ai.run.terminal.persistence_unavailable': 'persistenceUnavailable',
  'ai.run.terminal.cancelled': 'cancelled',
  'ai.run.terminal.budget_limit': 'budgetLimit',
  'ai.run.terminal.operation_failed': 'operationFailed',
  'ai.run.terminal.unknown': 'unknown'
};

const providerCategories = new Set([
  'rate_limited',
  'provider_unavailable',
  'timeout',
  'stream_protocol_error',
  'response_mismatch',
  'model_failed'
]);

export function runTerminalNotice(run: Api.AI.AgentRun): TerminalNotice | null {
  if (run.terminal_message_id) return null;
  if (run.user_message_key && noticeByMessageKey[run.user_message_key]) {
    return noticeByMessageKey[run.user_message_key];
  }
  const category = run.error_category || '';
  if (run.status === 'cancelled' || category === 'cancelled') return 'cancelled';
  if (providerCategories.has(category)) return 'providerUnavailable';
  if (['authentication_failed', 'model_not_found', 'capability_not_supported'].includes(category)) {
    return 'modelUnavailable';
  }
  if (category === 'content_filtered') return 'safetyRefusal';
  if (category.includes('authorization') || category === 'permission_denied') return 'authorizationChanged';
  if (category.startsWith('document_') || category.includes('context') || category === 'input_too_large') {
    return 'contextUnavailable';
  }
  if (category.includes('budget')) return 'budgetLimit';
  if (category === 'tool_failed') return 'operationFailed';
  if (run.status === 'failed') return 'unknown';
  return null;
}
