package system

import (
	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
)

func registerUserActions(a *app.App, svc *UserService) error {
	auditOn := true
	definitions := []struct {
		id          string
		path        string
		name        string
		description string
		handler     action.Handler
	}{
		{ActionUserList, "/api/v1/system/users/list", "用户列表", "分页查询当前租户用户", svc.List},
		{ActionUserGet, "/api/v1/system/users/get", "用户详情", "查询当前租户用户与角色", typedAction[userIDRequest](svc.Get)},
		{ActionUserCreate, "/api/v1/system/users/create", "创建用户", "创建当前租户用户并分配角色", typedAction[userCreateRequest](svc.Create)},
		{ActionUserUpdate, "/api/v1/system/users/update", "更新用户", "更新当前租户用户资料", typedAction[userUpdateRequest](svc.Update)},
		{ActionUserStatusUpdate, "/api/v1/system/users/status/update", "更新用户状态", "启停当前租户用户并使旧 Token 失效", typedAction[userStatusRequest](svc.UpdateStatus)},
		{ActionUserRolesUpdate, "/api/v1/system/users/roles/update", "更新用户角色", "事务化替换用户角色并使旧 Token 失效", typedAction[userRolesRequest](svc.UpdateRoles)},
		{ActionUserPasswordReset, "/api/v1/system/users/password/reset", "重置用户密码", "重置密码并使旧 Token 失效", typedAction[userPasswordRequest](svc.ResetPassword)},
	}
	for _, definition := range definitions {
		definition := definition
		if err := a.RegisterAction(action.Action{
			ID: definition.id, Handler: definition.handler,
			Meta: action.Metadata{
				Capability: "system", Name: definition.name, NameKey: "action." + definition.id + ".name",
				PermissionGroup: "system.user_management", PermissionGroupName: "用户管理", PermissionGroupNameKey: "permission_group.system.user_management",
				Description: definition.description, DescriptionKey: "action." + definition.id + ".description",
				Audit: &auditOn, HTTP: &action.HTTPMeta{Method: "POST", Path: definition.path},
			},
		}); err != nil {
			return err
		}
	}
	return nil
}
