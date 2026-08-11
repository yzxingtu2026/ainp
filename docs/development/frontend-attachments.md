# 前端统一附件组件

本文档约定对接 #18 / #36 附件协议的前端 Client、上传队列与可复用组件。Agent 要点：`.agent/rules/frontend-attachments.md`。

## 边界

| 层级 | 职责 | 不负责 |
|------|------|--------|
| 附件后端 | prepare / transfer / bind / download / 策略 / thumb 派生 | 前端 UI |
| 本 Issue 前端 | Client、队列、Picker/Field/List/Preview、Demo、附件中心、宫格 thumb | Provider 分支 |
| 业务页面 | 传入已注册 `resourceType` / `resourceId` / `field` | 拼 object_key、长期缓存 token、OSS 处理 URL |

## Client

- Action：`src/service/api/attachment.ts`
- 二进制传输：`src/service/attachment/transfer.ts`（PUT/GET + 短时 token；支持上传进度与 AbortSignal）
- 类型：`src/typings/api/attachment.d.ts`（含 `thumb_status`、`DownloadVariant`）
- 资源常量：`src/constants/attachment.ts` 的 `ATTACHMENT_RESOURCE_TYPES`（禁止任意字符串）

流程：`upload_prepare` → raw `putAttachmentTransfer` 或小程序 multipart Adapter；`download_prepare({ variant })` → `getAttachmentTransferBlob`。Web/Vue 缺省 `transport=raw`，保持现有 PUT 行为；小程序必须显式请求 `transport=multipart`，并使用响应中的 `method=POST` 与 `field_name=file`。重试必须重新 prepare，禁止重放一次性 upload token。

### 微信小程序 / Taro

文件路径直接交给平台原生上传任务，不用 `FileSystemManager` 把完整影像读成 ArrayBuffer。Token 只放 Header，`name` 使用 prepare 返回值：

```ts
const prepared = await prepareAttachmentUpload({
  original_name: displayName,
  content_type: mimeHint,
  size,
  transport: 'multipart'
});
if (prepared.method !== 'POST' || !prepared.upload_url || !prepared.field_name) {
  throw new Error('unsupported attachment transfer contract');
}

const task = Taro.uploadFile({
  url: apiBase + prepared.upload_url,
  name: prepared.field_name,
  filePath,
  header: { Authorization: `Bearer ${prepared.token}` },
  success(result) {
    const envelope = JSON.parse(result.data) as ApiEnvelope<{ attachment: Api.Attachment.Item }>;
    if (envelope.code !== 0) throw new Error(envelope.message);
  }
});

task.progress(update => onProgress(update.progress));
onCancel(() => task.abort());
```

`wx.uploadFile` 使用同样的 `url/name/filePath/header` 契约及 `UploadTask.onProgressUpdate/abort`。示例中的 `apiBase`、文件内容和身份信息均由下游安全配置提供；禁止写入 AppSecret、OpenID/UnionID、患者图片、生产 Token 或生产域名。任何网络失败、取消或非零信封都重新调用 prepare，不能复用旧 Token。当前不支持分片、断点续传或离线重放。

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
| `AttachmentList` | 列表、统一预览入口、下载、解绑/删除确认 |
| `AttachmentPreview` | 图片/PDF/视频 Blob 预览、状态、下载、新窗口与资源回收 |
| `AttachmentField` | 打开选择器并绑定到 resource；无 resourceId 时临时 pending |

`AttachmentPicker` 必须显式区分选择语义：默认 `replace` 返回最终选择集合；绑定型业务使用 `append` + `lockedSelectedIds`，已关联项显示锁定状态且确认事件只返回本次新增项。解除关联由业务列表独立操作，禁止把锁定项作为新增请求重复提交。

个人资料头像复用 Picker 的单选 replace 模式和共享附件头像加载器。替换顺序为绑定新 `system.account_profile/avatar` → 更新个人资料中的 attachment ID → 解绑旧头像；更新失败须回滚新绑定。认证 store 只保存 attachment ID，全局 AI 用户消息按 ID 走 prepare/download 获取头像，缺失或加载失败时使用账户图标降级。

AI Builder 的预览消息复用 `AttachmentPicker` 与上传队列，但不伪造业务 `resource_type`：只允许当前上传者有权访问、状态 ready 且符合 #73 图片限制的附件，并将 attachment ID + revision + detail 作为 `input_image` Content Part 提交。当前不支持通用文件、音视频或 PDF 作为模型输入。

有效上传策略：`POST .../attachments/policy`（设置覆盖 + 代码默认）。附件中心：`/system/attachments`（状态用 `CodeSelect` / `CodeTag` + CodeSet `system.attachment_status`；上传者展示 `uploader_name`）。

### 统一预览

业务列表和附件中心必须复用 `AttachmentPreview`，不得在页面内分别维护 `img` / `iframe` / `video`、MIME 判断、Provider URL 或 transfer token。组件只接受 attachment ID 或 `Api.Attachment.Item`；传 ID 时先通过附件详情接口取得标准元数据。object key、Provider URL、公开对象地址和长期 token 都不是组件输入。

首期支持矩阵：

| MIME / 状态 | 预览方式 | 说明 |
|-------------|----------|------|
| ready `image/*` | 原生 `img` | Blob Object URL，保持比例自适应 |
| ready `application/pdf` | 原生 PDF iframe | 不解析、不批注；Chrome 兼容要求见下文 |
| ready `video/*` | 原生 `video` | Blob 全量下载后播放，不支持 Range、自适应码率或转码 |
| 非 ready / 其他 MIME | 不预览 | fail-closed，不调用 download prepare/transfer；仍可按权限显式下载 |

预览通过 `useAttachmentPreview()` 串联详情、download prepare 和 transfer，取消信号贯穿三段。打开新附件前、关闭和卸载时都必须中止当前请求并 `revokeObjectURL`；请求版本隔离保证旧请求完成后不能覆盖新附件。新窗口只打开当前 Blob URL，并使用 `noopener,noreferrer`。

图片预览统一支持以鼠标位置为中心的滚轮缩放，缩放范围限制为 25%–500%；放大后可按住拖动查看局部，双击恢复 100%。切换附件、关闭预览或组件卸载时必须同时复位缩放、位移与 pointer capture。该交互属于 `AttachmentPreview` 公共能力，业务页面不得另行包装图片查看器。

Chrome 原生 PDF Viewer 无法在 sandbox iframe 中运行，会显示“此页面已被 Chrome 屏蔽”；`allow-same-origin` 也不能解除该限制。因此统一组件的 PDF iframe **禁止设置 `sandbox`**，并必须保留 `referrerpolicy="no-referrer"`。这不是放宽附件来源：iframe 的 `src` 只能是组件在附件状态为 ready、MIME 精确等于 `application/pdf`、大小未超限后，经鉴权 prepare/transfer 创建的内存 Blob URL；业务 URL、Provider URL、object key 与长期 token 仍不能进入组件。若后续安全策略要求 iframe 必须 sandbox，则不得继续依赖浏览器原生 Viewer，应另行引入 PDF.js 等受控渲染方案。

Blob 预览需要先完整下载到浏览器内存，统一上限为 **50 MiB**。超过上限时不调用 preview prepare/transfer，界面明确显示文件大小和上限，仅保留显式下载，不宣称流式播放。视频 Range 与短时流式 URL 由 #256 另行建设；在该能力交付前，大视频统一降级为下载。

选择器「我的上传」为**宫格预览** + **滚动加载更多**（首屏 24 条，触底追加，软顶 200；搜索重置第 1 页）。图/视在 `thumb_status=ready` 时经 `variant=thumb` 加载；`pending` 显示 Spin。`useAttachmentThumbStatusPolling` 仅在调用方可见且仍有 pending 图/视时每 2.5 秒静默批量刷新当前用户已加载范围，按 ID 合并状态；全部完成、调用方隐藏或卸载后立即停止，不重置选择、草稿或滚动位置。Picker 的“我的上传”和 AI Composer 图片草稿统一复用该 Hook，因此粘贴上传后的异步缩略图会自动替换文件名占位。否则显示类型图标。缩略图并发受限（默认 3）并在关闭时 `revokeObjectURL`。token 不进持久存储与日志。

Demo：`/demo/attachments`（选择器主路径）。

## 安全

- transfer / download token 仅内存使用，不进 localStorage / sessionStorage / console
- multipart 上传只传平台 `filePath`，不在 JS 内存全量读取；filename、Part MIME 和表单字段不能作为正式元数据
- 不发送 `tenant_id`、完整 object key 或未注册 `resource_type`
- **禁止**拼 ObjectStore / OSS URL 或厂商图片处理参数
- 业务页面禁止绕过 `AttachmentPreview` 自建图片/PDF/视频预览分支
- 预览 Object URL 关闭/卸载时 `revokeObjectURL`

## 检查清单

- [ ] 业务只用统一 Client / 组件，不拼 Provider URL
- [ ] `resourceType` 来自常量且后端已注册 AccessPolicy
- [ ] 宫格不在 thumb 未就绪时拉原图
- [ ] 图片/PDF/视频预览复用 `AttachmentPreview`，非 ready/不支持/超限不发起预览下载
- [ ] `pnpm test:attachment-preview` 通过并由 CI 明确执行
- [ ] `pnpm typecheck` / `pnpm build` 通过
- [ ] 变更同步本文档与 Agent 规则并 `agent-rules sync`
