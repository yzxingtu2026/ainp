/** 会话中一次工具调用的视图模型（assistant 消息的 tool_call 与 tool 消息的 tool_result 经 tool_call_id 配对） */
export interface ToolOperation {
  toolCallId: string;
  retryOfToolCallId?: string;
  toolName: string;
  presentation?: Api.AI.ToolPresentation;
  /** tool_call 的调用参数 */
  request: unknown;
  /** 配对 tool_result 的结果载荷，结构异构（aitool.Result / search / load 响应等） */
  result: unknown;
  hasResult: boolean;
  retriedAfterDeferral?: boolean;
}

export function collapseRetriedToolOperations(operations: ToolOperation[]): ToolOperation[] {
  const operationsByCallId = new Map(operations.map(operation => [operation.toolCallId, operation] as const));
  const callIdCounts = new Map<string, number>();
  const retryCounts = new Map<string, number>();
  for (const operation of operations) {
    callIdCounts.set(operation.toolCallId, (callIdCounts.get(operation.toolCallId) || 0) + 1);
    if (!operation.retryOfToolCallId) continue;
    retryCounts.set(operation.retryOfToolCallId, (retryCounts.get(operation.retryOfToolCallId) || 0) + 1);
  }
  const retriesByOriginal = new Map<string, ToolOperation>();
  const collapsedRetryCallIds = new Set<string>();
  for (const operation of operations) {
    if (!operation.retryOfToolCallId || retryCounts.get(operation.retryOfToolCallId) !== 1) continue;
    const original = operationsByCallId.get(operation.retryOfToolCallId);
    if (
      !original?.hasResult ||
      !toolResultDeferred(original.result) ||
      original.retryOfToolCallId ||
      callIdCounts.get(original.toolCallId) !== 1 ||
      callIdCounts.get(operation.toolCallId) !== 1 ||
      operation.toolName !== original.toolName ||
      !sameJsonValue(operation.request, original.request)
    )
      continue;
    retriesByOriginal.set(operation.retryOfToolCallId, operation);
    collapsedRetryCallIds.add(operation.toolCallId);
  }
  return operations
    .filter(operation => !collapsedRetryCallIds.has(operation.toolCallId))
    .map(operation => {
      const retry = retriesByOriginal.get(operation.toolCallId);
      if (!retry) return operation;
      return {
        ...operation,
        result: retry.hasResult ? retry.result : undefined,
        hasResult: retry.hasResult,
        retriedAfterDeferral: true
      };
    });
}

function sameJsonValue(left: unknown, right: unknown): boolean {
  if (Object.is(left, right)) return true;
  if (Array.isArray(left) || Array.isArray(right)) {
    return (
      Array.isArray(left) &&
      Array.isArray(right) &&
      left.length === right.length &&
      left.every((value, index) => sameJsonValue(value, right[index]))
    );
  }
  if (!left || !right || typeof left !== 'object' || typeof right !== 'object') return false;
  const leftRecord = left as Record<string, unknown>;
  const rightRecord = right as Record<string, unknown>;
  const leftKeys = Object.keys(leftRecord).sort();
  const rightKeys = Object.keys(rightRecord).sort();
  return (
    leftKeys.length === rightKeys.length &&
    leftKeys.every((key, index) => key === rightKeys[index] && sameJsonValue(leftRecord[key], rightRecord[key]))
  );
}

/** 从异构工具结果中尽力提取一句人类可读摘要 */
export function toolResultSummary(result: unknown): string {
  if (!result || typeof result !== 'object') return '';
  const summary = (result as Record<string, unknown>).summary;
  return typeof summary === 'string' ? summary : '';
}

/** 结果是否标记失败（aitool.Result.status === 'failed'，其余结构默认成功） */
export function toolResultFailed(result: unknown): boolean {
  if (!result || typeof result !== 'object') return false;
  return (result as Record<string, unknown>).status === 'failed';
}

export function toolResultRehydrated(result: unknown): boolean {
  if (!result || typeof result !== 'object') return false;
  return (result as Record<string, unknown>).status === 'rehydrated';
}

export function toolResultDeferred(result: unknown): boolean {
  if (!result || typeof result !== 'object') return false;
  return (result as Record<string, unknown>).error_category === 'deferred_for_confirmation';
}

/** 格式化为可展示的 JSON 文本；字符串入参尝试先解析再美化 */
export function formatToolPayload(value: unknown): string {
  if (value === undefined || value === null) return '';
  if (typeof value === 'string') {
    try {
      return JSON.stringify(JSON.parse(value), null, 2);
    } catch {
      return value;
    }
  }
  return JSON.stringify(value, null, 2) ?? String(value);
}
