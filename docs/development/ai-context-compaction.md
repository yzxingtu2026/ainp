# AI 上下文压缩

上下文压缩只生成版本化 `ContextSnapshot`，不修改或删除 canonical Conversation/Message。执行顺序固定为模型预算计算、确定性缩减、结构化摘要、质量校验、不可变 Snapshot。

## 预算

可用输入预算从目标 Deployment 的 `context_window` 扣除输出预留、安全余量、固定 Agent 指令、Tool Schema 和附件展开预算。模型窗口未知或固定内容已经超窗时 fail-closed。`JSONTokenizer` 是确定性默认实现；供应商 Tokenizer 可通过接口替换。

Agent Run 还可用 `run_budget_pressure` 提供更小的目标输入预算。该目标来自“剩余 Run 预算 - 最终输出预留 - 安全余量”，正常 ReAct 还需为下一次最终输入保留额度，因此即使当前请求尚未达到模型 context window，也允许提前压缩。`run_budget_pressure` 只改变压缩目标和 Snapshot reason，不改变 canonical Conversation，也不能放宽模型窗口或 Run 硬预算。

## 压缩策略

- 消息始终按完整边界处理，不切开 Content Part 或 tool call/result。
- 近期原文尾部完整保留；旧区间先移除非 canonical 流式 delta，再进入摘要。
- 多模态只保留 attachment ID/revision 引用，装配阶段继续执行 AccessPolicy。
- 摘要使用严格 JSON Schema，包含目标、决策、约束、副作用、资源、未决问题、待确认事项、失败状态和来源范围。
- `source_message_ids` 必须与被摘要区间完全一致；范围漂移、字段缺失、Token 不降或结果仍超窗均拒绝发布。
- 最大连续压缩深度为 3；超过后返回 `cannot_fit`，禁止循环重试。
- 相同 conversation + idempotency key 已生成 Snapshot 时直接复用，禁止重复摘要；恢复或重放不得重复增加压缩次数。

## 安全事实

Agent Version、tenant/actor/auth、ToolConfirmation、工具副作用与幂等事实由结构化运行时事实固定，不以摘要文本作为授权或恢复依据。摘要 Provider 通过统一 Model Runtime 和 Structured Output 调用，不绑定供应商私有压缩 API。

由 Agent Run 触发的摘要请求必须携带可信 `agent_run_id`，对应 ModelInvocation 的实际 input/output Usage 纳入同一 Run 累计 Token。Run 通过关联 Invocation 汇总 Usage，而不是对重试结果做非幂等加法；压缩失败但供应商已经报告的 Usage 同样计费。

Snapshot 记录 parent、conversation revision、Agent Version、目标及摘要 Deployment、策略/Prompt/Schema 版本、来源 sequence、压缩深度、Token 估算、原因和质量状态。旧 revision Snapshot 仍可审计，但 `is_current=false`。

日志、审计、Outbox 和诊断只输出 Snapshot ID、范围、版本、Token、原因与状态，不输出消息、摘要、工具敏感结果或附件内容。
