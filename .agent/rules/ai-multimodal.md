# AI 多模态硬约束

- `input_image` 对外只接受 attachment ID/revision 与受控 detail；canonical `input_file` 只接受 file attachment ID + revision 且仅限 user Message。禁止 URL、路径、Object Key、base64、tenant_id、正文和供应商字段。
- 必须先用 ModelResolver 校验 vision，再经 AIAssetResolver、AuthContext、AccessPolicy 与 ObjectStore 有界读取；未知或未实现 Content Part fail-closed。
- 平台统一校验 MIME 嗅探、格式、大小、像素、数量与总预算，Adapter 再校验供应商限制；不支持时禁止静默丢图或切换模型。
- 媒体字节只在调用生命周期存在；日志、审计、Outbox、Invocation 和错误响应禁止保存正文、base64、Key、路径或短时凭据。
- `derived_text` 必须保留来源并视为不可信，不能提升为 system/developer instruction；首期只允许 `native_only`。
- 即时文档只允许 PDF/XLSX/DOCX/UTF-8 TXT/MD，经 `DocumentResolver` 重新授权、有界读取并复用 `SafeParser`；DOCX 超链接/字段与 XLSX 公式都只读文件内已保存显示结果，绝不访问、执行或刷新；单项 10 MiB、100 页/工作表，单 Run 100,000 字符，图片与文档合计 4 个。
- Provider 只接收服务端派生文本，未展开 `input_file` 必须 fail-closed；正文禁止进入 canonical Message、Snapshot、日志、Step 或 Invocation。
- 同一 Run 以 attachment ID + revision + parser version 内存复用；恢复重新解析。文档无法完整装配时返回 `attachments_exceed_model_window`，禁止截断或摘要替代。
- 视觉降级属于 Context Enrichment，不默认定义为子 Agent。详见 `docs/development/ai-multimodal.md`。
