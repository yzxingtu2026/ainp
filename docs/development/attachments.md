# 租户隔离附件系统

本文档约定 ObjectStore 之上的附件元数据、短时传输协议、业务绑定与访问策略。Agent 要点：`.agent/rules/attachments.md`。

## 边界

| 层级 | 职责 | 不负责 |
|------|------|--------|
| ObjectStore（#17） | 二进制 Put/Open/Range/Delete、key | 租户权限、文件名、业务归属 |
| 附件系统（本 Issue） | 元数据、prepare + transfer token、绑定、策略、删除补偿、审计 | 直接拼路径 / 厂商 SDK / 前端组件 |
| 租户设置（#13） | 未来覆盖大小/MIME/保留策略 | 第二套附件配置表 |

前端上传/预览组件见 `docs/development/frontend-attachments.md`（#19）。

## 数据模型

- `attachments`：`tenant_id` 强制非空；`provider` / `object_key` / `original_name` / `content_type` / `size` / `sha256` / `status` / `uploader_id`；`upload_jti` 仅存一次性上传令牌 ID（非 token 明文）；`expires_at` 用于 pending 临时保留。
- `attachment_links`：`(tenant_id, attachment_id, resource_type, resource_id, field)` 唯一；查询写入经 `tenant.ScopeByTenant` fail-closed。
- 状态机值：`pending` / `ready` / `deleting` / `deleted` / `failed`（CodeSet `system.attachment_status`）。禁止用对象是否存在推断业务状态。
- `original_name` 仅展示；`object_key` 由服务端 `storage.BuildKey` 生成。

迁移：`platform/migrations/0005_attachments.sql`；开发可用 AutoMigrate + CHECK。

## 上传协议

1. Action `system.attachment_upload_prepare`：校验权限与上传策略，创建 `pending` 记录，签发短时 **upload** transfer JWT。
2. 客户端 `PUT /api/v1/attachments/transfer/:id`，携带 token（`Authorization: Bearer` / `X-Attachment-Token` / `?token=`），流式写入 body；**文件内容不进 JSON Action**。
3. 服务端校验 size / SHA-256 / MIME 探测与扩展名策略，成功则 `ready` 并清空 `upload_jti`（令牌一次性）。

Local Provider 返回本服务流式 URL；未来 S3/OSS 可换预签名 URL，上层 prepare 协议不变。

## 下载协议

1. Action `system.attachment_download_prepare`：校验租户、状态、上传者/业务访问策略后签发 **download** token。
2. `GET` / `HEAD` `/api/v1/attachments/transfer/:id`：支持 Range、Content-Length、Content-Type、ETag、安全 `Content-Disposition`（拒绝 CRLF）。
3. 写流前失败返回统一 JSON 信封；开始写流后不再改 JSON。

## 业务绑定与访问策略

- 领域服务：`Bind` / `Unbind` / `ListByResource`；业务模块在自身 Action 权限与事务内调用。
- `resource_type` 必须代码注册 `attachment.AccessPolicy`；**未注册默认拒绝**，禁止仅凭同租户访问任意附件。
- pending/failed：仅上传者且在 `expires_at` 内可访问；ready：无链接时仅上传者，有链接时任一链接策略通过即可。
- Demo 注册 `demo.task` 策略（同租户已认证即可），便于联调；真实业务须注册自己的策略。

## 策略与设置

- 代码默认：`attachment.DefaultUploadPolicy()`（大小、MIME、扩展名、临时保留、transfer TTL）。
- `PolicyProvider` 稳定接口；当前 `StaticPolicyProvider`；#13 租户覆盖通过 Provider 读取，不建第二套设置表。

## 删除与补偿

- DB 与 ObjectStore 非同一事务：先标 `deleting`，再删对象；对象失败保持 `deleting`，对外不再暴露下载。
- 有 `attachment_links` 时禁止直接删除，须先 Unbind。
- `CleanupExpired`：清理过期 pending/failed，并重试 `deleting` 对象删除。

## Action / 审计 / 传输端点

| Action | 说明 | audit |
|--------|------|-------|
| `system.attachment_upload_prepare` | 准备上传 | true |
| `system.attachment_download_prepare` | 准备下载 | true |
| `system.attachment_bind` / `unbind` | 绑定/解绑 | true |
| `system.attachment_list_by_resource` | 按资源列表 | true |
| `system.attachment_delete` | 删除 | true |

传输端点是专用 HTTP adapter，不接受业务参数；权限决策只在 Action / 领域服务完成。审计记录 attachment/resource/size/type/result，**禁止**记录文件内容与 transfer token 明文。

## 检查清单

- [ ] 新业务注册 AccessPolicy，不硬编码「同租户即可」
- [ ] 上传/下载走 prepare + transfer，不把字节塞进 JSON
- [ ] 租户隔离与跨租户 ID 测试通过
- [ ] token 过期/重放、未注册 resource_type、Range/HEAD 覆盖
- [ ] 变更同步本文档与 Agent 规则并 `agent-rules sync`
