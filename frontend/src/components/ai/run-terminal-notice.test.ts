import assert from 'node:assert/strict';
import test from 'node:test';
import { runTerminalNotice } from './run-terminal-notice';

function run(fields: Partial<Api.AI.AgentRun> = {}): Api.AI.AgentRun {
  return {
    id: 'run-1',
    user_id: 'user-1',
    conversation_id: 'conversation-1',
    conversation_revision: 1,
    trigger_message_id: 'message-1',
    agent_version_id: 'version-1',
    status: 'failed',
    budget: {
      schema_version: 1,
      max_steps: 1,
      max_model_calls: 1,
      max_tool_calls: 0,
      max_compactions: 0,
      max_duration_seconds: 10,
      run_token_budget: 1000
    },
    current_step: 1,
    model_calls: 1,
    tool_calls: 0,
    compactions: 0,
    input_tokens: 0,
    output_tokens: 0,
    revision: 1,
    created_at: '',
    updated_at: '',
    page_context_status: 'none',
    page_context_bytes: 0,
    ...fields
  };
}

test('prefers the backend terminal message key and hides machine categories', () => {
  assert.equal(
    runTerminalNotice(
      run({ error_category: 'internal_secret_code', user_message_key: 'ai.run.terminal.authorization_changed' })
    ),
    'authorizationChanged'
  );
  assert.equal(runTerminalNotice(run({ error_category: 'internal_secret_code' })), 'unknown');
});

test('prefers a persisted terminal assistant message over a notice', () => {
  assert.equal(
    runTerminalNotice(
      run({ user_message_key: 'ai.run.terminal.provider_unavailable', terminal_message_id: 'message-1' })
    ),
    null
  );
});

test('keeps historical terminal runs readable without backend terminal fields', () => {
  assert.equal(runTerminalNotice(run({ error_category: 'timeout' })), 'providerUnavailable');
  assert.equal(runTerminalNotice(run({ error_category: 'model_not_found' })), 'modelUnavailable');
  assert.equal(runTerminalNotice(run({ status: 'cancelled', error_category: 'cancelled' })), 'cancelled');
  assert.equal(runTerminalNotice(run({ error_category: 'document_too_large' })), 'contextUnavailable');
  assert.equal(runTerminalNotice(run({ error_category: 'model.context_budget_invalid' })), 'modelUnavailable');
});
