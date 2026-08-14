# AI 模型统一调用硬约束

- 业务只能通过 `model_alias` 调用 `aimodelruntime`，禁止绕过 Resolver 或直接调用供应商 SDK/HTTP。
- 结构化响应只声明 `StructuredOutput`；response-only function 仅由 Adapter 私有映射，禁止业务写入 `Tools/ToolChoice`，且不得进入 Tool Runtime 或产生执行事实。
- response-only function 固定覆盖关闭模型思考模式，由 Runtime 私有约束并由 Adapter 映射，禁止业务或模型名称分支。
- 传输协议只选择 Adapter；结构化输出策略只取 Deployment 有序声明与 Adapter 实现能力的交集，禁止按协议、供应商、Alias 或模型名称猜测，空交集 fail-closed。
- Adapter 必须注册到协议 Registry，未知协议 fail-closed；自定义 Endpoint 是不带 `/v1` 的根地址。
- Stream 必须使用有界事件 channel 和独立 buffered terminal result，传播取消，并保证单次开始、单调序号、Item 生命周期、唯一终态及 Invocation 持久化先于 consumer 收口。
- terminal 已识别时必须胜过随后发生的 close/cancel/deadline；无 terminal 且无可识别上游错误的 EOF 才是 `stream_protocol_error`。`timeout`、`cancelled` 与 Provider 稳定分类禁止相互覆盖。
- Embedding 走统一 `EmbeddingRuntime.Embed` 与 `EmbeddingAdapterRegistry`，禁止旁路或用 Chat 伪造向量；详规见 `ai-embedding-runtime`。
- Provider 调用前创建 `running` Invocation；终态更新与 `ai.model_invocation.finished.v1` 必须同事务。
- Invocation/Outbox 禁止保存提示词、输出、工具参数、原文、向量、凭证、原始供应商错误和响应体。
- 遗留 `running` 只能恢复为 `unknown`，禁止自动重放非幂等模型调用。
- 协议与安全测试统一放在 `platform/tests`。详见 `docs/development/ai-model-runtime.md`。
