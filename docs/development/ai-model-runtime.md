# AI 模型统一调用协议

## 调用边界

- 业务只提交 `model_alias`，不得直接指定 Provider、Deployment、远端模型 ID、Endpoint 或凭证。
- `ModelResolver` 负责租户隔离、启用状态和能力校验；`AdapterRegistry` / `EmbeddingAdapterRegistry` 按协议 fail-closed 分派。
- 当前生成协议支持 OpenAI Responses、Anthropic Messages 与 OpenAI-Compatible Chat Completions。
- Embedding 协议支持 OpenAI 与 OpenAI-Compatible `/v1/embeddings`；详见 `ai-embedding-runtime.md`。
- 自定义 Endpoint 配置为服务根地址，不带 `/v1`；Adapter 统一追加资源路径。官方 Endpoint 可留空。

## Generate 与 Stream

统一输入由消息、内容 Part、工具、结构化输出和超时组成。流式输出必须遵循：一次 `response.started`、单调递增序号、Item 先开始再增量、一次终态。未知供应商事件忽略，协议顺序异常立即失败。

HTTP Action 的流式响应使用 SSE。生产者使用有界 channel，使慢客户端形成反压；客户端断开必须向下传播取消。

## 结构化输出

业务模块只能用 `StructuredOutput` 声明稳定名称、JSON Schema 和 strict 期望，不得直接构造供应商参数，也不得把输出约束放进 `GenerateRequest.Tools`。后者始终是可执行 Agent Tool，必须经过 Tool Registry、Scope、Handle、权限重校验和确认协议。

传输协议只用于从 `AdapterRegistry` 选择请求/响应映射，不代表 Deployment 具备任何具体结构化输出能力。每个 Adapter 通过 `StructuredOutputStrategyProvider` 声明已实现策略；每个 Deployment 在 `capabilities.structured_output_strategies` 中按可靠性优先级显式声明实际支持策略。Runtime 只从两者交集中按 Deployment 顺序选择，空交集 fail-closed，禁止按协议、供应商、Alias 或远端模型名称猜测和默认回退。

当前标准策略为 `openai_json_schema`、`anthropic_output_config`、`constrained_decoding`、`response_only_function`、`json_mode` 和 `text_json`。OpenAI Responses Adapter 实现 JSON Schema 与 response-only function；Anthropic Messages Adapter 实现 `output_config` 与 response-only function；OpenAI-Compatible Adapter 实现 JSON Schema、response-only function、JSON Mode 与文本 JSON。Adapter 新增实现时只扩展自己的策略声明和私有映射，不修改业务调用方。

Response-only function 不注册到 Tool Registry、不创建 Scope/Handle/Confirmation、不执行函数，也不产生 Tool Step 或工具调用事实；Adapter 只校验函数名并将 arguments 归一为结构化文本。Invocation 层统一拒绝截断、拒绝、非文本 Item 和非法 JSON，业务层继续使用同一 DTO/Schema 执行未知字段与领域语义校验。

OpenAI Responses 的 response-only function 固定只投影一个私有函数，并使用字符串 `tool_choice=required`；不得使用供应商支持不一致的命名函数对象。Adapter 收到结果后仍须校验返回函数名与契约名称一致。

调用账本仅记录最终 `structured_output_strategy` 名称。禁止记录 Schema、Prompt、函数 arguments 或完整响应。格式错误使用 `response_mismatch`，能力不足使用 `capability_not_supported`；调用方只可在总 Job 次数、Token 和超时预算内重试。旧 Deployment 只有 `structured_output` feature、没有策略列表时视为能力元数据不完整，必须由管理员确认后补齐，不自动迁移猜测。

当前结构化输出只支持非流式 `Generate`；流式响应无法在发送增量前完成整份 Schema 校验，因此 `StructuredOutput + Stream` fail-closed。

## Embedding

批量文本向量化走 `EmbeddingRuntime.Embed`，`mode=embedding`，复用同一 Invocation 账本与 `ai.model_invocation.finished.v1`。禁止在 Tool / Knowledge / Memory 中旁路直连 Embedding 供应商。完整约束见 `docs/development/ai-embedding-runtime.md`。

## 调用账本

`ai_model_invocations` 在访问供应商前写入 `running`，终态与 `ai.model_invocation.finished.v1` Outbox 事件在同一事务完成。启动时将超时遗留的 `running` 标记为 `unknown`，绝不自动重放供应商调用。

账本和 Outbox 仅允许保存路由标识、状态、时间、用量、finish reason、受控错误分类和关联 ID。Embedding 可额外记录 purpose、item count、输入字节、dimensions 与子批数。禁止保存提示词、模型输出、工具参数、原文、向量、凭证、供应商原始错误或响应体。

本地协议排障可临时设置 `AINP_AI_PROVIDER_DEBUG_RAW=true`，并将 `logging.level` 设为 `debug`。此时非流式 HTTP Adapter 只向当前进程控制台输出供应商原始成功/错误响应体，单条最多 64 KiB并标记截断；不输出请求体或凭证，不写 Invocation、Job、Outbox、审计或数据库。该开关禁止用于生产，排障结束必须立即关闭并清理包含业务正文的终端日志。

## 测试要求

Adapter 变更必须在 `platform/tests` 增加请求/响应 fixture、结构化输出策略映射、流事件顺序、取消/超时与敏感字段测试。供应商协议变化应先更新 fixture，再调整映射。
