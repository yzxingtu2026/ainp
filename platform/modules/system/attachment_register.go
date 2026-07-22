package system

import (
	"context"
	"encoding/json"

	"github.com/yzxingtu2026/ainp/platform/framework/action"
	"github.com/yzxingtu2026/ainp/platform/framework/app"
	"github.com/yzxingtu2026/ainp/platform/framework/pagination"
)

func registerAttachmentActions(a *app.App, svc *AttachmentService) error {
	auditOn := true
	auditOff := false
	type def struct {
		ID, Path, Name, NameKey, Desc, DescKey string
		Permission                             string
		Audit                                  *bool
		Fn                                     func(context.Context, json.RawMessage) (any, error)
		AI                                     *action.AIMeta
	}
	actions := []def{
		{ActionAttachmentUploadPrepare, "/api/v1/system/attachments/upload/prepare", "附件上传准备", "action.system.attachment_upload_prepare.name", "创建 pending 附件并签发短时上传令牌", "action.system.attachment_upload_prepare.description", "", &auditOn, typedAction[attachmentPrepareRequest](svc.UploadPrepare), writeAIMeta("创建待上传附件记录并签发短时上传令牌", "准备上传附件", "附件上传已准备", "medium", action.SchemaFor[attachmentPrepareRequest](), action.SchemaFor[TransferPrepareView](), "附件", "上传", "准备")},
		{ActionAttachmentDownloadPrepare, "/api/v1/system/attachments/download/prepare", "附件下载准备", "action.system.attachment_download_prepare.name", "校验权限并签发短时下载令牌", "action.system.attachment_download_prepare.description", "", &auditOn, svc.DownloadPrepare, readAIMeta("校验附件访问权限并签发短时下载令牌", "准备下载附件", "附件下载已准备", action.SchemaFor[attachmentDownloadPrepareRequest](), action.SchemaFor[TransferPrepareView](), "附件", "下载", "准备")},
		{ActionAttachmentBind, "/api/v1/system/attachments/bind", "绑定附件", "action.system.attachment_bind.name", "将附件绑定到业务资源", "action.system.attachment_bind.description", "", &auditOn, typedAction[attachmentBindRequest](svc.Bind), writeAIMeta("将当前用户上传的就绪附件绑定到有权访问的业务资源", "绑定附件到业务资源", "附件已绑定", "medium", action.SchemaFor[attachmentBindRequest](), nil, "附件", "绑定", "业务资源")},
		{ActionAttachmentUnbind, "/api/v1/system/attachments/unbind", "解绑附件", "action.system.attachment_unbind.name", "解除附件与业务资源关联", "action.system.attachment_unbind.description", "", &auditOn, typedAction[attachmentBindRequest](svc.Unbind), writeAIMeta("解除附件与有权访问的业务资源之间的绑定", "解绑业务资源附件", "附件已解绑", "medium", action.SchemaFor[attachmentBindRequest](), nil, "附件", "解绑", "业务资源")},
		{ActionAttachmentListByResource, "/api/v1/system/attachments/list_by_resource", "按资源列附件", "action.system.attachment_list_by_resource.name", "按业务资源列出已绑定附件", "action.system.attachment_list_by_resource.description", "", &auditOn, svc.ListByResource, readAIMeta("列出当前用户有权访问的指定业务资源附件", "查询业务资源附件", "已返回业务资源附件", action.SchemaFor[attachmentListRequest](), nil, "附件", "业务资源", "列表")},
		{ActionAttachmentDelete, "/api/v1/system/attachments/delete", "删除附件", "action.system.attachment_delete.name", "标记删除并清理对象", "action.system.attachment_delete.description", "", &auditOn, svc.Delete, destructiveAIMeta("删除未绑定业务资源的附件并清理存储对象", "删除附件", "附件删除已受理", action.SchemaFor[attachmentIDRequest](), nil, "附件", "删除")},
		{ActionAttachmentList, "/api/v1/system/attachments/list", "附件列表", "action.system.attachment_list.name", "分页查询租户附件（中心/我的）", "action.system.attachment_list.description", "", &auditOff, action.JSON(svc.list), readAIMeta("分页查询当前用户上传或有权管理的租户附件", "查询附件列表", "已返回附件列表", action.SchemaFor[attachmentAdminListRequest](), action.SchemaFor[pagination.Result[AttachmentView]](), "附件", "列表", "查询")},
		{ActionAttachmentGet, "/api/v1/system/attachments/get", "附件详情", "action.system.attachment_get.name", "查询附件详情与业务绑定", "action.system.attachment_get.description", "", &auditOff, svc.Get, readAIMeta("查询指定附件详情及其业务资源绑定", "查询附件详情", "已返回附件详情", action.SchemaFor[attachmentIDRequest](), action.SchemaFor[AttachmentDetailView](), "附件", "详情", "绑定")},
		{ActionAttachmentPolicyGet, "/api/v1/system/attachments/policy", "附件上传策略", "action.system.attachment_policy_get.name", "返回当前租户有效上传策略", "action.system.attachment_policy_get.description", ActionAttachmentUploadPrepare, &auditOff, svc.PolicyGet, readAIMeta("查询当前租户生效的附件类型、大小和传输限制", "查询附件上传策略", "已返回附件上传策略", action.SchemaFor[struct{}](), nil, "附件", "上传策略", "查询")},
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
				AI:                     item.AI,
				Audit:                  item.Audit,
				HTTP:                   &action.HTTPMeta{Method: "POST", Path: item.Path},
			},
		}); err != nil {
			return err
		}
	}
	return nil
}
