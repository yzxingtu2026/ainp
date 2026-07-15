package system

import (
	"context"
	"encoding/json"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
)

func registerAttachmentActions(a *app.App, svc *AttachmentService) error {
	auditOn := true
	actions := []struct {
		ID, Path, Name, NameKey, Desc, DescKey string
		Fn                                     func(context.Context, json.RawMessage) (any, error)
	}{
		{ActionAttachmentUploadPrepare, "/api/v1/system/attachments/upload/prepare", "附件上传准备", "action.system.attachment_upload_prepare.name", "创建 pending 附件并签发短时上传令牌", "action.system.attachment_upload_prepare.description", svc.UploadPrepare},
		{ActionAttachmentDownloadPrepare, "/api/v1/system/attachments/download/prepare", "附件下载准备", "action.system.attachment_download_prepare.name", "校验权限并签发短时下载令牌", "action.system.attachment_download_prepare.description", svc.DownloadPrepare},
		{ActionAttachmentBind, "/api/v1/system/attachments/bind", "绑定附件", "action.system.attachment_bind.name", "将附件绑定到业务资源", "action.system.attachment_bind.description", svc.Bind},
		{ActionAttachmentUnbind, "/api/v1/system/attachments/unbind", "解绑附件", "action.system.attachment_unbind.name", "解除附件与业务资源关联", "action.system.attachment_unbind.description", svc.Unbind},
		{ActionAttachmentListByResource, "/api/v1/system/attachments/list_by_resource", "按资源列附件", "action.system.attachment_list_by_resource.name", "按业务资源列出已绑定附件", "action.system.attachment_list_by_resource.description", svc.ListByResource},
		{ActionAttachmentDelete, "/api/v1/system/attachments/delete", "删除附件", "action.system.attachment_delete.name", "标记删除并清理对象", "action.system.attachment_delete.description", svc.Delete},
	}
	for _, def := range actions {
		def := def
		if err := a.RegisterAction(action.Action{
			ID:      def.ID,
			Handler: def.Fn,
			Meta: action.Metadata{
				Capability: "system", Name: def.Name, NameKey: def.NameKey,
				Description: def.Desc, DescriptionKey: def.DescKey,
				PermissionGroup: "system.attachment_management", PermissionGroupName: "附件管理",
				PermissionGroupNameKey: "permission_group.system.attachment_management",
				Audit:                  &auditOn,
				HTTP:                   &action.HTTPMeta{Method: "POST", Path: def.Path},
			},
		}); err != nil {
			return err
		}
	}
	return nil
}
