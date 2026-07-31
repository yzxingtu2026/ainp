package center

import (
	"encoding/json"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
)

func readAIMeta(summary, toolTitle, humanDescription, resultSummary string, inputSchema, outputSchema json.RawMessage, tags ...string) *action.AIMeta {
	return &action.AIMeta{
		ExposeTool: true, Risk: "low", SideEffect: "read",
		ToolTitle: toolTitle, Summary: summary, HumanDescription: humanDescription, ResultSummary: resultSummary,
		Tags: tags, InputSchema: inputSchema, OutputSchema: outputSchema,
	}
}

func mutationAIMeta(summary, toolTitle, humanDescription, resultSummary, risk, sideEffect string, inputSchema, outputSchema json.RawMessage, tags ...string) *action.AIMeta {
	return &action.AIMeta{
		ExposeTool: true, Risk: risk, SideEffect: sideEffect, RequiresConfirmation: true,
		ToolTitle: toolTitle, Summary: summary, HumanDescription: humanDescription, ResultSummary: resultSummary,
		Tags: tags, InputSchema: inputSchema, OutputSchema: outputSchema,
	}
}

type operationIDResult struct {
	ID string `json:"id"`
}
