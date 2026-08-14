package settings

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/auth"
	"github.com/yzxingtu2026/ainp/platform/framework/errs"
	"github.com/yzxingtu2026/ainp/platform/framework/setting"
)

const (
	ActionSettingList                = setting.ActionListPermission
	ActionSettingGet                 = "system.setting_get"
	ActionSettingUpdate              = setting.ActionUpdatePermission
	ActionSettingSecretUpdate        = "system.setting_secret_update"
	ActionSettingReset               = "system.setting_reset"
	ActionProfileGet                 = "system.profile_get"
	ActionProfileUpdate              = "system.profile_update"
	ActionProfilePasswordUpdate      = "system.profile_password_update"
	ActionUserPreferencesGet         = "system.user_preferences_get"
	ActionUserPreferencesUpdate      = "system.user_preferences_update"
	ActionMCPConnectionInfo          = "system.mcp_connection_info"
	SettingKeyAttachmentUploadPolicy = "attachment.upload_policy"
)

func readAIMeta(summary, toolTitle, humanDescription, resultSummary string, inputSchema, outputSchema json.RawMessage, tags ...string) *action.AIMeta {
	return &action.AIMeta{ExposeTool: true, Risk: "low", SideEffect: "read", ToolTitle: toolTitle, Summary: summary, HumanDescription: humanDescription, ResultSummary: resultSummary, Tags: tags, InputSchema: inputSchema, OutputSchema: outputSchema}
}

func writeAIMeta(summary, toolTitle, humanDescription, resultSummary, risk string, inputSchema, outputSchema json.RawMessage, tags ...string) *action.AIMeta {
	return &action.AIMeta{ExposeTool: true, Risk: risk, SideEffect: "write", RequiresConfirmation: true, ToolTitle: toolTitle, Summary: summary, HumanDescription: humanDescription, ResultSummary: resultSummary, Tags: tags, InputSchema: inputSchema, OutputSchema: outputSchema}
}

func withAIMeta(meta action.Metadata, ai *action.AIMeta) action.Metadata { meta.AI = ai; return meta }

func tenantIdentity(ctx context.Context) (string, string, error) {
	ac := auth.MustFromContext(ctx)
	if !ac.Authenticated || ac.Realm != auth.RealmTenant || ac.TenantID == "" || ac.PrincipalID == "" {
		return "", "", errs.NewKey(errs.CodeUnauthorized, "common.unauthorized", "unauthorized")
	}
	return ac.TenantID, ac.PrincipalID, nil
}

func requireTenant(ctx context.Context) (*auth.Context, error) {
	ac := auth.MustFromContext(ctx)
	if !ac.Authenticated || strings.TrimSpace(ac.TenantID) == "" {
		return nil, errs.NewKey(errs.CodeUnauthorized, "common.unauthorized", "unauthorized")
	}
	return ac, nil
}
