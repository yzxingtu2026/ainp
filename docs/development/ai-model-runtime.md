# AI 模型统一调用协议

## 调用边界

- 业务只提交 `model_alias`，不得直接指定 Provider、Deployment、远端模型 ID、Endpoint 或凭证。
- `ModelResolver` 负责租户隔离、启用状态和能力校验；`AdapterRegistry` / `EmbeddingAdapterRegistry` 按协议 fail-closed 分派。
- 当前生成协议支持 OpenAI Responses、Anthropic Messages 与 OpenAI-Compatible Chat Completions。
- Embedding 协议支持 OpenAI 与 OpenAI-Compatible `/v1/embeddings`；详见 `ai-embedding-runtime.md`。
- 自定义 Endpoint 配置为服务根地址，不带 `/v1`；Adapter 统一追加资源路径。官方 Endpoint 可留空。
- Chat 调用前必须由 Resolver 使用公共预算函数验证 Deployment `context_window` 与 `max_output_tokens`；非法历史数据以 `model.context_budget_invalid` fail-closed，不得调用 Provider，也不得回退默认输出额度。

## Generate 与 Stream

统一输入由消息、内容 Part、工具、结构化输出和超时组成。流式输出必须遵循：一次 `response.started`、单调递增序号、Item 先开始再增量、一次终态。未知供应商事件忽略，协议顺序异常立即失败。

HTTP Action 的流式响应使用 SSE。生产者使用有界 channel，使慢客户端形成反压；客户端断开必须向下传播取消。

### 流终态契约

Model Runtime 返回运行时自有的 `ResultStream`：有界 `Events` 只承载已验证的增量事件，独立的 buffered `Terminal` 在 Invocation 完成持久化后恰好报告一次最终 error 语义，`InvocationID` 随 stream 直接交给 consumer。consumer 不得仅根据事件 channel close 推断错误。

终态分类固定为：请求 deadline 为 `timeout`；用户或可信上游主动取消为 `cancelled`；Adapter/Runtime 的稳定 Provider 分类原样保留；只有事件顺序违法，或没有 terminal 且没有可识别上游错误的 EOF，才是 `stream_protocol_error`。已识别的 terminal event 胜过随后同时发生的 deadline、cancel 或 provider body close error，禁止竞态二次覆盖。

生产者必须先用不继承原取消信号、但带 5 秒上限的 context 完成 Invocation 终态，再发布 `Terminal` 并关闭 channel。退出路径必须取消 provider context、关闭 provider response body，且 consumer 提前退出时不得造成 goroutine 或 channel 阻塞。账本和下游只保留安全分类，不透传 provider body、凭证、完整参数、堆栈或原始错误。

文本 delta 和 Tool Arguments delta 都是追加式部分事实。失败前的文本可用于流式展示；未收到完整 tool call terminal 的 arguments 只能作为部分响应诊断，绝不能进入 Tool Runtime 执行。

### 拒绝与终态映射

OpenAI、Anthropic 与 OpenAI-Compatible Adapter 必须把供应商安全拒绝统一映射为 `PartRefusal` / `FinishContentFilter` 或稳定 `content_filtered` 分类，不得把拒绝误归为普通 `model_failed`。有安全拒绝文本时只允许原样交给 Agent Runtime 持久化；无文本时由 Runtime 的本地化模板兜底。Adapter、Model Runtime 和 Agent Runtime 均不得调用其他模型改写拒绝，也不得把 Provider 原始响应体、安全规则细节或请求正文放入 Invocation、日志、Event、Outbox 或 API。

Agent Runtime 的安全终态模型调用属于独立 `finalization=safe_failure` Invocation：输入只能是 `FailureEnvelope v1`，必须 `EnableTools=false`、无 Tool Scope/Tools 且 `tool_choice=none`。Provider/Model Runtime 本身已失败、当前授权失效、用户取消或安全拒绝时禁止发起该调用。

## 结构化输出

业务模块只能用 `StructuredOutput` 声明稳定名称、JSON Schema 和 strict 期望，不得直接构造供应商参数，也不得把输出约束放进 `GenerateRequest.Tools`。后者始终是可执行 Agent Tool，必须经过 Tool Registry、Scope、Handle、权限重校验和确认协议。

传输协议只用于从 `AdapterRegistry` 选择请求/响应映射，不代表 Deployment 具备任何具体结构化输出能力。每个 Adapter 通过 `StructuredOutputStrategyProvider` 声明已实现策略；每个 Deployment 在 `capabilities.structured_output_strategies` 中按可靠性优先级显式声明实际支持策略。Runtime 只从两者交集中按 Deployment 顺序选择，空交集 fail-closed，禁止按协议、供应商、Alias 或远端模型名称猜测和默认回退。

当前标准策略为 `openai_json_schema`、`anthropic_output_config`、`constrained_decoding`、`response_only_function`、`json_mode` 和 `text_json`。OpenAI Responses Adapter 实现 JSON Schema 与 response-only function；Anthropic Messages Adapter 实现 `output_config` 与 response-only function；OpenAI-Compatible Adapter 实现 JSON Schema、response-only function、JSON Mode 与文本 JSON。Adapter 新增实现时只扩展自己的策略声明和私有映射，不修改业务调用方。

Response-only function 不注册到 Tool Registry、不创建 Scope/Handle/Confirmation、不执行函数，也不产生 Tool Step 或工具调用事实；Adapter 只校验函数名并将 arguments 归一为结构化文本。Invocation 层统一拒绝截断、拒绝、非文本 Item 和非法 JSON，业务层继续使用同一 DTO/Schema 执行未知字段与领域语义校验。

OpenAI Responses 的 response-only function 固定只投影一个私有函数，并使用字符串 `tool_choice=required`；不得使用供应商支持不一致的命名函数对象。Adapter 收到结果后仍须校验返回函数名与契约名称一致。

选择 `response_only_function` 时，Runtime 必须在私有 `AdapterRequest` 中强制 `DisableReasoning=true`，防止思考模式拒绝强制工具选择或输出非函数内容。OpenAI Responses 与 OpenAI-Compatible Adapter 映射为 `enable_thinking=false`；Anthropic Messages 的扩展思考为请求级 opt-in，因此该策略不得注入 thinking 配置。业务层和模型名称不得参与该约束。

调用账本仅记录最终 `structured_output_strategy` 名称。禁止记录 Schema、Prompt、函数 arguments 或完整响应。格式错误使用 `response_mismatch`，能力不足使用 `capability_not_supported`；调用方只可在总 Job 次数、Token 和超时预算内重试。旧 Deployment 只有 `structured_output` feature、没有策略列表时视为能力元数据不完整，必须由管理员确认后补齐，不自动迁移猜测。

当前结构化输出只支持非流式 `Generate`；流式响应无法在发送增量前完成整份 Schema 校验，因此 `StructuredOutput + Stream` fail-closed。

## Embedding

批量文本向量化走 `EmbeddingRuntime.Embed`，`mode=embedding`，复用同一 Invocation 账本与 `ai.model_invocation.finished.v1`。禁止在 Tool / Knowledge / Memory 中旁路直连 Embedding 供应商。完整约束见 `docs/development/ai-embedding-runtime.md`。

## 调用账本

`ai_model_invocations` 在访问供应商前写入 `running`，终态与 `ai.model_invocation.finished.v1` Outbox 事件在同一事务完成。启动时将超时遗留的 `running` 标记为 `unknown`，绝不自动重放供应商调用。

账本和 Outbox 仅允许保存路由标识、状态、时间、用量、finish reason、受控错误分类和关联 ID。Embedding 可额外记录 purpose、item count、输入字节、dimensions 与子批数。禁止保存提示词、模型输出、工具参数、原文、向量、凭证、供应商原始错误或响应体。

本地协议排障可临时设置 `AINP_AI_PROVIDER_DEBUG_RAW=true`，并将 `logging.level` 设为 `debug`。此时非流式 HTTP Adapter 只向当前进程控制台输出供应商原始成功/错误响应体，单条最多 64 KiB并标记截断；不输出请求体或凭证，不写 Invocation、Job、Outbox、审计或数据库。该开关禁止用于生产，排障结束必须立即关闭并清理包含业务正文的终端日志。

## 测试要求

Adapter 变更必须在 `platform/tests` 增加请求/响应 fixture、结构化输出策略映射、流事件顺序、取消/超时与敏感字段测试。流测试至少覆盖首 token 前、文本 delta 后、Tool Arguments 中途、Provider 分类错误、主动取消、缺 terminal EOF、非法序列、terminal/deadline 竞态、consumer 提前退出和 provider body 关闭。供应商协议变化应先更新 fixture，再调整映射。
