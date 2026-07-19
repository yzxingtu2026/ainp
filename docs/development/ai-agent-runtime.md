# AI Agent Run Runtime

Agent Runtime 把不可变 Agent Version 执行为可暂停、可恢复、可审计的 Run/Step。HTTP 只创建或变更 Run 事实；实际循环由版本化 Outbox 触发 Job，禁止在请求中维持无界模型循环。

## 状态与事实

Run 固定 tenant、actor、session/auth version、conversation、trigger message、agent version、conversation revision、预算、当前 ContextSnapshot，以及首次执行生成的 evaluated_at/timezone Time Snapshot。Step 追加记录 context assemble/compact、model、tool search/load/execute、confirmation、finalize，输入输出只保存安全摘要。

Run 状态为 queued/running/waiting_confirmation/paused/completed/failed/cancelled；Step 为 pending/running/waiting/completed/failed/skipped/cancelled。非法迁移 fail-closed，completed/cancelled 不可恢复；failed 只能显式 retry。

Builder 预览约束：

- Conversation `purpose=builder_preview` 时允许 agent 为 `draft` 或 `enabled` 执行；`disabled` 仍 fail-closed。
- `purpose=workspace` 不得执行 `kind=preview` 的 Agent Version。
- 非 builder_preview 路径不得放宽 agent.enabled 要求。
- Builder 消息附件必须复用统一 `AttachmentPicker` / 上传队列；当前仅允许最多 4 张图片，以 attachment ID + revision + 受控 detail 构造 `input_image`，并按当前 Alias 的 vision 能力 fail-closed。禁止另建上传协议或提交 URL、Object Key、base64、tenant_id、短时 Token。

## 执行与恢复

- Worker 使用行锁和短 lease 抢占。同一 Run 的重复 Job、有主 lease 或终态均幂等 no-op；过期 lease 才允许接管。
- 第一次执行经统一 RuntimeContextProvider 生成不可变时间快照；同一 Run 的模型调用、恢复和 retry 复用原快照。动态可信身份/时间区块靠近本轮 user message，固定 Agent instructions 不变。
- 每次 Job 从 AuthorizationProvider 重新加载 actor 当前权限和 auth version，不使用平台管理员身份或长期 service token。
- 模型调用只经统一 Model Runtime；Agent 执行使用 `Stream`，Provider 的 `output_text.delta` 到达后立即追加为 `run.delta`，禁止等待完整 `Generate` 响应后再人为切片伪造流式输出。调用前由 ContextCompactor 按目标模型判断预算并固定 Snapshot。
- Provider context overflow 只允许受预算限制的压缩重试；step/model/tool/compaction/Token/时长均有硬上限。
- ToolScope 只允许 Agent bindings 与 meta search/load。Handle 只在服务端内存使用，不写 Message/Event。
- Knowledge binding 只限定候选 Knowledge Base；每次模型调用前由统一 `KnowledgeRetriever` 使用 Run 当前 actor 重鉴权并执行预算受控检索。结果只能作为 `user` 级不可信参考消息注入，禁止写入 Contract、system/developer instructions、Snapshot 或事件摘要。
- 敏感工具复用 ToolConfirmation。Run 保存 confirmation/invocation 引用并释放 lease；恢复前校验 actor、auth version、会话 revision、Agent/Tool/Schema 和已持久化调用结果。
- 模型返回 tool_call 后，必须先幂等追加 assistant（含 `tool_call`）消息，再追加 tool result；二者成对出现，禁止只落 tool_result。最终无工具回复经 finalize 提交；失败或取消不伪造 assistant 完成消息。

## 事件与敏感信息

Run 事件使用单调 sequence，查询和 SSE 均按 tenant/user + run 过滤，支持 `after_sequence` 续传。事件仅包含状态、游标、Step 类型、安全错误分类和可展示文本 delta；不包含 Secret、工具 Handle、完整敏感参数、压缩摘要或供应商原始响应。

Run/Step 终态与 `ai.agent_run.terminal.v1` 在同一事务提交。requested/resume 事件经 Outbox → Job，Job 使用稳定 idempotency key，崩溃后从 canonical Message、Snapshot、ToolInvocation 和 Confirmation 重建。
