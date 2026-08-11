# 租户隔离附件系统

- 附件元数据与绑定走 AttachmentService；业务事务只经强类型 `attachment.ResourceLinker` 原子 Bind/Unbind，禁止直写链接表；字节只经 ObjectStore + 短时 transfer token。
- upload prepare 显式协商 `raw|multipart`（缺省 raw）；Token 绑定 transport，旧无 claim Token 仅 raw。raw PUT 与 multipart POST 必须共用数据库原子 claim + `AttachmentService.CompleteUpload`，禁止复制完成链路或进程内锁。
- multipart Adapter 只接受唯一 `file` Part，使用流式 `multipart.Reader` 并限制 Body/元数据/Header；禁止 `ParseMultipartForm`、全量内存/临时文件、普通字段、嵌套 multipart、Content-Transfer-Encoding，以及信任 filename/Part MIME。
- 上传失败/取消后 Token 不可重放，附件进入 failed 或按 expires_at 清理；小程序失败必须重新 prepare。日志禁止 Token、Header/表单值、文件名、内容、object key 和底层错误全文。
- `resource_type` 必须注册 AccessPolicy；未注册默认拒绝。
- 账号头像使用 `system.account_profile` / `avatar`，只允许当前 AuthContext 主体绑定、读取和解绑自己的 ready 图片；账号表只保存 attachment ID。
- 上传策略经设置 `attachment.upload_policy` + PolicyProvider；业务入口用选择器，附件中心为管理页。
- Picker / 对话只调用 authenticated `system.attachment_mine_list` 查询本人附件；租户附件中心继续使用权限型 `system.attachment_list`，禁止为自助选择器开放管理列表。
- 派生：`thumb` 为标准变体（约 200²）；状态走独立 CodeSet `system.attachment_thumb_status`；原图 ready 不依赖派生。
- 生成经 Outbox `attachment.ready.v1` → Job `attachment.generate_thumb`；下载 `variant=thumb` 须鉴权；禁止暴露 OSS 处理 URL。
- 缩略图确定性输入错误必须 permanent，基础设施错误 retryable，取消/超时 cancelled；补偿只重放 pending 或 `thumb_retryable=true`，历史/永久 failed 禁止自动重放。
- 后处理扩展（转码/OCR/向量化等非本阶段）：仍走 Outbox→Job→独立 object_key→variant token；多产物再拆 derivatives 表，禁止堆列/另起协议。
- 删除须级联清理 `thumb_object_key`；变更同步 `docs/development/attachments.md` 并 `agent-rules sync`。
- 官方应用与公开 `platform/host` 必须由 system 模块统一注册 raw/multipart Transfer 路由；下游不得复制 Handler。

详规：`docs/development/attachments.md`
