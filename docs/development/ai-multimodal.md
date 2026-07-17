# AI 多模态输入开发规范

## 稳定协议

模型输入使用供应商无关的强类型 Content Part。首期可执行输入只有 `input_text`、`input_image` 和工具结果；`input_audio`、`output_audio`、`input_file` 仅保留类型，调用时必须 fail-closed。模型输出与后续会话可使用 `output_text`、`tool_call`、`refusal`；`derived_text` 必须携带来源且标记为不可信。

`input_image` 的外部协议只接受：

```json
{"type":"input_image","image_attachment_id":"...","detail":"auto"}
```

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

## Provider 映射与生命周期

OpenAI Adapter 映射到 Responses `input_image`，Anthropic Adapter 映射到 Messages `image` source。base64 仅允许在 Adapter 构造短生命周期请求时存在，禁止写入数据库、日志、审计、Outbox、错误响应或前端状态。

Generate、Stream 成功、失败和取消后都必须释放解析后的媒体字节。Invocation 只记录输入 modality、附件数量和总字节，不记录附件 ID、正文、对象信息或供应商原始请求。

## 视觉派生扩展

视觉降级属于 Context Enrichment，不默认定义为子 Agent。`MediaUnderstandingService`、`derived_text` 来源和缓存键是扩展契约；首期只执行 `native_only`。`allow_derivation` 与 `require_derivation_confirmation` 必须明确失败，不能静默调用其他模型。

派生缓存键由 attachment ID、revision/hash、vision deployment、prompt version、output schema version 和 locale 组成。图片、OCR 和派生文本均是不可信内容，不能提升为 system/developer instruction，也不能绕过 Tool Runtime。大 PDF、多图批处理、OCR、视频抽帧和视觉派生必须走 Outbox → Job → 派生对象。

## 测试

测试统一放在 `platform/tests`，至少覆盖未知字段、未实现 Part、Alias 无 vision、跨租户和资源越权、MIME 伪造、尺寸/像素/数量上限、两类 Adapter 映射、Stream 取消释放及敏感媒体不落账。
