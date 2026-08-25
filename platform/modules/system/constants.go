package system

import (
	"github.com/yzxingtu2026/ainp/platform/modules/system/access"
	"github.com/yzxingtu2026/ainp/platform/modules/system/attachments"
	"github.com/yzxingtu2026/ainp/platform/modules/system/notifications"
	"github.com/yzxingtu2026/ainp/platform/modules/system/settings"
)

const (
	ActionRoleList              = access.ActionRoleList
	ActionRoleGet               = access.ActionRoleGet
	ActionRoleCreate            = access.ActionRoleCreate
	ActionRoleUpdate            = access.ActionRoleUpdate
	ActionRoleDelete            = access.ActionRoleDelete
	ActionRolePermissionsUpdate = access.ActionRolePermissionsUpdate
	ActionPermissionCatalog     = access.ActionPermissionCatalog

	ActionAccountList          = access.ActionAccountList
	ActionAccountGet           = access.ActionAccountGet
	ActionAccountCreate        = access.ActionAccountCreate
	ActionAccountUpdate        = access.ActionAccountUpdate
	ActionAccountStatusUpdate  = access.ActionAccountStatusUpdate
	ActionAccountRolesUpdate   = access.ActionAccountRolesUpdate
	ActionAccountPasswordReset = access.ActionAccountPasswordReset

	ActionSettingList         = settings.ActionSettingList
	ActionSettingGet          = settings.ActionSettingGet
	ActionSettingUpdate       = settings.ActionSettingUpdate
	ActionSettingSecretUpdate = settings.ActionSettingSecretUpdate
	ActionSettingReset        = settings.ActionSettingReset

	ActionProfileGet                 = settings.ActionProfileGet
	ActionProfileUpdate              = settings.ActionProfileUpdate
	ActionProfilePasswordUpdate      = settings.ActionProfilePasswordUpdate
	ActionUserPreferencesGet         = settings.ActionUserPreferencesGet
	ActionUserPreferencesUpdate      = settings.ActionUserPreferencesUpdate
	ActionMCPConnectionInfo          = settings.ActionMCPConnectionInfo
	RuntimePreferencesKey            = settings.RuntimePreferencesKey
	SettingKeyAttachmentUploadPolicy = settings.SettingKeyAttachmentUploadPolicy

	ActionAttachmentUploadPrepare        = attachments.ActionAttachmentUploadPrepare
	ActionAttachmentDownloadPrepare      = attachments.ActionAttachmentDownloadPrepare
	ActionAttachmentBind                 = attachments.ActionAttachmentBind
	ActionAttachmentUnbind               = attachments.ActionAttachmentUnbind
	ActionAttachmentListByResource       = attachments.ActionAttachmentListByResource
	ActionAttachmentDelete               = attachments.ActionAttachmentDelete
	ActionAttachmentList                 = attachments.ActionAttachmentList
	ActionAttachmentMineList             = attachments.ActionAttachmentMineList
	ActionAttachmentGet                  = attachments.ActionAttachmentGet
	ActionAttachmentContentInspect       = attachments.ActionAttachmentContentInspect
	ActionAttachmentPolicyGet            = attachments.ActionAttachmentPolicyGet
	AccountProfileAttachmentResourceType = attachments.AccountProfileAttachmentResourceType
	AccountProfileAvatarField            = attachments.AccountProfileAvatarField

	ActionNotificationList            = notifications.ActionNotificationList
	ActionNotificationUnreadCount     = notifications.ActionNotificationUnreadCount
	ActionNotificationMarkRead        = notifications.ActionNotificationMarkRead
	ActionNotificationMarkAllRead     = notifications.ActionNotificationMarkAllRead
	ActionNotificationArchive         = notifications.ActionNotificationArchive
	ActionNotificationUnarchive       = notifications.ActionNotificationUnarchive
	ActionNotificationTemplateList    = notifications.ActionNotificationTemplateList
	ActionNotificationTemplateUpdate  = notifications.ActionNotificationTemplateUpdate
	ActionNotificationTemplateReset   = notifications.ActionNotificationTemplateReset
	ActionNotificationTemplatePreview = notifications.ActionNotificationTemplatePreview

	MenuRoleManagement     = access.MenuRoleManagement
	MenuAccountManagement  = access.MenuAccountManagement
	MenuSettingManagement  = "system.settings"
	MenuAttachmentCenter   = "system.attachments"
	BuiltinAdminCode       = access.BuiltinAdminCode
	PageTypeSystemAccounts = access.PageTypeSystemAccounts
	PageTypeSystemRoles    = access.PageTypeSystemRoles
)

var adminMinimumPermissions = []string{
	ActionRoleList,
	ActionRoleGet,
	ActionRoleCreate,
	ActionRoleUpdate,
	ActionRoleDelete,
	ActionRolePermissionsUpdate,
	ActionPermissionCatalog,
	ActionAccountList,
	ActionAccountGet,
	ActionAccountCreate,
	ActionAccountUpdate,
	ActionAccountStatusUpdate,
	ActionAccountRolesUpdate,
	ActionAccountPasswordReset,
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
