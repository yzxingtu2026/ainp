package system

import "github.com/yzxingtu2026/ainp/platform/modules/system/access"

const (
	ActionRoleList              = access.ActionRoleList
	ActionRoleGet               = access.ActionRoleGet
	ActionRoleCreate            = access.ActionRoleCreate
	ActionRoleUpdate            = access.ActionRoleUpdate
	ActionRoleDelete            = access.ActionRoleDelete
	ActionRolePermissionsUpdate = access.ActionRolePermissionsUpdate
	ActionPermissionCatalog     = access.ActionPermissionCatalog

	ActionUserList          = access.ActionUserList
	ActionUserGet           = access.ActionUserGet
	ActionUserCreate        = access.ActionUserCreate
	ActionUserUpdate        = access.ActionUserUpdate
	ActionUserStatusUpdate  = access.ActionUserStatusUpdate
	ActionUserRolesUpdate   = access.ActionUserRolesUpdate
	ActionUserPasswordReset = access.ActionUserPasswordReset

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
	ActionAttachmentMineList        = "system.attachment_mine_list"
	ActionAttachmentGet             = "system.attachment_get"
	ActionAttachmentPolicyGet       = "system.attachment_policy_get"

	MenuRoleManagement    = access.MenuRoleManagement
	MenuUserManagement    = access.MenuUserManagement
	MenuSettingManagement = "system.settings"
	MenuAttachmentCenter  = "system.attachments"
	BuiltinAdminCode      = access.BuiltinAdminCode
	PageTypeSystemUsers   = access.PageTypeSystemUsers
	PageTypeSystemRoles   = access.PageTypeSystemRoles
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
	ActionAttachmentBind,
	ActionAttachmentUnbind,
	ActionAttachmentListByResource,
	ActionAttachmentDelete,
	ActionAttachmentList,
	ActionAttachmentGet,
}

// AdminMinimumPermissions returns the management permission baseline used by architecture contracts.
func AdminMinimumPermissions() []string {
	return append([]string(nil), adminMinimumPermissions...)
}
