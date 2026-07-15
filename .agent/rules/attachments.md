# 租户隔离附件系统

- 附件元数据与绑定走 `modules/system` AttachmentService；字节只经 `ObjectStore` + 短时 transfer token 流式端点，禁止把文件内容塞进 JSON Action。
- `resource_type` 必须注册 `attachment.AccessPolicy`；未注册默认拒绝，禁止仅凭同租户放行。
- 上传/绑定/解绑/删除须鉴权且 `audit=true`；日志与审计禁止写 transfer token、文件内容与密钥。
- 删除先 `deleting` 再清理对象；有链接须先解绑；过期/失败用 `CleanupExpired` 补偿。
- 变更同步 `docs/development/attachments.md` 并 `agent-rules sync`。

详规：`docs/development/attachments.md`
