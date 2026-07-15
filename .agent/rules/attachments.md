# 租户隔离附件系统

- 附件元数据与绑定走 AttachmentService；字节只经 ObjectStore + 短时 transfer token。
- `resource_type` 必须注册 AccessPolicy；未注册默认拒绝。
- 上传策略经设置 `attachment.upload_policy` + PolicyProvider；业务入口用选择器，附件中心为管理页。
- 派生：`thumb` 为标准变体（约 200²）；状态走独立 CodeSet `system.attachment_thumb_status`；原图 ready 不依赖派生。
- 生成经 Outbox `attachment.ready.v1` → Job `attachment.generate_thumb`；下载 `variant=thumb` 须鉴权；禁止暴露 OSS 处理 URL。
- 删除须级联清理 `thumb_object_key`；变更同步 `docs/development/attachments.md` 并 `agent-rules sync`。

详规：`docs/development/attachments.md`
