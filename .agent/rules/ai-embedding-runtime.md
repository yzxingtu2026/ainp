# AI Embedding Runtime 硬约束

- 业务只能通过 `model_alias` 调用统一 `EmbeddingRuntime.Embed`，禁止直连供应商 Embedding API/SDK，禁止用 Chat/Generate 伪造向量。
- Resolver 强制 tenant、enabled、`model_type=embedding`、`embeddings` 能力与 dimensions 限制；未知协议 fail-closed。
- Tool/Navigation/Knowledge 等设置写入前必须按用途解析 Alias；配置、能力、认证和非法响应 permanent，临时 provider 故障 retryable，superseded 成功收口。
- JobRun、结构化日志与索引文档必须保存一致的受控分类和阶段；禁止保存原文、向量、Secret 或 provider 原始响应。
- Adapter 只做协议互转；可拆子批但必须恢复顺序并形成一次逻辑 invocation，子批失败整次失败。
- 自定义/OpenAI-Compatible Endpoint 默认按最多 10 个 input 拆 Provider 子批；业务必须尊重 RuntimeError.Retryable，确定性 4xx 不得盲目重试。
- Runtime 不默认归一化/截断/降维，不内建跨业务向量缓存；业务负责 `[]float32` 到存储类型的转换。
- Invocation `mode=embedding` 的终态与 `ai.model_invocation.finished.v1` 同事务；禁止保存原文、向量、凭证或供应商原始响应。
- 测试统一放在 `platform/tests`。详见 `docs/development/ai-embedding-runtime.md`。
