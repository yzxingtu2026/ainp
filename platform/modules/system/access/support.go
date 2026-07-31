package access

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
)

const (
	ActionRoleList              = "system.role_list"
	ActionRoleGet               = "system.role_get"
	ActionRoleCreate            = "system.role_create"
	ActionRoleUpdate            = "system.role_update"
	ActionRoleDelete            = "system.role_delete"
	ActionRolePermissionsUpdate = "system.role_permissions_update"
	ActionPermissionCatalog     = "system.permission_catalog"
	ActionUserList              = "system.user_list"
	ActionUserGet               = "system.user_get"
	ActionUserCreate            = "system.user_create"
	ActionUserUpdate            = "system.user_update"
	ActionUserStatusUpdate      = "system.user_status_update"
	ActionUserRolesUpdate       = "system.user_roles_update"
	ActionUserPasswordReset     = "system.user_password_reset"
	MenuRoleManagement          = "system.roles"
	MenuUserManagement          = "system.users"
	BuiltinAdminCode            = "admin"
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

func requireTenant(ctx context.Context) (*auth.Context, error) {
	ac := auth.MustFromContext(ctx)
	if !ac.Authenticated || strings.TrimSpace(ac.TenantID) == "" {
		return nil, errs.NewKey(errs.CodeUnauthorized, "common.unauthorized", "unauthorized")
	}
	return ac, nil
}
