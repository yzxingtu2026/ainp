export type ContextBudgetFieldError = 'positiveInteger' | 'outputWithinWindow' | 'inputCapacity';

export interface ModelContextBudgetResult {
  valid: boolean;
  contextWindowError?: ContextBudgetFieldError;
  maxOutputTokensError?: ContextBudgetFieldError;
  safetyMarginTokens: number | null;
  availableInputTokens: number | null;
}

export function resolveModelContextBudget(
  modelType: string,
  contextWindow: number | null,
  maxOutputTokens: number | null
): ModelContextBudgetResult {
  if (modelType !== 'chat') {
    return { valid: true, safetyMarginTokens: null, availableInputTokens: null };
  }
  if (!Number.isSafeInteger(contextWindow) || (contextWindow ?? 0) <= 0) {
    return {
      valid: false,
      contextWindowError: 'positiveInteger',
      safetyMarginTokens: null,
      availableInputTokens: null
    };
  }
  if (!Number.isSafeInteger(maxOutputTokens) || (maxOutputTokens ?? 0) <= 0) {
    return {
      valid: false,
      maxOutputTokensError: 'positiveInteger',
      safetyMarginTokens: null,
      availableInputTokens: null
    };
  }
  const window = contextWindow as number;
  const output = maxOutputTokens as number;
  if (output >= window) {
    return {
      valid: false,
      maxOutputTokensError: 'outputWithinWindow',
      safetyMarginTokens: Math.floor(window / 10),
      availableInputTokens: null
    };
  }
  const safetyMarginTokens = Math.floor(window / 10);
  const availableInputTokens = window - output - safetyMarginTokens;
  if (availableInputTokens <= 0) {
    return { valid: false, maxOutputTokensError: 'inputCapacity', safetyMarginTokens, availableInputTokens };
  }
  return { valid: true, safetyMarginTokens, availableInputTokens };
}
