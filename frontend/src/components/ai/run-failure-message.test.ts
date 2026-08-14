import assert from "node:assert/strict";
import test from "node:test";
import { runFailurePresentation } from "./run-failure-message";

function detail(
  category: string,
  steps: Partial<Api.AI.AgentStep>[] = [],
): Api.AI.AgentRunDetail {
  return {
    run: {
      id: "run-1",
      user_id: "user-1",
      conversation_id: "conversation-1",
      conversation_revision: 1,
      trigger_message_id: "message-1",
      agent_version_id: "version-1",
      status: category === "cancelled" ? "cancelled" : "failed",
      error_category: category,
      budget: {
        schema_version: 1,
        max_steps: 1,
        max_model_calls: 1,
        max_tool_calls: 0,
        max_compactions: 0,
        max_duration_seconds: 10,
        run_token_budget: 1000,
      },
      current_step: 1,
      model_calls: 1,
      tool_calls: 0,
      compactions: 0,
      input_tokens: 0,
      output_tokens: 0,
      revision: 1,
      created_at: "",
      updated_at: "",
      page_context_status: "none",
      page_context_bytes: 0,
    },
    steps: steps.map((step, index) => ({
      id: `step-${index}`,
      run_id: "run-1",
      step_index: index + 1,
      type: "model",
      status: "failed",
      input_summary: {},
      output_summary: {},
      created_at: "",
      ...step,
    })),
  };
}

test("maps stable model stream failure categories independently", () => {
  assert.equal(runFailurePresentation(detail("timeout")).kind, "timeout");
  assert.equal(runFailurePresentation(detail("cancelled")).kind, "cancelled");
  assert.equal(
    runFailurePresentation(detail("provider_unavailable")).kind,
    "providerUnavailable",
  );
  assert.equal(
    runFailurePresentation(detail("stream_protocol_error")).kind,
    "streamProtocol",
  );
});

test("shows the side-effect notice only from a completed linked tool step", () => {
  assert.equal(
    runFailurePresentation(
      detail("timeout", [
        {
          type: "tool_execute",
          status: "completed",
          tool_invocation_id: "invocation-1",
        },
      ]),
    ).completedToolOperation,
    true,
  );
  assert.equal(
    runFailurePresentation(
      detail("timeout", [
        {
          type: "tool_execute",
          status: "failed",
          tool_invocation_id: "invocation-1",
        },
      ]),
    ).completedToolOperation,
    false,
  );
  assert.equal(
    runFailurePresentation(
      detail("timeout", [{ type: "tool_execute", status: "completed" }]),
    ).completedToolOperation,
    false,
  );
});
