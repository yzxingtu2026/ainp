package tests

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/aitool"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/config"
	"github.com/yzxingtu2026/ainp/platform/modules/system"
)

func TestSystemManagementAndPersonalActionsExposeSafeAIContracts(t *testing.T) {
	application := app.New(&config.Config{}, openOutboxTestDB(t), nil, nil)
	attachStorage(t, application)
	if err := system.Register(application); err != nil {
		t.Fatal(err)
	}

	readActions := []string{
		system.ActionUserList, system.ActionUserGet,
		system.ActionRoleList, system.ActionRoleGet, system.ActionPermissionCatalog,
		system.ActionSettingList, system.ActionSettingGet,
		system.ActionProfileGet, system.ActionUserPreferencesGet,
		system.ActionNotificationList, system.ActionNotificationUnreadCount,
		system.ActionNotificationTemplateList, system.ActionNotificationTemplatePreview,
		system.ActionAttachmentDownloadPrepare, system.ActionAttachmentListByResource,
		system.ActionAttachmentList, system.ActionAttachmentGet, system.ActionAttachmentPolicyGet,
	}
	writeActions := []string{
		system.ActionUserUpdate, system.ActionUserStatusUpdate, system.ActionUserRolesUpdate,
		system.ActionRoleCreate, system.ActionRoleUpdate, system.ActionRoleDelete, system.ActionRolePermissionsUpdate,
		system.ActionSettingUpdate, system.ActionSettingReset,
		system.ActionProfileUpdate, system.ActionUserPreferencesUpdate,
		system.ActionNotificationMarkRead, system.ActionNotificationMarkAllRead,
		system.ActionNotificationArchive, system.ActionNotificationUnarchive,
		system.ActionNotificationTemplateUpdate, system.ActionNotificationTemplateReset,
		system.ActionAttachmentUploadPrepare, system.ActionAttachmentBind,
		system.ActionAttachmentUnbind, system.ActionAttachmentDelete,
	}

	for _, actionID := range readActions {
		assertSystemAIContract(t, application, actionID, false)
	}
	for _, actionID := range writeActions {
		assertSystemAIContract(t, application, actionID, true)
	}

	for _, actionID := range []string{system.ActionUserCreate, system.ActionUserPasswordReset} {
		credentialAction, ok := application.Actions.Get(actionID)
		if !ok || credentialAction.Meta.AI == nil || credentialAction.Meta.AI.ExposeTool || credentialAction.Meta.AI.ExclusionReason != action.AIExcludeSensitiveCredential {
			t.Fatalf("credential Action %s must remain behind the sensitive credential boundary: %#v", actionID, credentialAction.Meta.AI)
		}
	}

	projected, err := (&aitool.ActionProvider{Actions: application.Actions}).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	projectedIDs := make(map[string]bool, len(projected))
	projectedTitles := make(map[string]string, len(projected))
	for _, definition := range projected {
		projectedIDs[definition.RuntimeRef] = true
		projectedTitles[definition.RuntimeRef] = definition.TitleForHuman
		if action.HasExplicitInputContract(definition.InputSchema) {
			t.Fatalf("Action contract marker leaked into projected Tool schema: %s", definition.RuntimeRef)
		}
	}
	for _, actionID := range append(readActions, writeActions...) {
		if !projectedIDs[actionID] {
			t.Fatalf("second-batch action was not projected from Action Registry: %s", actionID)
		}
	}
	if projectedIDs[system.ActionUserCreate] || projectedIDs[system.ActionUserPasswordReset] {
		t.Fatal("credential-bearing user Actions must not be projected as Action Tools")
	}
	for actionID, want := range map[string]string{
		system.ActionUserList:                    "查询系统用户列表",
		system.ActionUserUpdate:                  "更新系统用户资料",
		system.ActionAttachmentDownloadPrepare:   "准备下载附件",
		system.ActionNotificationTemplatePreview: "预览通知模板",
	} {
		if got := projectedTitles[actionID]; got != want {
			t.Fatalf("Action %s tool title = %q, want %q", actionID, got, want)
		}
	}
	userUpdate, _ := application.Actions.Get(system.ActionUserUpdate)
	assertSchemaRequired(t, userUpdate.Meta.AI.InputSchema, "id", "display_name")
}

func assertSystemAIContract(t *testing.T, application *app.App, actionID string, mutation bool) {
	t.Helper()
	registered, ok := application.Actions.Get(actionID)
	if !ok || registered.Meta.AI == nil || !registered.Meta.AI.ExposeTool {
		t.Fatalf("Action %s must expose an AI contract: registered=%t meta=%#v", actionID, ok, registered.Meta.AI)
	}
	meta := registered.Meta.AI
	if !json.Valid(meta.InputSchema) || meta.ToolTitle == "" || meta.Summary == "" || meta.HumanDescription == "" || meta.ResultSummary == "" {
		t.Fatalf("Action %s has an incomplete AI contract: %#v", actionID, meta)
	}
	if !action.HasExplicitInputContract(meta.InputSchema) {
		t.Fatalf("Action %s does not use the explicit input contract", actionID)
	}
	if mutation {
		if !meta.RequiresConfirmation || (meta.SideEffect != "write" && meta.SideEffect != "destructive") {
			t.Fatalf("mutation Action %s must use a confirmed side effect: %#v", actionID, meta)
		}
		return
	}
	if meta.RequiresConfirmation || meta.SideEffect != "read" || meta.Risk != "low" {
		t.Fatalf("read Action %s must remain low-risk without confirmation: %#v", actionID, meta)
	}
}

func assertSchemaRequired(t *testing.T, schema json.RawMessage, expected ...string) {
	t.Helper()
	var decoded struct {
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(schema, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Required) != len(expected) {
		t.Fatalf("required = %#v, want %#v", decoded.Required, expected)
	}
	for i := range expected {
		if decoded.Required[i] != expected[i] {
			t.Fatalf("required = %#v, want %#v", decoded.Required, expected)
		}
	}
}
