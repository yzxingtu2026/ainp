# AI 模型统一调用协议

## 调用边界

- 业务只提交 `model_alias`，不得直接指定 Provider、Deployment、远端模型 ID、Endpoint 或凭证。
- `ModelResolver` 负责租户隔离、启用状态和能力校验；`AdapterRegistry` / `EmbeddingAdapterRegistry` 按协议 fail-closed 分派。
- 当前生成协议支持 OpenAI Responses 与 Anthropic Messages；`openai_compatible` 未纳入本阶段推理 Adapter。
- Embedding 协议支持 OpenAI 与 OpenAI-Compatible `/v1/embeddings`；详见 `ai-embedding-runtime.md`。
- 自定义 Endpoint 配置为服务根地址，不带 `/v1`；Adapter 统一追加资源路径。官方 Endpoint 可留空。

## Generate 与 Stream

统一输入由消息、内容 Part、工具、结构化输出和超时组成。流式输出必须遵循：一次 `response.started`、单调递增序号、Item 先开始再增量、一次终态。未知供应商事件忽略，协议顺序异常立即失败。

HTTP Action 的流式响应使用 SSE。生产者使用有界 channel，使慢客户端形成反压；客户端断开必须向下传播取消。

## Embedding

批量文本向量化走 `EmbeddingRuntime.Embed`，`mode=embedding`，复用同一 Invocation 账本与 `ai.model_invocation.finished.v1`。禁止在 Tool / Knowledge / Memory 中旁路直连 Embedding 供应商。完整约束见 `docs/development/ai-embedding-runtime.md`。

## 调用账本

`ai_model_invocations` 在访问供应商前写入 `running`，终态与 `ai.model_invocation.finished.v1` Outbox 事件在同一事务完成。启动时将超时遗留的 `running` 标记为 `unknown`，绝不自动重放供应商调用。

账本和 Outbox 仅允许保存路由标识、状态、时间、用量、finish reason、受控错误分类和关联 ID。Embedding 可额外记录 purpose、item count、输入字节、dimensions 与子批数。禁止保存提示词、模型输出、工具参数、原文、向量、凭证、供应商原始错误或响应体。

## 测试要求

Adapter 变更必须在 `platform/tests` 增加请求/响应 fixture、流事件顺序、取消/超时与敏感字段测试。供应商协议变化应先更新 fixture，再调整映射。
