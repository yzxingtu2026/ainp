# 前端统一附件组件

- 业务页通过 `AttachmentField` / `AttachmentPicker` 对接附件协议；上传放在选择器内，禁止拼 object_key 或长期缓存 transfer token。
- `AttachmentPicker` 默认 `replace`；绑定型业务必须使用 `append` + `lockedSelectedIds`，锁定已关联项且确认时只返回新增项。
- 上传策略经 `attachment.upload_policy` 设置 + `PolicyProvider`；前端预校验拉 `/attachments/policy`。
- `resourceType` 必须用 `ATTACHMENT_RESOURCE_TYPES` 常量；附件中心仅管理页，不是跨业务文件库。
- 个人头像单选替换必须按“绑定新头像 → 更新资料 attachment ID → 解绑旧头像”执行，失败回滚新绑定；认证 store 只保存 ID。
- 宫格仅在 `thumb_status=ready` 时请求 `variant=thumb`；禁止未就绪时 silent 降级拉原图；禁止拼 OSS/ObjectStore URL。
- Picker 仅在可见、“我的上传”且存在 pending 图/视缩略图时短间隔批量轮询已加载范围，按 ID 合并状态；全部完成、切 Tab、关闭或卸载必须停止，不得重置选择和滚动位置。
- 变更同步 `docs/development/frontend-attachments.md` 并 `agent-rules sync`。

详规：`docs/development/frontend-attachments.md`
