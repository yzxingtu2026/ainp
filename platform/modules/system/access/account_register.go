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
		path        string
		name        string
		description string
		handler     action.Handler
		ai          *action.AIMeta
	}{
		{ActionUserList, "/api/v1/system/users/list", "用户列表", "分页查询当前租户用户", action.JSON(svc.list), readAIMeta("查询当前租户的用户列表，可按关键词、状态和角色筛选", "查询系统用户列表", "查询当前租户的用户列表，可按关键词、状态和角色筛选", "已返回当前用户有权查看的系统用户列表", action.InputSchemaFor[accountListRequest](), action.SchemaFor[pagination.Result[AccountView]](), "用户", "用户管理", "列表", "查询")},
		{ActionUserGet, "/api/v1/system/users/get", "用户详情", "查询当前租户用户与角色", typedAction[accountIDRequest](svc.Get), readAIMeta("按用户 ID 查询当前租户中的用户资料和角色", "查询系统用户详情", "按用户 ID 查询当前租户中的用户资料和角色", "已返回当前用户有权查看的系统用户详情", action.InputSchemaFor[accountIDRequest](), action.SchemaFor[AccountView](), "用户", "用户管理", "详情", "角色")},
		{ActionUserCreate, "/api/v1/system/users/create", "创建用户", "创建当前租户用户并分配角色", typedAction[accountCreateRequest](svc.Create), action.ExcludeFromAI(action.AIExcludeSensitiveCredential)},
		{ActionUserUpdate, "/api/v1/system/users/update", "更新用户", "更新当前租户用户资料", typedAction[accountUpdateRequest](svc.Update), writeAIMeta("更新当前租户中指定用户的显示名称和组织信息", "更新系统用户资料", "更新当前租户中指定用户的显示名称和组织信息", "用户资料已更新", "medium", action.InputSchemaFor[accountUpdateRequest](), action.SchemaFor[AccountView](), "用户", "更新", "资料")},
		{ActionUserStatusUpdate, "/api/v1/system/users/status/update", "更新用户状态", "启停当前租户用户并使旧 Token 失效", typedAction[accountStatusRequest](svc.UpdateStatus), writeAIMeta("启用或停用当前租户用户，状态变化会使其旧会话失效", "更新系统用户状态", "启用或停用当前租户用户，状态变化会使其旧会话失效", "用户状态已更新", "high", action.InputSchemaFor[accountStatusRequest](), action.SchemaFor[AccountView](), "用户", "状态", "启用", "停用")},
		{ActionUserRolesUpdate, "/api/v1/system/users/roles/update", "更新用户角色", "事务化替换用户角色并使旧 Token 失效", typedAction[accountRolesRequest](svc.UpdateRoles), writeAIMeta("替换当前租户中指定用户的角色集合，权限变化会使其旧会话失效", "更新系统用户角色", "替换当前租户中指定用户的角色集合，权限变化会使其旧会话失效", "用户角色已更新", "high", action.InputSchemaFor[accountRolesRequest](), action.SchemaFor[AccountView](), "用户", "角色", "权限")},
		{ActionUserPasswordReset, "/api/v1/system/users/password/reset", "重置用户密码", "重置密码并使旧 Token 失效", typedAction[accountPasswordRequest](svc.ResetPassword), action.ExcludeFromAI(action.AIExcludeSensitiveCredential)},
	}
	for _, definition := range definitions {
		definition := definition
		if definition.id == ActionUserRolesUpdate {
			definition.ai.DynamicReferences = []action.DynamicReference{{Field: "role_ids[]", ResolverID: "system.assignable_roles", DiscoveryAction: ActionRoleList, RequireEvidence: true, MaxInvalidValues: 8, MaxInvalidBytes: 512}}
		}
		if err := a.RegisterAction(action.Action{
			ID: definition.id, Handler: definition.handler,
			Meta: action.Metadata{
				Capability: "system", Name: definition.name, NameKey: "action." + definition.id + ".name",
				PermissionGroup: "system.user_management", PermissionGroupName: "用户管理", PermissionGroupNameKey: "permission_group.system.user_management",
				Description: definition.description, DescriptionKey: "action." + definition.id + ".description",
				Audit: &auditOn, AI: definition.ai, HTTP: &action.HTTPMeta{Method: "POST", Path: definition.path},
			},
		}); err != nil {
			return err
		}
	}
	return nil
}
