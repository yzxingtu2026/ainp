package system

const (
	ActionRoleList              = "system.role_list"
	ActionRoleGet               = "system.role_get"
	ActionRoleCreate            = "system.role_create"
	ActionRoleUpdate            = "system.role_update"
	ActionRoleDelete            = "system.role_delete"
	ActionRolePermissionsUpdate = "system.role_permissions_update"
	ActionPermissionCatalog     = "system.permission_catalog"

	MenuRoleManagement = "system.roles"
	BuiltinAdminCode   = "admin"
)

var adminMinimumPermissions = []string{
	"system.auth_me",
	"system.auth_permissions",
	ActionRoleList,
	ActionRoleGet,
	ActionRoleCreate,
	ActionRoleUpdate,
	ActionRoleDelete,
	ActionRolePermissionsUpdate,
	ActionPermissionCatalog,
}
