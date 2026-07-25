package system

import (
	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/pagination"
)

type permissionCatalogResponse struct {
	Groups []PermissionCatalogGroup `json:"groups"`
}

func registerRoleActions(a *app.App, svc *RoleService) error {
	auditOn := true
	definitions := []struct {
		id          string
		path        string
		name        string
		description string
		handler     action.Handler
		ai          *action.AIMeta
	}{
		{ActionRoleList, "/api/v1/system/roles/list", "角色列表", "分页查询当前租户角色", action.JSON(svc.list), readAIMeta("查询当前租户的角色列表，可按关键词和状态筛选", "查询系统角色列表", "查询当前租户的角色列表，可按关键词和状态筛选", "已返回当前用户有权查看的系统角色列表", action.InputSchemaFor[roleListRequest](), action.SchemaFor[pagination.Result[RoleView]](), "角色", "权限管理", "列表", "查询")},
		{ActionRoleGet, "/api/v1/system/roles/get", "角色详情", "查询当前租户角色与权限", typedAction[roleIDRequest](svc.Get), readAIMeta("按角色 ID 查询当前租户中的角色详情和权限", "查询系统角色详情", "按角色 ID 查询当前租户中的角色详情和权限", "已返回当前用户有权查看的系统角色详情", action.InputSchemaFor[roleIDRequest](), action.SchemaFor[RoleView](), "角色", "权限管理", "详情", "权限")},
		{ActionRoleCreate, "/api/v1/system/roles/create", "创建角色", "创建当前租户角色", typedAction[roleCreateRequest](svc.Create), writeAIMeta("在当前租户创建角色并设置初始权限", "创建系统角色", "在当前租户创建角色并设置初始权限", "角色已创建", "high", action.InputSchemaFor[roleCreateRequest](), action.SchemaFor[RoleView](), "角色", "创建", "权限")},
		{ActionRoleUpdate, "/api/v1/system/roles/update", "更新角色", "更新当前租户角色资料与状态", typedAction[roleUpdateRequest](svc.Update), writeAIMeta("更新当前租户角色的名称、说明和状态", "更新系统角色", "更新当前租户角色的名称、说明和状态", "角色已更新", "high", action.InputSchemaFor[roleUpdateRequest](), action.SchemaFor[RoleView](), "角色", "更新", "状态")},
		{ActionRoleDelete, "/api/v1/system/roles/delete", "删除角色", "删除未分配用户的非内置角色", typedAction[roleIDRequest](svc.Delete), destructiveAIMeta("删除当前租户中未绑定用户的非内置角色", "删除系统角色", "删除当前租户中未绑定用户的非内置角色", "角色已删除", action.InputSchemaFor[roleIDRequest](), nil, "角色", "删除", "权限")},
		{ActionRolePermissionsUpdate, "/api/v1/system/roles/permissions/update", "更新角色权限", "事务化替换角色权限并使旧 Token 失效", typedAction[rolePermissionsRequest](svc.UpdatePermissions), writeAIMeta("替换当前租户指定角色的权限集合，相关用户旧会话会失效", "更新系统角色权限", "替换当前租户指定角色的权限集合，相关用户旧会话会失效", "角色权限已更新", "high", action.InputSchemaFor[rolePermissionsRequest](), action.SchemaFor[RoleView](), "角色", "权限", "更新")},
		{ActionPermissionCatalog, "/api/v1/system/permissions/catalog", "权限目录", "从 Action Registry 返回可分配权限目录", svc.PermissionCatalog, readAIMeta("查询当前租户可分配的权限目录及其功能分组", "查询系统权限目录", "查询当前租户可分配的权限目录及其功能分组", "已返回当前用户有权查看的系统权限目录", action.InputSchemaFor[struct{}](), action.SchemaFor[permissionCatalogResponse](), "权限", "权限目录", "功能", "查询")},
	}
	for _, definition := range definitions {
		definition := definition
		if definition.id == ActionRoleCreate || definition.id == ActionRolePermissionsUpdate {
			definition.ai.DynamicReferences = []action.DynamicReference{{Field: "permissions[]", ResolverID: "system.assignable_permissions", DiscoveryAction: ActionPermissionCatalog, RequireEvidence: true, MaxInvalidValues: 8, MaxInvalidBytes: 512}}
		}
		if err := a.RegisterAction(action.Action{
			ID: definition.id, Handler: definition.handler,
			Meta: action.Metadata{
				Capability: "system", Name: definition.name, NameKey: "action." + definition.id + ".name",
				PermissionGroup: "system.role_management", PermissionGroupName: "权限管理", PermissionGroupNameKey: "permission_group.system.role_management",
				Description: definition.description, DescriptionKey: "action." + definition.id + ".description",
				Audit: &auditOn, AI: definition.ai, HTTP: &action.HTTPMeta{Method: "POST", Path: definition.path},
			},
		}); err != nil {
			return err
		}
	}
	return nil
}
