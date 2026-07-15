# 租户隔离附件系统

- 附件元数据与绑定走 AttachmentService；字节只经 ObjectStore + 短时 transfer token。
- `resource_type` 必须注册 AccessPolicy；未注册默认拒绝。
- 上传策略经设置 `attachment.upload_policy` + PolicyProvider；业务入口用选择器，附件中心为管理页。
- 变更同步 `docs/development/attachments.md` 并 `agent-rules sync`。

详规：`docs/development/attachments.md`
