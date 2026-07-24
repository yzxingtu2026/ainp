package system

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

func writeAIMeta(summary, toolTitle, humanDescription, resultSummary string, risk string, inputSchema, outputSchema json.RawMessage, tags ...string) *action.AIMeta {
	return &action.AIMeta{
		ExposeTool: true, Risk: risk, SideEffect: "write", RequiresConfirmation: true,
		ToolTitle: toolTitle, Summary: summary, HumanDescription: humanDescription, ResultSummary: resultSummary,
		Tags: tags, InputSchema: inputSchema, OutputSchema: outputSchema,
	}
}

func destructiveAIMeta(summary, toolTitle, humanDescription, resultSummary string, inputSchema, outputSchema json.RawMessage, tags ...string) *action.AIMeta {
	return &action.AIMeta{
		ExposeTool: true, Risk: "high", SideEffect: "destructive", RequiresConfirmation: true,
		ToolTitle: toolTitle, Summary: summary, HumanDescription: humanDescription, ResultSummary: resultSummary,
		Tags: tags, InputSchema: inputSchema, OutputSchema: outputSchema,
	}
}

func withAIMeta(meta action.Metadata, ai *action.AIMeta) action.Metadata {
	meta.AI = ai
	return meta
}
