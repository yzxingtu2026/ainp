package system

const (
	ActionRoleList              = "system.role_list"
	ActionRoleGet               = "system.role_get"
	ActionRoleCreate            = "system.role_create"
	ActionRoleUpdate            = "system.role_update"
	ActionRoleDelete            = "system.role_delete"
	ActionRolePermissionsUpdate = "system.role_permissions_update"
	ActionPermissionCatalog     = "system.permission_catalog"

	ActionUserList          = "system.user_list"
	ActionUserGet           = "system.user_get"
	ActionUserCreate        = "system.user_create"
	ActionUserUpdate        = "system.user_update"
	ActionUserStatusUpdate  = "system.user_status_update"
	ActionUserRolesUpdate   = "system.user_roles_update"
	ActionUserPasswordReset = "system.user_password_reset"

	ActionSettingList   = "system.setting_list"
	ActionSettingGet    = "system.setting_get"
	ActionSettingUpdate = "system.setting_update"
	ActionSettingReset  = "system.setting_reset"

	ActionProfileGet            = "system.profile_get"
	ActionProfileUpdate         = "system.profile_update"
	ActionProfilePasswordUpdate = "system.profile_password_update"
	ActionUserPreferencesGet    = "system.user_preferences_get"
	ActionUserPreferencesUpdate = "system.user_preferences_update"
	ActionMCPConnectionInfo     = "system.mcp_connection_info"

	ActionAttachmentUploadPrepare   = "system.attachment_upload_prepare"
	ActionAttachmentDownloadPrepare = "system.attachment_download_prepare"
	ActionAttachmentBind            = "system.attachment_bind"
	ActionAttachmentUnbind          = "system.attachment_unbind"
	ActionAttachmentListByResource  = "system.attachment_list_by_resource"
	ActionAttachmentDelete          = "system.attachment_delete"
	ActionAttachmentList            = "system.attachment_list"
	ActionAttachmentGet             = "system.attachment_get"
	ActionAttachmentPolicyGet       = "system.attachment_policy_get"

	MenuRoleManagement    = "system.roles"
	MenuUserManagement    = "system.users"
	MenuSettingManagement = "system.settings"
	MenuAttachmentCenter  = "system.attachments"
	BuiltinAdminCode      = "admin"
)

var adminMinimumPermissions = []string{
	ActionRoleList,
	ActionRoleGet,
	ActionRoleCreate,
	ActionRoleUpdate,
	ActionRoleDelete,
	ActionRolePermissionsUpdate,
	ActionPermissionCatalog,
	ActionUserList,
	ActionUserGet,
	ActionUserCreate,
	ActionUserUpdate,
	ActionUserStatusUpdate,
	ActionUserRolesUpdate,
	ActionUserPasswordReset,
	ActionSettingList,
	ActionSettingGet,
	ActionSettingUpdate,
	ActionSettingReset,
	ActionAttachmentUploadPrepare,
	ActionAttachmentDownloadPrepare,
	ActionAttachmentBind,
	ActionAttachmentUnbind,
	ActionAttachmentListByResource,
	ActionAttachmentDelete,
	ActionAttachmentList,
	ActionAttachmentGet,
}

// AdminMinimumPermissions 返回内置管理员必须保留的最小权限副本（测试与文档对齐用）。
func AdminMinimumPermissions() []string {
	return append([]string(nil), adminMinimumPermissions...)
}
