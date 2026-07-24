# 租户隔离附件系统

- 附件元数据与绑定走 AttachmentService；业务事务只经强类型 `attachment.ResourceLinker` 原子 Bind/Unbind，禁止直写链接表；字节只经 ObjectStore + 短时 transfer token。
- `resource_type` 必须注册 AccessPolicy；未注册默认拒绝。
- 用户头像使用 `system.user_profile` / `avatar`，只允许当前 AuthContext 用户绑定、读取和解绑自己的 ready 图片；用户表只保存 attachment ID。
- 上传策略经设置 `attachment.upload_policy` + PolicyProvider；业务入口用选择器，附件中心为管理页。
- Picker / 对话只调用 authenticated `system.attachment_mine_list` 查询本人附件；租户附件中心继续使用权限型 `system.attachment_list`，禁止为自助选择器开放管理列表。
- 派生：`thumb` 为标准变体（约 200²）；状态走独立 CodeSet `system.attachment_thumb_status`；原图 ready 不依赖派生。
- 生成经 Outbox `attachment.ready.v1` → Job `attachment.generate_thumb`；下载 `variant=thumb` 须鉴权；禁止暴露 OSS 处理 URL。
- 后处理扩展（转码/OCR/向量化等非本阶段）：仍走 Outbox→Job→独立 object_key→variant token；多产物再拆 derivatives 表，禁止堆列/另起协议。
- 删除须级联清理 `thumb_object_key`；变更同步 `docs/development/attachments.md` 并 `agent-rules sync`。

详规：`docs/development/attachments.md`
