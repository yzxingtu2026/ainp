package system

import (
	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/pagination"
)

func registerUserActions(a *app.App, svc *UserService) error {
	auditOn := true
	definitions := []struct {
		id          string
		path        string
		name        string
		description string
		handler     action.Handler
		ai          *action.AIMeta
	}{
		{ActionUserList, "/api/v1/system/users/list", "用户列表", "分页查询当前租户用户", action.JSON(svc.list), &action.AIMeta{ExposeTool: true, Risk: "low", SideEffect: "read", Summary: "查询当前租户的用户列表，可按关键词、状态和角色筛选", HumanDescription: "查询系统用户列表", Tags: []string{"用户", "用户管理", "列表", "查询"}, ResultSummary: "已返回当前用户有权查看的系统用户列表", InputSchema: action.SchemaFor[userListRequest](), OutputSchema: action.SchemaFor[pagination.Result[UserView]]()}},
		{ActionUserGet, "/api/v1/system/users/get", "用户详情", "查询当前租户用户与角色", typedAction[userIDRequest](svc.Get), &action.AIMeta{ExposeTool: true, Risk: "low", SideEffect: "read", Summary: "按用户 ID 查询当前租户中的用户资料和角色", HumanDescription: "查询系统用户详情", Tags: []string{"用户", "用户管理", "详情", "角色"}, ResultSummary: "已返回当前用户有权查看的系统用户详情", InputSchema: action.SchemaFor[userIDRequest](), OutputSchema: action.SchemaFor[UserView]()}},
		{ActionUserCreate, "/api/v1/system/users/create", "创建用户", "创建当前租户用户并分配角色", typedAction[userCreateRequest](svc.Create), nil},
		{ActionUserUpdate, "/api/v1/system/users/update", "更新用户", "更新当前租户用户资料", typedAction[userUpdateRequest](svc.Update), nil},
		{ActionUserStatusUpdate, "/api/v1/system/users/status/update", "更新用户状态", "启停当前租户用户并使旧 Token 失效", typedAction[userStatusRequest](svc.UpdateStatus), nil},
		{ActionUserRolesUpdate, "/api/v1/system/users/roles/update", "更新用户角色", "事务化替换用户角色并使旧 Token 失效", typedAction[userRolesRequest](svc.UpdateRoles), nil},
		{ActionUserPasswordReset, "/api/v1/system/users/password/reset", "重置用户密码", "重置密码并使旧 Token 失效", typedAction[userPasswordRequest](svc.ResetPassword), nil},
	}
	for _, definition := range definitions {
		definition := definition
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
