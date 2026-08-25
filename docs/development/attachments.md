# 租户隔离附件系统

本文档约定 ObjectStore 之上的附件元数据、短时传输协议、业务绑定与访问策略。Agent 要点：`.agent/rules/attachments.md`。

## 边界

| 层级 | 职责 | 不负责 |
|------|------|--------|
| ObjectStore（#17） | 二进制 Put/Open/Range/Delete、key | 租户权限、文件名、业务归属 |
| 附件系统（本 Issue） | 元数据、prepare + transfer token、绑定、策略、删除补偿、审计、**派生对象（thumb）** | 直接拼路径 / 厂商 SDK / 前端组件 |
| 租户设置（#13） | 覆盖大小、扩展名/MIME 白名单与保留策略 | 第二套附件配置表 |

前端上传/预览组件见 `docs/development/frontend-attachments.md`（#19 / #36）。

## 数据模型

- `attachments`：`tenant_id` 强制非空；`provider` / `object_key` / `original_name` / `content_type` / `size` / `sha256` / `status` / `uploader_id`；`upload_jti` 仅存一次性上传令牌 ID 或服务端 claimed 标记（非 token 明文）；`expires_at` 用于 pending/claimed 临时保留。
- **派生（thumb）列**（#36，Phase 1 单变体）：`thumb_status` / `thumb_object_key` / `thumb_content_type` / `thumb_size` / `thumb_error` / `thumb_error_category` / nullable `thumb_retryable` / `thumb_updated_at`。`thumb_retryable=NULL` 表示未分类的历史状态，不自动补偿重放。未选独立 `attachment_derivatives` 表（多变体时可再拆）。
- `attachment_links`：`(tenant_id, attachment_id, resource_type, resource_id, field)` 唯一；查询写入经 TenantDB Repository 与 GORM tenant Callback fail-closed。
- 原图状态机：`pending` / `ready` / `deleting` / `deleted` / `failed`（CodeSet `system.attachment_status`）。
- **派生状态机（独立 CodeSet，禁止复用 AttachmentStatus）**：`pending` / `ready` / `failed` / `skipped`（CodeSet `system.attachment_thumb_status`）。原图 `ready` **不依赖**派生成功。
- `original_name` 仅展示；`object_key` / `thumb_object_key` 由服务端 `storage.BuildKey` 生成（独立 object_id，租户隔离）。

迁移：`platform/migrations/0005_attachments.sql`、`0008_attachment_thumbs.sql`、`0053_attachment_thumb_failures.sql`；开发可用 AutoMigrate + CHECK。

## 上传协议

1. Action `system.attachment_upload_prepare`：校验权限与上传策略，创建 `pending` 记录，签发短时 **upload** transfer JWT。请求可显式传 `transport=raw|multipart`，缺省为 `raw`，禁止按 User-Agent、Origin 或平台猜测。
2. prepare 返回服务端决定的 `upload_url`、`method`、`transport`、`token`、`expires_at`；multipart 额外返回固定 `field_name=file`，客户端不得硬猜字段名。
3. Transfer JWT 的 `transport` claim 绑定传输入口；新 raw Token 只能 PUT，新 multipart Token 只能 POST。兼容窗口内缺少 claim 的旧 Token 仅解释为 raw，不能获得 multipart 权限。
4. Raw 与 multipart 都在读取完整文件前通过数据库条件更新将同一 `attachment_id + upload_jti` 原子 claim；多实例下最多一个请求进入 ObjectStore Put。失败即清空 JTI 并进入 `failed`，Token 不可重放；进程异常留下的 claimed pending 仍由 `expires_at` + `CleanupExpired` 确定性清理。
5. 两种入口都把 `io.Reader` 交给同一 `AttachmentService.CompleteUpload`，统一校验 size / SHA-256 / MIME 探测与策略。成功则 `ready` 并清空 `upload_jti`；ready 更新和 `attachment.ready.v1` Outbox 保持原事务/幂等语义。
6. **ready 后派生**：同事务写入 `thumb_status`（图/视 → `pending`；其他 → `skipped`）并 Append Outbox 事件 `attachment.ready.v1`；Handler 入队 Job `attachment.generate_thumb`（`file` 队列）。

| transport | HTTP | Body | 适用客户端 |
|-----------|------|------|------------|
| `raw`（默认） | `PUT /api/v1/attachments/transfer/:id` | 原始文件字节 | Web `fetch` / `axios`；现有 Vue 管理端 |
| `multipart` | `POST /api/v1/attachments/transfer/:id` | `multipart/form-data`，唯一 `file` Part | `wx.uploadFile` / `Taro.uploadFile` |

Token 优先放 `Authorization: Bearer` 或 `X-Attachment-Token` Header；兼容 raw 下载/上传仍支持 query token，但小程序上传禁止把 Token 放入文件名或表单字段。失败重试必须重新 prepare，禁止复用一次性 Token。

`Authorization: Bearer` 会先经过共享 HTTP 中间件，但 Web Session Auth 不得仅因 transfer token 携带 `principal_kind` / `surface` 就把它误判为 Session Token。分流后的请求仍是未建立 Web Session 的请求，必须由附件 transfer Handler 按 `ainp:attachment-transfer` audience 完整校验签名、过期时间、operation、transport、tenant、Principal Ref、attachment、variant、size 与 JTI。`X-Attachment-Token` 和 query 仅是同一协议的传递方式，不能作为规避服务端 Token 分类错误的替代修复。

### Multipart Adapter 安全边界

- 使用 `multipart.Reader` 流式解析，不调用 `ParseMultipartForm`、无界 `io.ReadAll`，也不把完整文件落到 `/tmp` 后再交给 ObjectStore。
- 只接受一个名为 `file` 且带 filename 参数的文件 Part；缺失、重复、未知文件字段、普通字段、嵌套 multipart 与 `Content-Transfer-Encoding` 全部拒绝，Part 字节不做 Header 驱动的透明解码。
- 主 `Content-Type` 必须是带合法 1-70 字节 boundary 的 `multipart/form-data`。整个 Body 上限为 Token expected size + 64 KiB 协议开销；multipart 元数据最多 32 KiB；单 Part Header 最多 16 KiB / 32 个 Header value。
- expected size 是文件 Part 的精确上限；截断、超长、读取错误、Context 取消或 ObjectStore 失败都会使 claimed 上传进入 `failed`，且对象不可访问。
- Part filename、Part `Content-Type` 和表单值均不可信，不覆盖 prepare 保存的展示名、MIME 提示、tenant、uploader、expected size 或 object key；最终 MIME 仍由服务端内容探测和策略决定。
- 日志只记录 attachment ID、transport/结果类别、预期大小、request ID 和耗时等安全字段；禁止记录 Token、Header 全文、客户端文件名、表单值、内容、object key 或底层错误全文。

Local Provider 返回本服务流式 URL；未来 S3/OSS 可换预签名 URL，上层 prepare 协议不变。

反向代理/Ingress 的请求体上限和上传超时必须不低于 AINP 当前有效上传策略加 multipart 协议开销，否则请求会在进入应用前失败；代理配置不能替代应用层 expected size、Token、MIME 和租户策略校验。本协议不支持分片、断点续传、S3/OSS multipart upload 或客户端直传；未来能力仍必须通过 prepare 返回独立受控 contract。

## 下载协议

1. Action `system.attachment_download_prepare`：校验租户、状态、上传者/业务访问策略后签发 **download** token。请求体可带 `variant`：`original`（默认）/ `thumb` / `preview`（本阶段 `preview` 仍 `attachment.variant_unavailable`）。
2. `thumb` 仅当 `thumb_status=ready` 且存在 `thumb_object_key` 时签发；否则返回业务码 `attachment.variant_unavailable`（**禁止** silent 回落原图）。
3. 资源 AccessPolicy 只在 prepare 使用真实 AuthContext 执行。prepare 成功后，短时 download token 在 TTL 内即为该主体对指定 `tenant + attachment_id + variant` 的 transfer 授权；token 不携带、复制或信任 Permissions 快照，transfer 也不重跑资源策略。资源随后解绑不会撤销已签发 token；需要即时撤销时必须让附件进入不可传输状态，或等待 token 到期。
4. Transfer JWT 携带签名的 issuer/audience、过期时间、op、tenant、principal、attachment ID、`var`（variant）和签发时 size；`GET` / `HEAD` `/api/v1/attachments/transfer/:id` 必须逐项 fail-closed 校验，并重新读取附件确认当前仍为 `ready`、variant 对象仍存在且 size 一致。`deleting` / `deleted` / pending / failed、跨租户、对象或 variant 不匹配均不得返回字节。
5. 普通 GET、HEAD 与 Range 使用同一 token 授权语义。Range 只打开一次目标范围，不得为了读取元数据先完整打开对象或重复执行资源权限决策。响应支持 Content-Length、Content-Type、ETag、安全 `Content-Disposition`。
6. **禁止**前端拼 Provider URL / OSS 图片处理参数；**禁止**长期公开缩略图链接。OSS 处理参数仅允许作为 Adapter 内部优化（非产品协议）。

## 派生生成管线（#36）

| 项 | 约定 |
|----|------|
| 标准变体 | `thumb`：约 200² JPEG cover |
| 图片 | 纯 Go 解码缩放（JPEG/PNG/GIF 首帧/WebP）；限制解码像素与读入体积 |
| 视频 | 封面帧经主机 `ffmpeg`；不可用则 `skipped`（`thumb_error=ffmpeg unavailable`），不回滚原图 |
| Job | `attachment.generate_thumb`；可靠路径：Outbox → Enqueuer → Asynq Worker（见 `docs/development/background-jobs.md`） |
| 幂等 | 已 `thumb_status=ready` 直接成功；重试可替换旧 thumb key |
| 失败 | 像素/源文件上限、无效尺寸、损坏/不支持编码为 permanent；ObjectStore/数据库为 retryable；取消/超时为 cancelled。失败只保存稳定类别和安全摘要。 |
| 补偿 | `jobs.RequeuePendingThumbs`：只扫描 `ready + pending` 或 `ready + failed + thumb_retryable=true`；permanent 与历史 NULL failed 不自动重放。 |
| 删除 | Delete / CleanupExpired 同步删 `thumb_object_key` 与原图 |

部署：生产 Worker 主机若需视频封面，须安装 `ffmpeg` 并在 PATH 中；CI 可用 fixture 测图片，视频依赖缺失时期望 `skipped`。

缩略图默认保留 4000 万像素与 64 MiB 源文件读取上限。结构化失败日志可记录 `error_category`、width/height、pixel_count/pixel_limit、source_size 与 attempt；禁止记录 object key、原文件内容、URL 或底层错误全文。提高限制或引入超大图处理器必须另行评估 Worker 内存预算。

## 后处理扩展约定（基建，非本阶段实现）

转码 / OCR / 向量化 / 特征提取等**不在本阶段落地**，但新增管道须遵守同一基建边界，避免另起一套：

| 层 | 约定 |
|----|------|
| 触发 | 原图 `ready` 同事务 Outbox（可复用 `attachment.ready.v1` 多 Handler，或新事件类型）；**禁止**业务事务直连 Redis/Asynq |
| 执行 | 新 Job Type（`modules/*/jobs/`，一 Type 一文件）；按耗时选 `file` / `ai` 等队列；Handler 幂等、可重试，失败不回滚原图 `ready` |
| 产物 | ObjectStore 独立 `object_key`；对外仍是 `attachment_id + variant + 短时 token`；**禁止**把厂商处理 URL 做成产品协议 |
| 状态 | 独立状态机/CodeSet（或将来 `attachment_derivatives`：`attachment_id + kind/variant`）；**禁止**复用 `AttachmentStatus` 冒充派生状态 |
| 删除 | 删除原图时级联清理该附件全部派生对象（含补偿路径） |
| 多产物 | Phase 1 仅 `thumb_*` 列；种类 ≥2 或需并行管道时再拆独立 derivatives 表，而不是继续往 `attachments` 堆列 |

本阶段只保证：**Outbox → Job → 派生对象 → variant 下载** 这条链可复用；具体转码/OCR/向量化另开 Issue。

## 业务绑定与访问策略

- HTTP 入口复用 `AttachmentService.BindResource` / `UnbindResource`；业务模块只依赖 `attachment.ResourceLinker` + 强类型 `ResourceLinkRequest`。
- 业务创建/更新/删除资源时必须把当前 GORM 事务传给 `ResourceLinker`，使资源事实与 `attachment_links` 原子提交；禁止业务直写链接表。
- `ResourceLinker` 会重新校验 AuthContext、附件 tenant/ready 状态与 AccessPolicy；策略可通过受控事务上下文读取尚未提交的资源行。
- `resource_type` 必须代码注册 `attachment.AccessPolicy`；**未注册默认拒绝**，禁止仅凭同租户访问任意附件。
- 下载 prepare 仅允许 `ready`：无链接时仅上传者，有链接时任一链接策略通过即可。pending/failed 只保留上传与清理流程所需的上传者访问语义，不得签发 download token。变体下载与原图在 prepare 共用同一套访问判定。
- Demo 注册 `demo.task` 策略（同租户已认证即可），便于联调；真实业务须注册自己的策略。
- 当前账号头像注册 `system.account_profile` / `avatar` 策略：`resource_id` 必须等于 AuthContext 当前主体 ID，绑定附件必须由当前主体上传且为 ready 图片。`tenant_accounts.avatar_attachment_id` 只保存附件 ID；头像读取仍逐次执行 prepare/download 鉴权，不保存对象地址或短时 token。

## 策略与设置

- 代码默认：`attachment.DefaultUploadPolicy()`（含常见图片、视频、PDF、纯文本、Markdown 与 Office Open XML 文档）。
- 租户覆盖：设置组 `attachment.upload_policy`；`PolicyProvider`（`NewSettingUploadPolicyProvider`）读取有效值。租户管理员可在扩展名与 MIME 多选中添加自定义值，服务端按字段 Schema 校验并统一规范化。
- 前端预校验：`system.attachment_policy_get`，禁止硬编码为唯一来源。

## 删除与补偿

- DB 与 ObjectStore 非同一事务：先标 `deleting`，再删 **thumb + 原图**；对象失败保持 `deleting`，对外不再暴露下载。
- 有 `attachment_links` 时禁止直接删除，须先 Unbind。
- `CleanupExpired`：清理过期 pending/failed，并重试 `deleting` 对象删除（含 thumb）。

## Action / 审计 / 传输端点

| Action | 说明 | audit |
|--------|------|-------|
| `system.attachment_upload_prepare` | 准备上传；返回传输凭据，因此不暴露为 AI Tool | true |
| `system.attachment_download_prepare` | 准备下载（含 variant）；返回传输凭据，因此不暴露为 AI Tool | true |
| `system.attachment_bind` / `unbind` | 绑定/解绑 | true |
| `system.attachment_list_by_resource` | 按资源列表 | true |
| `system.attachment_list` | 分页列表（mine/all）；补充 `uploader_name`、`thumb_status` | false |
| `system.attachment_mine_list` | authenticated 本人附件列表（Picker / 对话自助） | false |
| `system.attachment_get` | 详情与 links；补充 `uploader_name`、`thumb_status` | false |
| `system.attachment_content_inspect` | AI 按需查看文档片段或图片观察；每次重验授权/revision，结果为有界不可信文本 | false |
| `system.attachment_policy_get` | 有效上传策略 | false |
| `system.attachment_delete` | 删除（级联派生对象） | true |

传输端点是专用 HTTP adapter，不接受业务参数；公开 `platform/host` 和官方应用都由 system 模块的同一 `OnHTTP` 注册自动获得 raw PUT、multipart POST 与下载路由，下游禁止复制 Gin Handler。权限决策只在 Action / 领域服务完成。审计记录 attachment/resource/size/type/result，**禁止**记录文件内容与 transfer token 明文。

AI Tool Result 使用独立安全投影：上传/下载 prepare 因返回 transfer token/URL 而不进入 Tool Catalog；`system.attachment_get` 不返回 Provider/Object Key，`system.attachment_content_inspect` 不返回原始字节、base64、Object Key、下载 token/URL 或 Provider 管理字段。`attachment_id` 只用于定位，不能授予读取权限；内容查看必须在读取 ObjectStore 或调用 Vision 前重新执行 tenant、actor、Client Surface、AccessPolicy、ready 与 revision 校验。

独立下游在非 HTTP 业务流程中读取已绑定附件时，只使用 `framework/platformresource.AttachmentReader`，由 `app.App.PlatformResources` 注入。该 Port 必须重新执行当前 tenant/principal、AccessPolicy、绑定、ready、variant、对象存在与大小校验，只返回最小元数据和受控 reader；禁止返回 object key、provider、transfer token 或内部 Model。

## 检查清单

- [ ] 新业务注册 AccessPolicy，不硬编码「同租户即可」
- [ ] 上传/下载走 prepare + transfer，不把字节塞进 JSON；multipart 只接受服务端返回的 `field_name`；宫格只用 `variant=thumb`
- [ ] 上传失败/取消重新 prepare；禁止跨 raw/multipart 使用或重放 Token
- [ ] Picker / 对话只调用 `attachment_mine_list`；租户附件中心继续走权限型 `attachment_list`
- [ ] 租户隔离与跨租户 ID 测试通过
- [ ] token 过期/重放、未注册 resource_type、Range/HEAD、variant 未就绪覆盖
- [ ] 变更同步本文档与 Agent 规则并 `agent-rules sync`
