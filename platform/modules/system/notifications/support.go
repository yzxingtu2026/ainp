package notifications

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
)

func requireTenant(ctx context.Context) (*auth.Context, error) {
	ac := auth.MustFromContext(ctx)
	if !ac.Authenticated || strings.TrimSpace(ac.TenantID) == "" {
		return nil, errs.NewKey(errs.CodeUnauthorized, "common.unauthorized", "unauthorized")
	}
	return ac, nil
}

func readAIMeta(summary, toolTitle, humanDescription, resultSummary string, inputSchema, outputSchema json.RawMessage, tags ...string) *action.AIMeta {
	return &action.AIMeta{ExposeTool: true, Risk: "low", SideEffect: "read", ToolTitle: toolTitle, Summary: summary, HumanDescription: humanDescription, ResultSummary: resultSummary, Tags: tags, InputSchema: inputSchema, OutputSchema: outputSchema}
}

func writeAIMeta(summary, toolTitle, humanDescription, resultSummary, risk string, inputSchema, outputSchema json.RawMessage, tags ...string) *action.AIMeta {
	return &action.AIMeta{ExposeTool: true, Risk: risk, SideEffect: "write", RequiresConfirmation: true, ToolTitle: toolTitle, Summary: summary, HumanDescription: humanDescription, ResultSummary: resultSummary, Tags: tags, InputSchema: inputSchema, OutputSchema: outputSchema}
}
