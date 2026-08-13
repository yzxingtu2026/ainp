/** 会话中一次工具调用的视图模型（assistant 消息的 tool_call 与 tool 消息的 tool_result 经 tool_call_id 配对） */
export interface ToolOperation {
  toolCallId: string;
  toolName: string;
  presentation?: Api.AI.ToolPresentation;
  /** tool_call 的调用参数 */
  request: unknown;
  /** 配对 tool_result 的结果载荷，结构异构（aitool.Result / search / load 响应等） */
  result: unknown;
  hasResult: boolean;
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
