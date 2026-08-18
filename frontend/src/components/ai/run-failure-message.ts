export type RunFailureKind =
  | "budget"
  | "timeout"
  | "cancelled"
  | "providerUnavailable"
  | "streamProtocol"
  | "document"
  | "tool"
  | "generic";

export interface RunFailurePresentation {
  category: string;
  kind: RunFailureKind;
  completedToolOperation: boolean;
}

const budgetFailureCategories = new Set([
  "budget_completion_unavailable",
  "budget_completion_invalid_output",
  "budget_hard_limit_exceeded",
  "token_budget_exhausted",
]);

const documentFailureCategories = new Set([
  "document_unavailable",
  "document_unsupported_format",
  "document_too_large",
  "document_unsafe_content",
  "document_unsafe_docx_content",
  "document_unsafe_xlsx_content",
  "document_corrupt",
  "document_empty",
  "document_parse_failed",
]);

export function runFailurePresentation(
  detail: Api.AI.AgentRunDetail,
): RunFailurePresentation {
  const category =
    detail.run.error_category ||
    (detail.run.status === "cancelled" ? "cancelled" : "unknown");
  let kind: RunFailureKind = "generic";
  if (budgetFailureCategories.has(category)) kind = "budget";
  else if (documentFailureCategories.has(category)) kind = "document";
  else if (category === "timeout") kind = "timeout";
  else if (category === "cancelled") kind = "cancelled";
  else if (category === "provider_unavailable") kind = "providerUnavailable";
  else if (category === "stream_protocol_error") kind = "streamProtocol";
  else if (category === "tool_failed") kind = "tool";

  const completedToolOperation = detail.steps.some(
    (step) =>
      step.type === "tool_execute" &&
      step.status === "completed" &&
      Boolean(step.tool_invocation_id),
  );
  return { category, kind, completedToolOperation };
}
