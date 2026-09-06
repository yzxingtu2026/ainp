# AI 多模态输入开发规范

Conversation 可永久保留多张 canonical `input_image` 引用，但单次 Provider 请求仍受 `MaxNativeImagesPerRequest` 硬上限约束。ContextAssembler 在解析媒体前构造 Run 级原生图片工作集：当前触发消息优先，其余按最近 sequence 补足；未选中的历史图片不得调用 AssetResolver。每次 Run 从全部 canonical 图片/文档另投影只含 `source_sequence + attachment_id + attachment_revision + content_kind` 的 Manifest；它不含 Object Key、URL、token、base64、Provider file ID 或原始字节，ID 只是引用而非凭证。Model Runtime 继续独立执行相同硬上限，作为最终 fail-closed 边界。

## 稳定协议

模型输入使用供应商无关的强类型 Content Part。可执行输入包括 `input_text`、`input_image`、canonical `input_file` 和工具结果；`input_audio`、`output_audio` 仍必须 fail-closed。模型输出与后续会话可使用 `output_text`、`tool_call`、`refusal`；`derived_text` 只能由服务端生成，必须携带来源且标记为不可信。

`input_image` 的外部协议只接受：

```json
{"type":"input_image","image_attachment_id":"...","detail":"auto"}
```

`input_file` 的外部协议只接受：

```json
{"type":"input_file","file_attachment_id":"...","attachment_revision":"..."}
```

`input_file` 仅允许 user Message。外部请求禁止 URL、base64、服务器路径、object key、tenant ID、正文、供应商 file ID、`derived_source` 和未知字段；canonical Message 只持久化 attachment ID + revision。

禁止 URL、服务器路径、Object Key、base64、tenant_id 和供应商字段。Content Part 使用严格字段解析，未知字段直接拒绝。

## 解析与授权

调用顺序固定为：

```text
ModelResolver 能力校验
  → AIAssetResolver
  → tenant/user/AccessPolicy/ready 校验
  → ObjectStore 有界读取
  → MIME 嗅探、格式、大小、像素和数量校验
  → Provider Adapter
```

知道 attachment ID 不代表拥有权限。已绑定附件必须通过对应资源 AccessPolicy；未绑定附件仅上传者本人可用。读取必须使用 ObjectStore，业务模块不得读取路径或按存储 Provider 分支。

平台当前限制为单图 10 MiB、最多 4 图、总计 20 MiB、最大边长 8192、最大 2500 万像素，仅允许 JPEG、PNG、GIF、WebP。Adapter 还需执行供应商限制，Anthropic 单图当前限制为 5 MiB。任何限制必须在发送 Provider 前完成。

即时文档使用独立 `DocumentResolver`：消息创建时校验 tenant/user、AccessPolicy、ready、revision、MIME 和 10 MiB 上限；只有当前触发消息中的文档在本轮装配时重新授权并经 ObjectStore 有界读取。历史文档不得每轮重复解析，内容通过 `system.attachment_content_inspect` 按需读取。解析复用 `aiknowledge.SafeParser`，仅允许 PDF、XLSX、DOCX、UTF-8 TXT、Markdown；DOCX 普通 HTTP/HTTPS 超链接只读取显示文字且不访问目标，白名单字段只读取已保存结果，未知/危险/无缓存字段拒绝；XLSX 公式同样只读已保存缓存，绝不执行或刷新。单文档最多 100 页/工作表，Action Result 最多 8 个观察、12000 字符。MIME 伪造、主动 Office 内容、损坏/空文本、低质量提取、非 UTF-8/NUL 和资源超限均在 Provider 前 fail-closed；低质量 PDF 不得注入 `derived_text`。

`system.attachment_content_inspect` 是 `Capability=system`、`Risk=low`、`SideEffect=read` 的普通渐进发现 Action。每次调用重新校验 authenticated tenant actor、Client Surface、tenant、上传者或资源 AccessPolicy、ready、revision、MIME、大小与运行时预算。文档分支复用 `DocumentResolver + SafeParser` 并按 question/location 返回有界片段；图片分支复用 `AIAssetResolver`，使用当前 Agent 的受治理 Model Alias 经统一 Model Runtime 做 Vision 观察，不支持 Vision 时返回 `capability_not_supported`。结果只包含安全文件名、MIME、revision、位置/观察、`status`（`ready` / `empty` / `low_quality` / `partial_failure`）、可选按页 `page_statuses`、`truncated`、`untrusted=true`。工具执行完成不等于文本可用；空文本或乱码不得作为普通 ready observations。附件正文只存在于当次 Tool Result/模型输入。

## Provider 映射与生命周期

OpenAI Adapter 映射到 Responses `input_image`，Anthropic Adapter 映射到 Messages `image` source。文档在 Adapter 前展开为 user 侧 `derived_text`，正文前置安全文件名、MIME 和页/工作表位置，明确标记为不可信参考；Provider 收不到 `input_file`、attachment ID/revision、object key 或下载 URL。任何 Adapter 收到未展开 `input_file` 都必须 fail-closed。

Generate、Stream 成功、失败和取消后都必须释放解析后的媒体字节。Invocation 只记录输入 modality、附件数量和总字节，不记录附件 ID、正文、对象信息或供应商原始请求。

同一 Run 的当前文档使用 `attachment ID + revision + parser version` 内存缓存复用解析结果；Run 结束即释放，Worker 恢复允许重新授权并确定性解析。Action 首期不做跨 Run 内容缓存。解析正文和 Vision 观察不得进入 canonical Message、ContextSnapshot、日志、审计、Outbox、Agent Step 或 ModelInvocation 摘要；Vision 调用仍必须生成统一 ModelInvocation 用量事实。

## 视觉派生扩展

视觉降级属于 Context Enrichment，不默认定义为子 Agent。`MediaUnderstandingService`、`derived_text` 来源和缓存键是扩展契约；首期只执行 `native_only`。`allow_derivation` 与 `require_derivation_confirmation` 必须明确失败，不能静默调用其他模型。

派生缓存键由 attachment ID、revision/hash、vision deployment、prompt version、output schema version 和 locale 组成。图片、OCR 和派生文本均是不可信内容，不能提升为 system/developer instruction，也不能绕过 Tool Runtime。大 PDF、多图批处理、OCR、视频抽帧和视觉派生必须走 Outbox → Job → 派生对象。

## 测试

测试至少覆盖严格字段/角色、跨租户和资源越权、revision/状态变化、MIME 伪造、五类文档成功解析、主动内容/损坏/空文本/资源上限、Run 内缓存、三个 Adapter wire-level 正向正文与内部标识负向断言、Stream 取消释放及敏感媒体不落账。
