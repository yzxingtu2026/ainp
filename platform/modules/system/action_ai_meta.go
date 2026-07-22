package system

import (
	"encoding/json"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
)

func readAIMeta(summary, humanDescription, resultSummary string, inputSchema, outputSchema json.RawMessage, tags ...string) *action.AIMeta {
	return &action.AIMeta{
		ExposeTool: true, Risk: "low", SideEffect: "read",
		Summary: summary, HumanDescription: humanDescription, ResultSummary: resultSummary,
		Tags: tags, InputSchema: inputSchema, OutputSchema: outputSchema,
	}
}

func writeAIMeta(summary, humanDescription, resultSummary string, risk string, inputSchema, outputSchema json.RawMessage, tags ...string) *action.AIMeta {
	return &action.AIMeta{
		ExposeTool: true, Risk: risk, SideEffect: "write", RequiresConfirmation: true,
		Summary: summary, HumanDescription: humanDescription, ResultSummary: resultSummary,
		Tags: tags, InputSchema: inputSchema, OutputSchema: outputSchema,
	}
}

func destructiveAIMeta(summary, humanDescription, resultSummary string, inputSchema, outputSchema json.RawMessage, tags ...string) *action.AIMeta {
	return &action.AIMeta{
		ExposeTool: true, Risk: "high", SideEffect: "destructive", RequiresConfirmation: true,
		Summary: summary, HumanDescription: humanDescription, ResultSummary: resultSummary,
		Tags: tags, InputSchema: inputSchema, OutputSchema: outputSchema,
	}
}

func withAIMeta(meta action.Metadata, ai *action.AIMeta) action.Metadata {
	meta.AI = ai
	return meta
}
