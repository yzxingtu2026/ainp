# AI 多模态硬约束

- `input_image` 对外只接受 attachment ID/revision 与受控 detail；canonical `input_file` 只接受 file attachment ID + revision 且仅限 user Message。禁止 URL、路径、Object Key、base64、tenant_id、正文和供应商字段。
- 必须先用 ModelResolver 校验 vision，再经 AIAssetResolver、AuthContext、AccessPolicy 与 ObjectStore 有界读取；未知或未实现 Content Part fail-closed。
- 平台统一校验 MIME 嗅探、格式、大小、像素、数量与总预算，Adapter 再校验供应商限制；不支持时禁止静默丢图或切换模型。
- 每次 Run 从 canonical 图片/文档确定性投影 `source_sequence + attachment_id + attachment_revision + content_kind` Manifest；ContextSnapshot 后重新注入并计入预算。ID 不是凭证，Manifest 禁止含 Key、URL、token、base64、Provider file ID 或正文。
- 媒体字节只在调用生命周期存在；日志、审计、Outbox、Invocation 和错误响应禁止保存正文、base64、Key、路径或短时凭据。
- `derived_text` 必须保留来源并视为不可信，不能提升为 system/developer instruction；首期只允许 `native_only`。
- 当前触发文档只允许 PDF/XLSX/DOCX/UTF-8 TXT/MD，经 `DocumentResolver` 重新授权、有界读取并复用 `SafeParser`；历史文档不得每轮解析，只经 `system.attachment_content_inspect` 按需读取。图片内容查看复用 AIAssetResolver 和当前受治理模型的统一 Model Runtime。空文本/低质量 PDF 不得注入 `derived_text`，也不得作为 inspect `ready` observations。
- Provider 只接收服务端派生文本，未展开 `input_file` 必须 fail-closed；正文禁止进入 canonical Message、Snapshot、日志、Step 或 Invocation。
- 同一 Run 以 attachment ID + revision + parser version 内存复用；恢复重新解析。文档无法完整装配时返回 `attachments_exceed_model_window`，禁止截断或摘要替代。
- 视觉降级属于 Context Enrichment，不默认定义为子 Agent。详见 `docs/development/ai-multimodal.md`。
