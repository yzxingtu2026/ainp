# AI 上下文压缩

Token 预算与媒体预算必须独立收敛。即使 Token 估算为 `within_budget`，ContextAssembler 仍先应用原生图片工作集上限；ContextSnapshot 只能保存该次安全投影，不能覆盖 canonical 附件事实，也不能通过扩大模型窗口绕过 Provider 媒体硬限制。

上下文压缩只生成版本化 `ContextSnapshot`，不修改或删除 canonical Conversation/Message。执行顺序固定为模型预算计算、确定性缩减、结构化摘要、质量校验、不可变 Snapshot。

## 预算

可用输入预算先由公共模型预算函数计算 `context_window - max_output_tokens - floor(context_window / 10)`，再扣除固定 Agent 指令、Tool Schema、Attachment Reference Manifest 和附件展开预算。两项 Deployment Limits 都必须是正整数；缺失、输出不小于窗口或基础可用输入不为正时以 `model.context_budget_invalid` fail-closed，Compactor 不提供默认输出额度。原生图片按受控估算计入；当前触发文档按完整派生文本的 UTF-8 字节/Tokenizer 结果计入，历史文档只计 Manifest 引用且不得在压缩时重复解析。固定内容已经超窗时 fail-closed。`JSONTokenizer` 是确定性默认实现；供应商 Tokenizer 可通过接口替换。

Agent Run 还可用 `run_budget_pressure` 提供更小的目标输入预算。该目标来自“剩余 Run 预算 - 最终输出预留 - 安全余量”，正常 ReAct 还需为下一次最终输入保留额度，因此即使当前请求尚未达到模型 context window，也允许提前压缩。`run_budget_pressure` 只改变压缩目标和 Snapshot reason，不改变 canonical Conversation，也不能放宽模型窗口或 Run 硬预算。

## 压缩策略

- 消息始终按完整边界处理，不切开 Content Part 或 tool call/result。
- 近期原文尾部完整保留；旧区间先移除非 canonical 流式 delta，再进入摘要。
- 多模态只保留 attachment ID/revision 引用，装配阶段继续执行 AccessPolicy。
- Manifest 不进入摘要或 Snapshot payload；无论是否生成 Snapshot，最终模型请求都从 canonical Message 重新注入同一确定顺序的引用清单。
- 摘要使用严格 JSON Schema，包含目标、决策、约束、副作用、资源、未决问题、待确认事项、失败状态和来源范围。
- `source_message_ids` 必须与被摘要区间完全一致；范围漂移、字段缺失、Token 不降或结果仍超窗均拒绝发布。
- 最大连续压缩深度为 3；超过后返回 `cannot_fit`，禁止循环重试。
- 相同 conversation + idempotency key 已生成 Snapshot 时直接复用，禁止重复摘要；恢复或重放不得重复增加压缩次数。
- 文档派生正文不可静默截断。若压缩不能完整保留所有 `document_parse` 派生段，返回 `cannot_fit / attachments_exceed_model_window`，不得摘要替代或丢页/丢行。
- Snapshot 落库前把 retained 文档派生段还原为 canonical `input_file` 引用；恢复后重新授权解析，正文不得进入 Snapshot payload。

## 安全事实

Agent Version、tenant/actor/auth、ToolConfirmation、工具副作用与幂等事实由结构化运行时事实固定，不以摘要文本作为授权或恢复依据。摘要 Provider 通过统一 Model Runtime 和 Structured Output 调用，不绑定供应商私有压缩 API。

由 Agent Run 触发的摘要请求必须携带可信 `agent_run_id`，对应 ModelInvocation 的实际 input/output Usage 纳入同一 Run 累计 Token。Run 通过关联 Invocation 汇总 Usage，而不是对重试结果做非幂等加法；压缩失败但供应商已经报告的 Usage 同样计费。

Snapshot 记录 parent、conversation revision、Agent Version、目标及摘要 Deployment、策略/Prompt/Schema 版本、来源 sequence、压缩深度、Token 估算、原因和质量状态。旧 revision Snapshot 仍可审计，但 `is_current=false`。

日志、审计、Outbox 和诊断只输出 Snapshot ID、范围、版本、Token、原因与状态，不输出消息、摘要、工具敏感结果或附件内容。
