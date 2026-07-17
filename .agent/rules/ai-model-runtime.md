# AI 模型统一调用硬约束

- 业务只能通过 `model_alias` 调用 `aimodelruntime`，禁止绕过 Resolver 或直接调用供应商 SDK/HTTP。
- Adapter 必须注册到协议 Registry，未知协议 fail-closed；自定义 Endpoint 是不带 `/v1` 的根地址。
- Stream 必须使用有界 channel、传播取消，并保证单次开始、单调序号、Item 生命周期和唯一终态。
- Embedding 走统一 `EmbeddingRuntime.Embed` 与 `EmbeddingAdapterRegistry`，禁止旁路或用 Chat 伪造向量；详规见 `ai-embedding-runtime`。
- Provider 调用前创建 `running` Invocation；终态更新与 `ai.model_invocation.finished.v1` 必须同事务。
- Invocation/Outbox 禁止保存提示词、输出、工具参数、原文、向量、凭证、原始供应商错误和响应体。
- 遗留 `running` 只能恢复为 `unknown`，禁止自动重放非幂等模型调用。
- 协议与安全测试统一放在 `platform/tests`。详见 `docs/development/ai-model-runtime.md`。
