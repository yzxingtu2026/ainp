# AI Embedding Runtime

## 调用边界

业务模块（Tool Index、Knowledge、Memory）只通过统一 `EmbeddingRuntime.Embed` 调用，只提交 `model_alias` 与受控文本输入，禁止：

- 直连 OpenAI / OpenAI-Compatible Embedding API 或供应商 SDK
- 传入 Provider、Endpoint、API Key、远端模型 ID、encoding format 或供应商私有字段
- 用 Chat/Generate 伪造向量
- 在 Runtime 内建跨业务 / 跨租户通用向量缓存

统一链路：

```text
Tool / Knowledge / Memory
  → EmbeddingRuntime.Embed(alias, inputs)
  → ModelResolver（model_type=embedding + embeddings）
  → EmbeddingAdapterRegistry
  → OpenAI / OpenAI-Compatible Adapter
  → 统一 EmbeddingResponse（[]float32）
```

Runtime 只负责批量文本 → 向量与 Usage；pgvector 表、切块、检索、索引生命周期由调用模块负责。业务侧将 `[]float32` 转为存储类型，禁止 `[]any`、字符串或 base64 向量泄漏到业务层。

## 协议

- `EmbeddingRequest`：`model_alias`、有序 `inputs[{id,text}]`、可选 `dimensions`、`purpose`、`timeout_ms`、可选显式 `preprocessing`
- `purpose` 稳定值：`tool_index` / `knowledge_ingestion` / `knowledge_query` / `memory` / `other`

Tool Runtime 使用租户设置 `ai.tool_search.embedding_model_alias` 选择受治理 Alias。Catalog 索引和查询都必须使用 `purpose=tool_index` 与 `cosine_normalize.v1`，并校验 deployment、dimensions、preprocessing、schema version 和 catalog revision；禁止恢复旧的 `aitool.EmbeddingService` 或由 Tool 模块直连供应商。
- 首期仅文本；空白文本、重复 item ID、单项/批量字节与预计 Token 超限在 Provider 调用前拒绝
- `EmbeddingResponse`：安全 Model 摘要、按输入 ID/顺序的 vectors、dimensions、Usage、sub_batch_count
- 默认不归一化、不截断、不降维；仅当显式 `preprocessing=cosine_normalize` 时归一化，并记录 `preprocessing_version`

## Adapter 与限制

- `openai` 与 `openai_compatible` 注册 Embedding Adapter，路径统一为根 Endpoint + `/v1/embeddings`
- `openai_compatible` 必须配置自定义 Endpoint；未实现或不合法响应 fail-closed
- 不提供 Anthropic Embedding Adapter
- 平台硬限制：单批 item 数、单项/总字节、预计 Token、返回向量总元素数
- Adapter 可拆子批，但必须恢复原始顺序并形成一次逻辑 invocation；任一子批失败则整次失败
- 官方 OpenAI Endpoint 可使用协议上限；自定义或 OpenAI-Compatible Endpoint 默认按最多 10 个 input 拆分 Provider 子批，兼容常见供应商限制。
- 调用方必须尊重 `RuntimeError.Retryable`：400 等确定性请求错误不可盲目重试，429、超时与 5xx 才进入可靠重试。

## 调用账本

复用 `ai_model_invocations` 与 `ai.model_invocation.finished.v1`：

- `mode=embedding`
- 记录 purpose、item count、输入字节、dimensions、子批数、状态、时延、Usage、request/trace ID
- 禁止写入原始文本、文本 hash 明文、向量或供应商原始响应
- Usage 未知保持 null，不得把估算值当供应商计费事实
- 终态与 Outbox 同事务；调用模块不得再写平行模型用量事件

## 注入与测试

- 应用装配后通过 `app.EmbeddingRuntime` 注入给 Tool / Knowledge / Memory

## 配置与失败协议

Tool Search、Navigation Search 等设置在写入前通过统一 Setting Validator 解析 Embedding Alias；Alias 必须在当前租户解析到 enabled、能力合格且 Adapter 可用的 Embedding Deployment。空 Alias 仍表示显式使用词法降级。Knowledge 在自身创建、更新和启用边界复用同一 `EmbeddingRequirements`，业务模块不得复制能力判断。

Embedding 索引 Job 使用统一安全分类：configuration/capability/authentication/invalid response/provenance 为 permanent；timeout/rate limit/provider unavailable 为 retryable；persistence 故障为 retryable；superseded revision 成功收口。JobRun、结构化日志和索引文档写入一致的 `error_category`、`error_stage` 与受控摘要，禁止保存完整输入、向量、Secret 或 provider 原始响应。配置修正后通过 reconcile/reindex 恢复 ready vector 与 provenance。
- Runtime 不得反向依赖 Tool / Knowledge / Memory 模块
- 默认测试使用 Fake Adapter 与 httptest fixture；不依赖真实网络、Key 或额度
- 集成测试覆盖租户隔离、invocation/Outbox 原子性与跨租户拒绝，统一放在 `platform/tests`
