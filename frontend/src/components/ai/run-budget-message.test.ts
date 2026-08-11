import assert from 'node:assert/strict';
import test from 'node:test';
import { isBudgetFailureCategory, runNotice } from './run-budget-message';

function run(overrides: Partial<Api.AI.AgentRun>): Api.AI.AgentRun {
  return {
    id: 'run-1',
    user_id: 'user-1',
    conversation_id: 'conversation-1',
    conversation_revision: 1,
    trigger_message_id: 'message-1',
    agent_version_id: 'version-1',
    status: 'running',
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
    model_calls: 0,
    tool_calls: 0,
    compactions: 0,
    input_tokens: 0,
    output_tokens: 0,
    revision: 1,
    created_at: '',
    updated_at: '',
    page_context_status: 'none',
    page_context_bytes: 0,
    ...overrides
  };
}

test('maps budget lifecycle without exposing machine codes', () => {
  assert.equal(runNotice(run({ execution_mode: 'completion_only' })), 'budgetCompleting');
  assert.equal(runNotice(run({ status: 'completed', completion_reason: 'budget_limit' })), 'budgetLimited');
  assert.equal(runNotice(run({ status: 'completed', completion_reason: 'future_reason' })), 'completionNotice');
  assert.equal(runNotice(run({ status: 'failed', error_category: 'budget_completion_unavailable' })), 'budgetFailed');
});

test('recognizes only stable budget failure categories', () => {
  assert.equal(isBudgetFailureCategory('budget_hard_limit_exceeded'), true);
  assert.equal(isBudgetFailureCategory('tool_failed'), false);
});
