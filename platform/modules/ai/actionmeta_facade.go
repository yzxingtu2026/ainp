package ai

import (
	"encoding/json"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/modules/ai/internal/actionmeta"
)

func readAIMeta(summary, toolTitle, humanDescription, resultSummary string, inputSchema, outputSchema json.RawMessage, tags ...string) *action.AIMeta {
	return actionmeta.Read(summary, toolTitle, humanDescription, resultSummary, inputSchema, outputSchema, tags...)
}

func mutationAIMeta(summary, toolTitle, humanDescription, resultSummary, risk, sideEffect string, inputSchema, outputSchema json.RawMessage, tags ...string) *action.AIMeta {
	return actionmeta.Mutation(summary, toolTitle, humanDescription, resultSummary, risk, sideEffect, inputSchema, outputSchema, tags...)
}
