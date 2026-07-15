# 前端统一附件组件

- 业务页通过 `AttachmentField` / `AttachmentUpload` / `AttachmentList` 与 `service/api/attachment` 对接 #18；禁止拼 object_key、Provider URL 或长期缓存 transfer token。
- `resourceType` 必须用 `ATTACHMENT_RESOURCE_TYPES` 常量；无 `resourceId` 时仅临时上传，业务保存后再 `bindPending`。
- 上传重试必须重新 prepare；预览 Object URL 须在关闭/卸载时释放。
- 不做生产「附件中心」；变更同步 `docs/development/frontend-attachments.md` 并 `agent-rules sync`。

详规：`docs/development/frontend-attachments.md`
