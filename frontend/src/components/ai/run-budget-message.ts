export type RunNotice = 'budgetCompleting' | 'budgetLimited' | 'completionNotice' | 'budgetFailed';

const budgetFailureCategories = new Set([
  'budget_completion_unavailable',
  'budget_completion_invalid_output',
  'budget_hard_limit_exceeded',
  'token_budget_exhausted'
]);

export function isBudgetFailureCategory(category?: string) {
  return Boolean(category && budgetFailureCategories.has(category));
}

export function runNotice(run: Api.AI.AgentRun): RunNotice | undefined {
  if (isBudgetFailureCategory(run.error_category)) return 'budgetFailed';
  if (run.completion_reason === 'budget_limit') return 'budgetLimited';
  if (run.completion_reason) return 'completionNotice';
  if (run.execution_mode === 'completion_only' && !['completed', 'failed', 'cancelled'].includes(run.status)) {
    return 'budgetCompleting';
  }
  return undefined;
}
