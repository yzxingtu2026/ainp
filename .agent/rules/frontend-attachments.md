# 前端统一附件组件

- 业务页通过 `AttachmentField` / `AttachmentPicker` 对接附件协议；上传放在选择器内，禁止拼 object_key 或长期缓存 transfer token。
- Web/Vue 缺省使用 raw PUT；微信小程序显式 prepare `transport=multipart`，按响应的 URL/method/field_name 调用 `wx.uploadFile` / `Taro.uploadFile`，Token 只放 Header，filePath 直接交给平台且失败重新 prepare。
- 小程序禁止将文件全量读为 ArrayBuffer、信任 multipart filename/Part MIME、增加任意表单字段或复用一次性 Token；当前不支持分片/断点续传。
- `AttachmentPicker` 默认 `replace`；绑定型业务必须使用 `append` + `lockedSelectedIds`，锁定已关联项且确认时只返回新增项。
- 上传策略经 `attachment.upload_policy` 设置 + `PolicyProvider`；前端预校验拉 `/attachments/policy`。
- `resourceType` 必须用 `ATTACHMENT_RESOURCE_TYPES` 常量；附件中心仅管理页，不是跨业务文件库。
- 个人头像单选替换必须按“绑定新头像 → 更新资料 attachment ID → 解绑旧头像”执行，失败回滚新绑定；认证 store 只保存 ID。
- 宫格仅在 `thumb_status=ready` 时请求 `variant=thumb`；禁止未就绪时 silent 降级拉原图；禁止拼 OSS/ObjectStore URL。
- Picker 与 AI Composer 必须复用 `useAttachmentThumbStatusPolling`：仅在调用方可见且存在 pending 图/视缩略图时短间隔批量轮询当前用户已加载范围并按 ID 合并；全部完成、隐藏或卸载必须停止，不得重置选择、草稿和滚动位置。
- 图片、PDF、视频预览必须复用 `AttachmentPreview`，业务页禁止自建 MIME/`img`/`iframe`/`video` 分支或直接预览 Provider URL；组件只接收 attachment ID/标准 Item。
- 预览仅允许 `ready` 的 `image/*`、精确 `application/pdf`、`video/*`；其他状态/类型 fail-closed。Blob 全量预览上限 50 MiB，超限只提示下载；切换、关闭、卸载必须取消请求并回收 Object URL。
- 变更同步 `docs/development/frontend-attachments.md` 并 `agent-rules sync`。

详规：`docs/development/frontend-attachments.md`
