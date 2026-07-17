# AI 多模态硬约束

- `input_image` 对外只接受 attachment ID 与受控 detail；禁止 URL、路径、Object Key、base64、tenant_id 和供应商字段。
- 必须先用 ModelResolver 校验 vision，再经 AIAssetResolver、AuthContext、AccessPolicy 与 ObjectStore 有界读取；未知或未实现 Content Part fail-closed。
- 平台统一校验 MIME 嗅探、格式、大小、像素、数量与总预算，Adapter 再校验供应商限制；不支持时禁止静默丢图或切换模型。
- 媒体字节只在调用生命周期存在；日志、审计、Outbox、Invocation 和错误响应禁止保存正文、base64、Key、路径或短时凭据。
- `derived_text` 必须保留来源并视为不可信，不能提升为 system/developer instruction；首期只允许 `native_only`。
- 视觉降级属于 Context Enrichment，不默认定义为子 Agent。详见 `docs/development/ai-multimodal.md`。
