package access

import (
	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/pagination"
)

func RegisterAccountActions(a *app.App, svc *AccountService) error {
	auditOn := true
	definitions := []struct {
		id          string
		legacyID    string
		path        string
		legacyPath  string
		name        string
		description string
		handler     action.Handler
		ai          *action.AIMeta
	}{
		{ActionAccountList, legacyActionUserList, "/api/v1/system/accounts/list", "/api/v1/system/users/list", "账号列表", "分页查询当前租户后台账号", action.JSON(svc.list), readAIMeta("查询当前租户的后台账号列表，可按关键词、状态和角色筛选", "查询后台账号列表", "查询当前租户的后台账号列表，可按关键词、状态和角色筛选", "已返回当前主体有权查看的后台账号列表", action.InputSchemaFor[accountListRequest](), action.SchemaFor[pagination.Result[AccountView]](), "账号", "账号管理", "列表", "查询")},
		{ActionAccountGet, legacyActionUserGet, "/api/v1/system/accounts/get", "/api/v1/system/users/get", "账号详情", "查询当前租户后台账号与角色", typedAction[accountIDRequest](svc.Get), readAIMeta("按账号 ID 查询当前租户中的后台账号资料和角色", "查询后台账号详情", "按账号 ID 查询当前租户中的后台账号资料和角色", "已返回当前主体有权查看的后台账号详情", action.InputSchemaFor[accountIDRequest](), action.SchemaFor[AccountView](), "账号", "账号管理", "详情", "角色")},
		{ActionAccountCreate, legacyActionUserCreate, "/api/v1/system/accounts/create", "/api/v1/system/users/create", "创建账号", "创建当前租户后台账号并分配角色", typedAction[accountCreateRequest](svc.Create), action.ExcludeFromAI(action.AIExcludeSensitiveCredential)},
		{ActionAccountUpdate, legacyActionUserUpdate, "/api/v1/system/accounts/update", "/api/v1/system/users/update", "更新账号", "更新当前租户后台账号资料", typedAction[accountUpdateRequest](svc.Update), writeAIMeta("更新当前租户中指定账号的显示名称和组织信息", "更新后台账号资料", "更新当前租户中指定账号的显示名称和组织信息", "账号资料已更新", "medium", action.InputSchemaFor[accountUpdateRequest](), action.SchemaFor[AccountView](), "账号", "更新", "资料")},
		{ActionAccountStatusUpdate, legacyActionUserStatusUpdate, "/api/v1/system/accounts/status/update", "/api/v1/system/users/status/update", "更新账号状态", "启停当前租户后台账号并使旧 Token 失效", typedAction[accountStatusRequest](svc.UpdateStatus), writeAIMeta("启用或停用当前租户后台账号，状态变化会使其旧会话失效", "更新后台账号状态", "启用或停用当前租户后台账号，状态变化会使其旧会话失效", "账号状态已更新", "high", action.InputSchemaFor[accountStatusRequest](), action.SchemaFor[AccountView](), "账号", "状态", "启用", "停用")},
		{ActionAccountRolesUpdate, legacyActionUserRolesUpdate, "/api/v1/system/accounts/roles/update", "/api/v1/system/users/roles/update", "更新账号角色", "事务化替换后台账号角色并使旧 Token 失效", typedAction[accountRolesRequest](svc.UpdateRoles), writeAIMeta("替换当前租户中指定后台账号的角色集合，权限变化会使其旧会话失效", "更新后台账号角色", "替换当前租户中指定后台账号的角色集合，权限变化会使其旧会话失效", "账号角色已更新", "high", action.InputSchemaFor[accountRolesRequest](), action.SchemaFor[AccountView](), "账号", "角色", "权限")},
		{ActionAccountPasswordReset, legacyActionUserPasswordReset, "/api/v1/system/accounts/password/reset", "/api/v1/system/users/password/reset", "重置账号密码", "重置后台账号密码并使旧 Token 失效", typedAction[accountPasswordRequest](svc.ResetPassword), action.ExcludeFromAI(action.AIExcludeSensitiveCredential)},
	}
	for _, definition := range definitions {
		definition := definition
		if definition.id == ActionAccountRolesUpdate {
			definition.ai.DynamicReferences = []action.DynamicReference{{Field: "role_ids[]", ResolverID: "system.assignable_roles", DiscoveryAction: ActionRoleList, RequireEvidence: true, MaxInvalidValues: 8, MaxInvalidBytes: 512}}
		}
		if err := a.RegisterAction(action.Action{
			ID: definition.id, Handler: definition.handler,
			Meta: action.Metadata{
				Capability: "system", Name: definition.name, NameKey: "action." + definition.id + ".name",
				PermissionGroup: "system.account_management", PermissionGroupName: "账号管理", PermissionGroupNameKey: "permission_group.system.account_management",
				Description: definition.description, DescriptionKey: "action." + definition.id + ".description",
				Audit: &auditOn, AI: definition.ai, HTTP: &action.HTTPMeta{Method: "POST", Path: definition.path},
			},
		}); err != nil {
			return err
		}
		if err := a.RegisterActionAlias(action.Alias{ID: definition.legacyID, TargetID: definition.id, HTTP: &action.HTTPMeta{Method: "POST", Path: definition.legacyPath}}); err != nil {
			return err
		}
	}
	return nil
}
