# AI Agent Run Runtime

Agent Runtime 把不可变 Agent Version 执行为可暂停、可恢复、可审计的 Run/Step。HTTP 只创建或变更 Run 事实；实际循环由版本化 Outbox 触发 Job，禁止在请求中维持无界模型循环。

## 状态与事实

Run 固定 tenant、actor、session/auth version、origin request ID、trace ID、conversation、trigger message、agent version、conversation revision、可选 Page Context 安全快照、预算、当前 ContextSnapshot，以及首次执行生成的 evaluated_at/timezone Time Snapshot。Step 追加记录 context assemble/compact、model、tool search/load/execute、confirmation、finalize，输入输出只保存安全摘要。

Run 状态为 queued/running/waiting_confirmation/paused/completed/failed/cancelled；Step 为 pending/running/waiting/completed/failed/skipped/cancelled。非法迁移 fail-closed，completed/cancelled 不可恢复；failed 只能显式 retry。

## 运行预算

租户管理员只通过“系统设置 → AI → Agent 运行预算”维护 `ai.agent_runtime_budget`。解析顺序固定为：平台硬上限 → 租户设置默认/覆盖 → Run 创建时固化。客户端 Start 请求不接受预算字段，不能放大服务端限制；Agent Version 级覆盖属于未来能力，首期不实现。

Run 固化的版本化预算使用 `schema_version=1` 与 snake_case 字段：`max_steps`、`max_model_calls`、`max_tool_calls`、`max_compactions`、`max_duration_seconds`、`run_token_budget`。历史 PascalCase `BudgetJSON`（含 `MaxTokens`）仅做兼容读取，新 Run 禁止继续写旧格式。设置读取或类型转换失败必须 fail-closed；已经创建的 Run 始终读取自身快照，不受后续设置修改影响。

`run_token_budget` 是整个 Run 的累计 input + output Token 上限，不是单次模型上下文容量。单次上下文容量统一按目标 Deployment 的 `context_window - reserved_output_tokens - safety_margin_tokens` 计算，默认输出预留为 Deployment `max_output_tokens`（缺失时 2048），安全余量为上下文窗口的 10%；ContextAssembler 与 ContextCompactor 必须复用同一算法，后者再扣除 instructions、工具 Schema 和附件预算。

每次模型调用必须在最终请求装配完成后经过统一 Turn Budget Planner。估算范围包含 instructions、消息、动态 Tool Schema、附件、Page Context 与 Memory Recall。Planner 将累计硬预算拆为两层：

- 正常 ReAct 只能使用扣除最终输入、最终输出与估算安全余量后的软预算；`max_output_tokens` 同时受 Deployment 上限和软预算约束。
- 下一次正常调用将侵占收尾预留、或只剩最后一次模型调用额度时，Run 持久化切换为 `execution_mode=completion_only`。该状态在 confirmation 暂停、恢复、lease 接管和 retry 中不得回退为 normal。
- completion-only 最多调用一次。调用前先持久化 `completion_attempted`，临时追加 `ai/budget-completion` runtime 指令，并在协议层清空 Tool Scope/Tools、设置 `tool_choice=none`；指令不得写入 canonical Message。
- 收尾成功仍提交 canonical assistant Message，Run 为 `completed` 且 `completion_reason=budget_limit`。前端显示本地化预算提示，不向普通用户暴露预算错误机器码。
- 剩余预算无法容纳最小收尾、压缩不能降低上下文、Provider 失败或仅返回不可用输出时才 fail-closed。若调用已经返回无新工具依赖的可用文本，即使真实 Usage 超出估算安全余量，也先幂等提交该文本，不得直接丢弃。

收尾预留策略必须是集中、确定性且可测试的实现，客户端、Prompt 和租户设置均不能绕过 `run_token_budget` 硬上限。预算日志只记录上限、累计/估算 Token、输出额度、预留、模式、原因和压缩次数，不记录请求正文、工具参数/结果或 Secret。

平台默认值与硬范围：

| 字段 | 默认值 | 范围 |
| --- | ---: | ---: |
| `max_steps` | 24 | 1–100 |
| `max_model_calls` | 8 | 1–32 |
| `max_tool_calls` | 12 | 0–64 |
| `max_compactions` | 2 | 0–3 |
| `max_duration_seconds` | 300 | 10–1800 |
| `run_token_budget` | 100000 | 1–10000000 |

Builder 预览约束：

- Conversation `purpose=builder_preview` 时允许 agent 为 `draft` 或 `enabled` 执行；`disabled` 仍 fail-closed。
- `purpose=workspace` 不得执行 `kind=preview` 的 Agent Version。
- 非 builder_preview 路径不得放宽 agent.enabled 要求。
- Builder 消息附件必须复用统一 `AttachmentPicker` / 上传队列；当前仅允许最多 4 张图片，以 attachment ID + revision + 受控 detail 构造 `input_image`，并按当前 Alias 的 vision 能力 fail-closed。禁止另建上传协议或提交 URL、Object Key、base64、tenant_id、短时 Token。

## 执行与恢复

- Worker 使用行锁和短 lease 抢占。同一 Run 的重复 Job、有主 lease 或终态均幂等 no-op；过期 lease 才允许接管。
- 第一次执行经统一 RuntimeContextProvider 生成不可变时间快照；同一 Run 的模型调用、恢复和 retry 复用原快照。动态可信身份/时间区块靠近本轮 user message，固定 Agent instructions 不变。
- Run Start 对可选 Page Context 同步执行服务端 Resolver；只保存安全投影并在触发 user message 附近临时注入。恢复/重试复用原投影，禁止重读当前页面或写入 canonical Message。
- 每次 Job 从 AuthorizationProvider 重新加载 actor 当前权限和 auth version，不使用平台管理员身份或长期 service token。
- 授权恢复后必须沿用 Run 固定的 origin request/trace，并设置 `client=agent`、可信 `agent_run_id`；Job/Outbox 的关联 ID 与 client 只用于投递，不得覆盖最终 Action 执行主体。
- 模型调用只经统一 Model Runtime；Agent 执行使用 `Stream`，Provider 的 `output_text.delta` 到达后立即追加为 `run.delta`，禁止等待完整 `Generate` 响应后再人为切片伪造流式输出。调用前由 ContextCompactor 按目标模型判断预算并固定 Snapshot。
- Provider context overflow 只允许受预算限制的压缩重试；step/model/tool/compaction/Token/时长均有硬上限。
- 普通轮已返回并落库的 Tool Call 可在既有 tool/time/confirmation 预算内完成；若随后进入 completion-only，恢复后只能消费已预留的单次收尾调用，不接受或执行新的 Tool Call，也不为违规 Tool Call 伪造 Tool Result。
- ToolScope 只允许 Agent bindings 与 meta search/load。Handle 只在服务端内存使用，不写 Message/Event。
- ToolInvocation 创建后，Tool Runtime 将可信 invocation ID 注入执行上下文；Action Audit 与 ToolInvocation 必须共享 Run 固定的 request/trace，并分别记录 `agent_run_id`、`tool_invocation_id`。
- Knowledge binding 只限定候选 Knowledge Base；每次模型调用前由统一 `KnowledgeRetriever` 使用 Run 当前 actor 重鉴权并执行预算受控检索。结果只能作为 `user` 级不可信参考消息注入，禁止写入 Contract、system/developer instructions、Snapshot 或事件摘要。
- 记忆启用且用户允许自动记忆时，模型调用前由统一 Memory Retriever 执行受预算混合召回；无高质量结果不注入，结果只能进入 `UNTRUSTED USER MEMORY` user reference，Snapshot 只记录 provenance refs。
- 敏感工具复用 ToolConfirmation。Run 保存 confirmation/invocation 引用并释放 lease；恢复前校验 actor、auth version、会话 revision、Agent/Tool/Schema 和已持久化调用结果。
- 确认恢复必须同时接受 invocation 的 `completed` 与 `failed` 终态，并为原 assistant tool_call 追加一一配对的成功或失败 Tool Result。`invalid_arguments` 等可纠正失败允许模型生成新参数并发起新的 Tool Call 与逐次确认，禁止修改或复用旧 confirmation；只有确认执行成功才补足收尾 Step 额度。
- 已确认工具成功恢复后必须至少保留 4 个收尾 Step；仅补足不足的 Step 额度，不重置或扩张 model/tool/compaction/Token/时长预算，普通执行与未成功确认不得获得该额度。
- 模型返回 tool_call 后，必须先幂等追加 assistant（含 `tool_call`）消息，再追加 tool result；二者成对出现，禁止只落 tool_result。最终无工具回复经 finalize 提交；失败或取消不伪造 assistant 完成消息。

## 事件与敏感信息

Run 事件使用单调 sequence，查询和 SSE 均按 tenant/user + run 过滤，支持 `after_sequence` 续传。事件仅包含状态、游标、Step 类型、安全错误分类和可展示文本 delta；不包含 Secret、工具 Handle、完整敏感参数、压缩摘要或供应商原始响应。

Run/Step 终态与 `ai.agent_run.terminal.v1` 在同一事务提交。requested/resume 事件经 Outbox → Job，Job 使用稳定 idempotency key，崩溃后从 canonical Message、Snapshot、ToolInvocation 和 Confirmation 重建。

`ai.agent_run.execute` 固定路由到稳定 `ai` 资源队列。Demo/生产可部署只消费 `ai` 的独立 Worker，与不消费 `ai` 的通用 Worker 隔离执行槽；本地未配置进程覆盖时仍由单 Worker 消费全部队列。首次持久化 Stream Delta 时记录 `agent_first_delta_latency_ms=first_delta_at-run.started_at`，仅作为结构化时延字段，不把 tenant/user/run ID 作为指标 label。
