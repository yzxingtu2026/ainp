package system

import (
	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
)

func registerRoleActions(a *app.App, svc *RoleService) error {
	auditOn := true
	definitions := []struct {
		id          string
		path        string
		name        string
		description string
		handler     action.Handler
	}{
		{ActionRoleList, "/api/v1/system/roles/list", "角色列表", "分页查询当前租户角色", action.JSON(svc.list)},
		{ActionRoleGet, "/api/v1/system/roles/get", "角色详情", "查询当前租户角色与权限", typedAction[roleIDRequest](svc.Get)},
		{ActionRoleCreate, "/api/v1/system/roles/create", "创建角色", "创建当前租户角色", typedAction[roleCreateRequest](svc.Create)},
		{ActionRoleUpdate, "/api/v1/system/roles/update", "更新角色", "更新当前租户角色资料与状态", typedAction[roleUpdateRequest](svc.Update)},
		{ActionRoleDelete, "/api/v1/system/roles/delete", "删除角色", "删除未分配用户的非内置角色", typedAction[roleIDRequest](svc.Delete)},
		{ActionRolePermissionsUpdate, "/api/v1/system/roles/permissions/update", "更新角色权限", "事务化替换角色权限并使旧 Token 失效", typedAction[rolePermissionsRequest](svc.UpdatePermissions)},
		{ActionPermissionCatalog, "/api/v1/system/permissions/catalog", "权限目录", "从 Action Registry 返回可分配权限目录", svc.PermissionCatalog},
	}
	for _, definition := range definitions {
		definition := definition
		if err := a.RegisterAction(action.Action{
			ID: definition.id, Handler: definition.handler,
			Meta: action.Metadata{
				Capability: "system", Name: definition.name, NameKey: "action." + definition.id + ".name",
				PermissionGroup: "system.role_management", PermissionGroupName: "权限管理", PermissionGroupNameKey: "permission_group.system.role_management",
				Description: definition.description, DescriptionKey: "action." + definition.id + ".description",
				Audit: &auditOn, AI: roleActionAI(definition.id), HTTP: &action.HTTPMeta{Method: "POST", Path: definition.path},
			},
		}); err != nil {
			return err
		}
	}
	return nil
}
