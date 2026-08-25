# AI Agent Run Runtime

Agent Run 构造模型请求时必须携带触发消息身份，由 ContextAssembler 先选择有界原生图片工作集，并从 canonical Message 投影每轮 Attachment Reference Manifest。未选历史图片与历史文档不得无条件读取；Manifest 在 ContextSnapshot 装配后重新注入并计入 Context 预算。前置媒体校验失败保留 `invalid_request` 等 Model Runtime 分类，不得统一退化为 `model_failed`。

Agent Runtime 把不可变 Agent Version 执行为可暂停、可恢复、可审计的 Run/Step。HTTP 只创建或变更 Run 事实；实际循环由版本化 Outbox 触发 Job，禁止在请求中维持无界模型循环。

## 状态与事实

Run 固定 tenant、actor、session/auth version、origin request ID、trace ID、conversation、trigger message、agent version、conversation revision、可选 Page Context 安全快照、预算、当前 ContextSnapshot，以及首次执行生成的 evaluated_at/timezone Time Snapshot。Step 追加记录 context assemble/compact、model、tool search/load/execute、confirmation、finalize，输入输出只保存安全摘要。

Run 状态为 queued/running/waiting_confirmation/paused/completed/failed/cancelled；Step 为 pending/running/waiting/completed/failed/skipped/cancelled。非法迁移 fail-closed，completed/cancelled 不可恢复；failed 只能显式 retry。

## 安全终态协议

每个终态统一写入版本化事实：`terminal_protocol_version`、`terminal_reason`、`completion_reason`、`finalization_mode=model|template|notice_only|none`、`finalization_attempted`、`retryable`、`user_message_key`、`safe_args`、`safe_summary`、`terminal_message_id`，以及 completed/failed/unfinished 工具操作计数。`safe_args` 只包含重试布尔值和这三个有界计数。Run、终态 Event 与 `ai.agent_run.terminal.v1` Outbox 使用同一份安全投影并在同一事务提交，SSE 重连和详情查询不得重新推断终态。

集中决策矩阵只允许以下双轨行为：

- 业务校验、资源不存在、Tool 失败、部分结果及可安全降级的文档/上下文错误，在授权已重验、当前 Provider 未失败、模型与 Token 预算仍足够时，可使用 `FailureEnvelope v1` 做最多一次无工具模型收尾。
- Provider/模型认证、限流、超时、不可用、协议错误，授权/租户/安全边界变化，持久化不可用、未知错误和用户取消禁止再次调用模型，直接使用本地化模板或 notice-only。
- Provider 拒绝包含可安全展示文本时，按原文提交 assistant Message，Run 为 `completed` 且 `completion_reason=safety_refusal`；只有拒绝分类时使用本地模板。禁止调用其他模型改写或弱化拒绝。

`FailureEnvelope v1` 由代码从白名单字段构造，只包含稳定终态类别、安全摘要、`retryable`、受控 locale、下一步动作和有界 completed/failed/unfinished 操作计数。禁止接收任意错误 map，也禁止包含 SQL、堆栈、Secret、Token、Handle、对象存储标识、权限内部细节、附件/消息正文、压缩摘要、完整 Tool 参数/Result、Provider 原始响应或隐藏推理。Envelope 仅作为本次临时 user 级输入，不写 canonical Message、日志、Event、Outbox 或普通 API。

模型收尾在调用前原子写入 `finalization_attempted=true`，请求清空 Tool Scope/Tools 并强制 `tool_choice=none`，最多输出 512 Token 且仍受 Run 原始模型次数和 Token 硬预算约束。返回 Tool Call、拒绝、空文本、Provider 错误或超时后只降级一次模板；不得执行 Tool Call、伪造 Tool Result、递归收尾、切换 Provider 或扩大预算。Worker 在调用前后或消息提交阶段崩溃时，恢复只消费稳定 attempt/stream/message 事实：已提交消息补齐终态，未形成可恢复消息则模板降级，绝不重复模型调用。

状态语义保持真实：普通回答和带文本安全拒绝可为 `completed`；Provider、系统、协议和不可恢复 Runtime 故障即使有友好消息仍为 `failed`；用户取消始终为 `cancelled`。模板不是模型内容，不写 assistant Message；`terminal_message_id` 只指向真实持久化 assistant Message。已完成操作计数只统计 `tool_execute + completed + tool_invocation_id`，不得按错误类别或模型文本推断副作用。

## 运行预算

租户管理员只通过“系统设置 → AI → Agent 运行预算”维护 `ai.agent_runtime_budget`。解析顺序固定为：平台硬上限 → 租户设置默认/覆盖 → Run 创建时固化。客户端 Start 请求不接受预算字段，不能放大服务端限制；Agent Version 级覆盖属于未来能力，首期不实现。

Run 固化的版本化预算使用 `schema_version=1` 与 snake_case 字段：`max_steps`、`max_model_calls`、`max_tool_calls`、`max_compactions`、`max_duration_seconds`、`run_token_budget`。历史 PascalCase `BudgetJSON`（含 `MaxTokens`）仅做兼容读取，新 Run 禁止继续写旧格式。设置读取或类型转换失败必须 fail-closed；已经创建的 Run 始终读取自身快照，不受后续设置修改影响。

`run_token_budget` 是整个 Run 的累计 input + output Token 上限，不是单次模型上下文容量。单次上下文容量统一按目标 Deployment 的 `context_window - reserved_output_tokens - safety_margin_tokens` 计算，默认输出预留为 Deployment `max_output_tokens`（缺失时 2048），安全余量为上下文窗口的 10%；ContextAssembler 与 ContextCompactor 必须复用同一算法，后者再扣除 instructions、工具 Schema 和附件预算。

每次模型调用必须在最终请求装配完成后经过统一 Turn Budget Planner。估算范围包含 instructions、消息、动态 Tool Schema、附件、Page Context 与 Memory Recall。Planner 将累计硬预算拆为两层：

- 正常 ReAct 只能使用扣除最终输入、最终输出与估算安全余量后的软预算；`max_output_tokens` 同时受 Deployment 上限和软预算约束。
- 下一次正常调用将侵占收尾预留、或只剩最后一次模型调用额度时，Run 持久化切换为 `execution_mode=completion_only`。该状态在 confirmation 暂停、恢复、lease 接管和 retry 中不得回退为 normal。
- completion-only 最多调用一次。调用前先持久化 `completion_attempted`，临时追加 `ai/budget-completion` runtime 指令，并在协议层清空 Tool Scope/Tools、设置 `tool_choice=none`；指令不得写入 canonical Message。
- `completion_only` 的模式切换发生在实际发起无工具收尾请求之前，而不是正常模型轮返回 Tool Call 之后。正常轮已经形成的 Tool Call 必须先持久化并进入执行/确认；确认恢复追加 Tool Result 后，即使已到 step 边界，也必须保留一次模型消费结果的机会。若预算只允许收尾，runtime 指令必须说明工具仅因本 Run 的预算策略在当前轮禁用，禁止模型声称工具被移除、未加载、不可用或配置发生变化。
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
- Agent consumer 同时消费结构化事件与独立 terminal result，并直接使用 stream 携带的 Invocation ID。Invocation 已经确定的 `timeout`、`cancelled`、Provider 分类或 `stream_protocol_error` 必须原样写入 model Step、Run 和终态 Event；不得在 channel close 后重新猜测分类。`cancelled` 将 Run/活动 Step 收口为 cancelled，其余模型错误收口为 failed。
- 已发送的文本 delta 保持追加式，不因 Run 失败删除；Tool Arguments 只有在完整 tool call terminal 到达且整个模型 stream 成功时才可进入执行循环。超时或取消发生在 arguments 中途时，禁止解析、补全、执行或伪造 Tool Result。
- Provider context overflow 只允许受预算限制的压缩重试；step/model/tool/compaction/Token/时长均有硬上限。
- 普通轮已返回并落库的 Tool Call 可在既有 tool/time/confirmation 预算内完成；若随后进入 completion-only，恢复后只能消费已预留的单次收尾调用，不接受或执行新的 Tool Call，也不为违规 Tool Call 伪造 Tool Result。
- ToolScope 只允许 Agent bindings 与 meta search/load。Handle 只在服务端内存使用，不写 Message/Event。
- ToolInvocation 创建后，Tool Runtime 将可信 invocation ID 注入执行上下文；Action Audit 与 ToolInvocation 必须共享 Run 固定的 request/trace，并分别记录 `agent_run_id`、`tool_invocation_id`。
- Knowledge binding 只限定候选 Knowledge Base；每次模型调用前由统一 `KnowledgeRetriever` 使用 Run 当前 actor 重鉴权并执行预算受控检索。结果只能作为 `user` 级不可信参考消息注入，禁止写入 Contract、system/developer instructions、Snapshot 或事件摘要。
- 记忆启用且用户允许自动记忆时，模型调用前由统一 Memory Retriever 执行受预算混合召回；无高质量结果不注入，结果只能进入 `UNTRUSTED USER MEMORY` user reference，Snapshot 只记录 provenance refs。
- 敏感工具复用 ToolConfirmation。批量 Tool Call 遇确认时先为后续调用写完 `deferred_for_confirmation` 配对结果，再固定最终会话 revision、切换等待状态并发布事件。Run 保存 confirmation/invocation 引用并释放 lease；批准执行前和恢复时都校验 actor、auth version、会话 active/revision、Agent/Tool/Schema 和已持久化调用结果。
- 首项确认成功后，Runtime 必须先于下一次模型调用扫描 canonical Message，并按原响应顺序恢复同批 deferred 队列。每项 retry 使用由 Run ID 与原 Call ID 确定性派生的新 Call ID，创建独立 Invocation 和 Confirmation；批准当前项不批准后续项。拒绝、过期、取消、执行失败、会话 revision 改变、Tool/Step 预算耗尽或 lineage 不一致都立即终止自动队列。
- Worker 恢复只允许继续“retry Call 已提交但无 Invocation”的确定状态；`waiting_confirmation` 恢复最终等待快照，completed/failed Invocation 只从持久安全 Result 补 canonical Result。`running` 等不确定 Invocation 禁止猜测或重放 Action。恢复不重置 model/tool/compaction/Token/时长预算。
- 确认恢复必须同时接受 invocation 的 `completed` 与 `failed` 终态，并为原 assistant tool_call 追加一一配对的成功或失败 Tool Result。`invalid_arguments` 等可纠正失败允许模型生成新参数并发起新的 Tool Call 与逐次确认，禁止修改或复用旧 confirmation；只有确认执行成功才补足收尾 Step 额度。
- 已确认工具成功恢复后必须至少保留 4 个收尾 Step；仅补足不足的 Step 额度，不重置或扩张 model/tool/compaction/Token/时长预算，普通执行与未成功确认不得获得该额度。
- 模型返回 tool_call 后，必须先幂等追加 assistant（含 `tool_call`）消息，再追加 tool result；二者成对出现，禁止只落 tool_result。最终无工具回复经 finalize 提交；失败或取消不伪造 assistant 完成消息。

## 事件与敏感信息

Run 事件使用单调 sequence，查询和 SSE 均按 tenant/user + run 过滤，支持 `after_sequence` 续传。事件仅包含状态、游标、Step 类型、安全错误分类和可展示文本 delta；不包含 Secret、工具 Handle、完整敏感参数、压缩摘要或供应商原始响应。

前端先展示 `terminal_message_id` 指向的持久化 assistant Message；无 Message 时按 `user_message_key` 展示本地化 terminal notice，最后才使用内置通用服务错误。重试按钮必须与 `retryable` 一致，普通用户界面不得展示 `model_failed` 等机器分类。只有持久化事实中存在 `type=tool_execute`、`status=completed` 且关联真实 `tool_invocation_id` 的 Step，才可提示“已完成工具操作不会回滚”；不能仅依据 Run 错误分类推断发生过副作用。

Run/Step 终态与 `ai.agent_run.terminal.v1` 在同一事务提交。requested/resume 事件经 Outbox → Job，Job 使用稳定 idempotency key，崩溃后从 canonical Message、Snapshot、ToolInvocation 和 Confirmation 重建。

`ai.agent_run.execute` 固定路由到稳定 `ai` 资源队列。Demo/生产可部署只消费 `ai` 的独立 Worker，与不消费 `ai` 的通用 Worker 隔离执行槽；本地未配置进程覆盖时仍由单 Worker 消费全部队列。首次持久化 Stream Delta 时记录 `agent_first_delta_latency_ms=first_delta_at-run.started_at`，仅作为结构化时延字段，不把 tenant/user/run ID 作为指标 label。
