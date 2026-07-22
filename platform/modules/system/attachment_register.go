package system

import (
	"context"
	"encoding/json"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
)

func registerAttachmentActions(a *app.App, svc *AttachmentService) error {
	auditOn := true
	auditOff := false
	type def struct {
		ID, Path, Name, NameKey, Desc, DescKey string
		Permission                             string
		Audit                                  *bool
		Fn                                     func(context.Context, json.RawMessage) (any, error)
	}
	actions := []def{
		{ActionAttachmentUploadPrepare, "/api/v1/system/attachments/upload/prepare", "附件上传准备", "action.system.attachment_upload_prepare.name", "创建 pending 附件并签发短时上传令牌", "action.system.attachment_upload_prepare.description", "", &auditOn, typedAction[attachmentPrepareRequest](svc.UploadPrepare)},
		{ActionAttachmentDownloadPrepare, "/api/v1/system/attachments/download/prepare", "附件下载准备", "action.system.attachment_download_prepare.name", "校验权限并签发短时下载令牌", "action.system.attachment_download_prepare.description", "", &auditOn, svc.DownloadPrepare},
		{ActionAttachmentBind, "/api/v1/system/attachments/bind", "绑定附件", "action.system.attachment_bind.name", "将附件绑定到业务资源", "action.system.attachment_bind.description", "", &auditOn, typedAction[attachmentBindRequest](svc.Bind)},
		{ActionAttachmentUnbind, "/api/v1/system/attachments/unbind", "解绑附件", "action.system.attachment_unbind.name", "解除附件与业务资源关联", "action.system.attachment_unbind.description", "", &auditOn, typedAction[attachmentBindRequest](svc.Unbind)},
		{ActionAttachmentListByResource, "/api/v1/system/attachments/list_by_resource", "按资源列附件", "action.system.attachment_list_by_resource.name", "按业务资源列出已绑定附件", "action.system.attachment_list_by_resource.description", "", &auditOn, svc.ListByResource},
		{ActionAttachmentDelete, "/api/v1/system/attachments/delete", "删除附件", "action.system.attachment_delete.name", "标记删除并清理对象", "action.system.attachment_delete.description", "", &auditOn, svc.Delete},
		{ActionAttachmentList, "/api/v1/system/attachments/list", "附件列表", "action.system.attachment_list.name", "分页查询租户附件（中心/我的）", "action.system.attachment_list.description", "", &auditOff, action.JSON(svc.list)},
		{ActionAttachmentGet, "/api/v1/system/attachments/get", "附件详情", "action.system.attachment_get.name", "查询附件详情与业务绑定", "action.system.attachment_get.description", "", &auditOff, svc.Get},
		{ActionAttachmentPolicyGet, "/api/v1/system/attachments/policy", "附件上传策略", "action.system.attachment_policy_get.name", "返回当前租户有效上传策略", "action.system.attachment_policy_get.description", ActionAttachmentUploadPrepare, &auditOff, svc.PolicyGet},
	}
	for _, item := range actions {
		item := item
		if err := a.RegisterAction(action.Action{
			ID:      item.ID,
			Handler: item.Fn,
			Meta: action.Metadata{
				Capability: "system", Name: item.Name, NameKey: item.NameKey,
				Description: item.Desc, DescriptionKey: item.DescKey,
				PermissionGroup: "system.attachment_management", PermissionGroupName: "附件管理",
				PermissionGroupNameKey: "permission_group.system.attachment_management",
				Permission:             item.Permission,
				AI:                     action.ExcludeFromAI(action.AIExcludePendingSafetyReview),
				Audit:                  item.Audit,
				HTTP:                   &action.HTTPMeta{Method: "POST", Path: item.Path},
			},
		}); err != nil {
			return err
		}
	}
	return nil
}
