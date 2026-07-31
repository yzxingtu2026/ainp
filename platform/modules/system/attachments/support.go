package attachments

import (
	"context"
	"encoding/json"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
)

const (
	ActionAttachmentUploadPrepare   = "system.attachment_upload_prepare"
	ActionAttachmentDownloadPrepare = "system.attachment_download_prepare"
	ActionAttachmentBind            = "system.attachment_bind"
	ActionAttachmentUnbind          = "system.attachment_unbind"
	ActionAttachmentListByResource  = "system.attachment_list_by_resource"
	ActionAttachmentDelete          = "system.attachment_delete"
	ActionAttachmentList            = "system.attachment_list"
	ActionAttachmentMineList        = "system.attachment_mine_list"
	ActionAttachmentGet             = "system.attachment_get"
	ActionAttachmentPolicyGet       = "system.attachment_policy_get"
)

func readAIMeta(summary, toolTitle, humanDescription, resultSummary string, inputSchema, outputSchema json.RawMessage, tags ...string) *action.AIMeta {
	return &action.AIMeta{ExposeTool: true, Risk: "low", SideEffect: "read", ToolTitle: toolTitle, Summary: summary, HumanDescription: humanDescription, ResultSummary: resultSummary, Tags: tags, InputSchema: inputSchema, OutputSchema: outputSchema}
}
func writeAIMeta(summary, toolTitle, humanDescription, resultSummary, risk string, inputSchema, outputSchema json.RawMessage, tags ...string) *action.AIMeta {
	return &action.AIMeta{ExposeTool: true, Risk: risk, SideEffect: "write", RequiresConfirmation: true, ToolTitle: toolTitle, Summary: summary, HumanDescription: humanDescription, ResultSummary: resultSummary, Tags: tags, InputSchema: inputSchema, OutputSchema: outputSchema}
}
func destructiveAIMeta(summary, toolTitle, humanDescription, resultSummary string, inputSchema, outputSchema json.RawMessage, tags ...string) *action.AIMeta {
	return &action.AIMeta{ExposeTool: true, Risk: "high", SideEffect: "destructive", RequiresConfirmation: true, ToolTitle: toolTitle, Summary: summary, HumanDescription: humanDescription, ResultSummary: resultSummary, Tags: tags, InputSchema: inputSchema, OutputSchema: outputSchema}
}

func typedAction[T any](handler func(context.Context, json.RawMessage) (any, error)) action.Handler {
	return action.JSON(func(ctx context.Context, request T) (any, error) {
		input, err := json.Marshal(request)
		if err != nil {
			return nil, errs.Wrap(errs.CodeInternal, "encode typed request failed", err)
		}
		return handler(ctx, input)
	})
}

func tenantIdentity(ctx context.Context) (string, string, error) {
	ac := auth.MustFromContext(ctx)
	if !ac.Authenticated || ac.Realm != auth.RealmTenant || ac.TenantID == "" || ac.UserID == "" {
		return "", "", errs.NewKey(errs.CodeUnauthorized, "common.unauthorized", "unauthorized")
	}
	return ac.TenantID, ac.UserID, nil
}
