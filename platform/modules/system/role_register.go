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
		{ActionRoleList, "/api/v1/system/roles/list", "角色列表", "分页查询当前租户角色", action.JSON(svc.list), &action.AIMeta{ExposeTool: true, Risk: "low", SideEffect: "read", Summary: "查询当前租户的角色列表，可按关键词和状态筛选", HumanDescription: "查询系统角色列表", Tags: []string{"角色", "权限管理", "列表", "查询"}, ResultSummary: "已返回当前用户有权查看的系统角色列表", InputSchema: action.SchemaFor[roleListRequest](), OutputSchema: action.SchemaFor[pagination.Result[RoleView]]()}},
		{ActionRoleGet, "/api/v1/system/roles/get", "角色详情", "查询当前租户角色与权限", typedAction[roleIDRequest](svc.Get), &action.AIMeta{ExposeTool: true, Risk: "low", SideEffect: "read", Summary: "按角色 ID 查询当前租户中的角色详情和权限", HumanDescription: "查询系统角色详情", Tags: []string{"角色", "权限管理", "详情", "权限"}, ResultSummary: "已返回当前用户有权查看的系统角色详情", InputSchema: action.SchemaFor[roleIDRequest](), OutputSchema: action.SchemaFor[RoleView]()}},
		{ActionRoleCreate, "/api/v1/system/roles/create", "创建角色", "创建当前租户角色", typedAction[roleCreateRequest](svc.Create), nil},
		{ActionRoleUpdate, "/api/v1/system/roles/update", "更新角色", "更新当前租户角色资料与状态", typedAction[roleUpdateRequest](svc.Update), nil},
		{ActionRoleDelete, "/api/v1/system/roles/delete", "删除角色", "删除未分配用户的非内置角色", typedAction[roleIDRequest](svc.Delete), nil},
		{ActionRolePermissionsUpdate, "/api/v1/system/roles/permissions/update", "更新角色权限", "事务化替换角色权限并使旧 Token 失效", typedAction[rolePermissionsRequest](svc.UpdatePermissions), nil},
		{ActionPermissionCatalog, "/api/v1/system/permissions/catalog", "权限目录", "从 Action Registry 返回可分配权限目录", svc.PermissionCatalog, &action.AIMeta{ExposeTool: true, Risk: "low", SideEffect: "read", Summary: "查询当前租户可分配的权限目录及其功能分组", HumanDescription: "查询系统权限目录", Tags: []string{"权限", "权限目录", "功能", "查询"}, ResultSummary: "已返回当前用户有权查看的系统权限目录", InputSchema: action.SchemaFor[struct{}](), OutputSchema: action.SchemaFor[permissionCatalogResponse]()}},
	}
	for _, definition := range definitions {
		definition := definition
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
