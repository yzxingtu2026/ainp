# 前端统一附件组件

本文档约定对接 #18 附件协议的前端 Client、上传队列与可复用组件。Agent 要点：`.agent/rules/frontend-attachments.md`。

## 边界

| 层级 | 职责 | 不负责 |
|------|------|--------|
| #18 附件后端 | prepare / transfer / bind / download / 策略 | 前端 UI |
| 本 Issue 前端 | Client、队列、Picker/Field/List、Demo、附件中心 | Provider 分支 |
| 业务页面 | 传入已注册 `resourceType` / `resourceId` / `field` | 拼 object_key、长期缓存 token |

## Client

- Action：`src/service/api/attachment.ts`
- 二进制传输：`src/service/attachment/transfer.ts`（PUT/GET + 短时 token；支持上传进度与 AbortSignal）
- 类型：`src/typings/api/attachment.d.ts`
- 资源常量：`src/constants/attachment.ts` 的 `ATTACHMENT_RESOURCE_TYPES`（禁止任意字符串）

流程：`upload_prepare` → `putAttachmentTransfer`；`download_prepare` → `getAttachmentTransferBlob`。重试必须重新 prepare，禁止重放一次性 upload token。

## 上传队列

`useAttachmentUploadQueue`（`src/hooks/business/attachment-upload-queue.ts`）：

- 状态：`queued` / `preparing` / `uploading` / `completing` / `success` / `failed` / `cancelled`
- 并发限制、取消、失败重试、卸载中止
- 前端预校验对齐默认策略；后端拒绝仍以服务端 `message` 为准

## 组件

| 组件 | 用途 |
|------|------|
| `AttachmentPicker` | 选择器：我的上传 + 选择器内上传；业务主入口 |
| `AttachmentUpload` | 队列上传 UI（由选择器/中心复用） |
| `AttachmentList` | 列表、图片预览、下载、解绑/删除确认 |
| `AttachmentField` | 打开选择器并绑定到 resource；无 resourceId 时临时 pending |

有效上传策略：`POST .../attachments/policy`（设置覆盖 + 代码默认）。附件中心：`/system/attachments`（状态用 `CodeSelect` / `CodeTag` + CodeSet `system.attachment_status`；上传者展示 `uploader_name`）。

选择器「我的上传」为**宫格预览** + **滚动加载更多**（首屏 24 条，触底追加，软顶 200；搜索重置第 1 页）。图片经 `loadAttachmentPreviewBlob`（`thumb_status=ready` 时优先 `variant=thumb`，否则/失败降级原图）；非图显示类型图标。缩略图并发受限并在关闭时 `revokeObjectURL`。派生对象完整能力见 #36。

Demo：`/demo/attachments`（选择器主路径）。

## 安全

- transfer / download token 仅内存使用，不进 localStorage / sessionStorage / console
- 不发送 `tenant_id`、完整 object key 或未注册 `resource_type`
- 预览 Object URL 关闭/卸载时 `revokeObjectURL`

## 检查清单

- [ ] 业务只用统一 Client / 组件，不拼 Provider URL
- [ ] `resourceType` 来自常量且后端已注册 AccessPolicy
- [ ] `pnpm typecheck` / `pnpm build` 通过
- [ ] 变更同步本文档与 Agent 规则并 `agent-rules sync`
