# 前端统一附件组件

本文档约定对接 #18 / #36 附件协议的前端 Client、上传队列与可复用组件。Agent 要点：`.agent/rules/frontend-attachments.md`。

## 边界

| 层级 | 职责 | 不负责 |
|------|------|--------|
| 附件后端 | prepare / transfer / bind / download / 策略 / thumb 派生 | 前端 UI |
| 本 Issue 前端 | Client、队列、Picker/Field/List、Demo、附件中心、宫格 thumb | Provider 分支 |
| 业务页面 | 传入已注册 `resourceType` / `resourceId` / `field` | 拼 object_key、长期缓存 token、OSS 处理 URL |

## Client

- Action：`src/service/api/attachment.ts`
- 二进制传输：`src/service/attachment/transfer.ts`（PUT/GET + 短时 token；支持上传进度与 AbortSignal）
- 类型：`src/typings/api/attachment.d.ts`（含 `thumb_status`、`DownloadVariant`）
- 资源常量：`src/constants/attachment.ts` 的 `ATTACHMENT_RESOURCE_TYPES`（禁止任意字符串）

流程：`upload_prepare` → `putAttachmentTransfer`；`download_prepare({ variant })` → `getAttachmentTransferBlob`。重试必须重新 prepare，禁止重放一次性 upload token。

变体约定：

| 场景 | variant | 行为 |
|------|---------|------|
| 宫格缩略图 | `thumb` | 仅当列表项 `thumb_status === 'ready'`；未就绪显示类型图标/生成中 Spin，**不**降级拉原图 |
| 业务大图/下载 | `original` | `downloadAttachmentById` / `useAttachmentPreview` |

`loadAttachmentPreviewBlob`：thumb 未就绪返回 `null`（占位）；禁止 silent 回落原图撑爆宫格。

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

`AttachmentPicker` 必须显式区分选择语义：默认 `replace` 返回最终选择集合；绑定型业务使用 `append` + `lockedSelectedIds`，已关联项显示锁定状态且确认事件只返回本次新增项。解除关联由业务列表独立操作，禁止把锁定项作为新增请求重复提交。

AI Builder 的预览消息复用 `AttachmentPicker` 与上传队列，但不伪造业务 `resource_type`：只允许当前上传者有权访问、状态 ready 且符合 #73 图片限制的附件，并将 attachment ID + revision + detail 作为 `input_image` Content Part 提交。当前不支持通用文件、音视频或 PDF 作为模型输入。

有效上传策略：`POST .../attachments/policy`（设置覆盖 + 代码默认）。附件中心：`/system/attachments`（状态用 `CodeSelect` / `CodeTag` + CodeSet `system.attachment_status`；上传者展示 `uploader_name`）。

选择器「我的上传」为**宫格预览** + **滚动加载更多**（首屏 24 条，触底追加，软顶 200；搜索重置第 1 页）。图/视在 `thumb_status=ready` 时经 `variant=thumb` 加载；`pending` 显示 Spin；否则类型图标。缩略图并发受限（默认 3）并在关闭时 `revokeObjectURL`。token 不进持久存储与日志。

Demo：`/demo/attachments`（选择器主路径）。

## 安全

- transfer / download token 仅内存使用，不进 localStorage / sessionStorage / console
- 不发送 `tenant_id`、完整 object key 或未注册 `resource_type`
- **禁止**拼 ObjectStore / OSS URL 或厂商图片处理参数
- 预览 Object URL 关闭/卸载时 `revokeObjectURL`

## 检查清单

- [ ] 业务只用统一 Client / 组件，不拼 Provider URL
- [ ] `resourceType` 来自常量且后端已注册 AccessPolicy
- [ ] 宫格不在 thumb 未就绪时拉原图
- [ ] `pnpm typecheck` / `pnpm build` 通过
- [ ] 变更同步本文档与 Agent 规则并 `agent-rules sync`
