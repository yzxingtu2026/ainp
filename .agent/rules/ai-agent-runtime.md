# AI Agent Runtime 硬约束

- Run 必须固定 actor/session/auth version、Conversation/trigger Message、不可变 Agent Version、预算与 ContextSnapshot。
- HTTP 只写 Run/控制事实；执行必须经版本化 Outbox → Job，禁止请求内无界循环。
- Run/Step 状态迁移 fail-closed；重复 Job/事件/Step/工具副作用/最终消息必须幂等。
- Worker 必须使用行锁 + lease；有效 lease 和终态重复投递 no-op，只有过期 lease 可接管。
- 每次执行/恢复重新加载当前用户授权；禁止平台管理员身份、长期全权限 token 或跳过 confirmation。
- Run 创建时固定 origin request/trace；执行、重试和确认恢复沿用该关联并设置 `client=agent`，不得继承 Job/Outbox client。Action Audit 与 ToolInvocation 共享关联 ID 并记录可信 Run/Invocation 引用。
- `purpose=builder_preview` 才允许 draft agent 执行；workspace 禁止 preview Version；非 builder 路径不得放宽 enabled。
- 模型只经统一 Model Runtime，Agent 输出必须消费真实 Stream delta，禁止完整 Generate 后切片伪流式；工具只经 Tool Runtime，压缩只经 ContextCompactor。
- ToolConfirmation 暂停时释放 lease；恢复必须校验 auth、permission、Agent/Tool/Schema、参数 hash、会话 revision 与 Snapshot。
- assistant/tool 消息只经 ConversationService 追加；模型 tool_call 必须先落 assistant 再落 tool_result，二者成对；失败/取消禁止伪造完成消息。
- 事件 cursor 单调且 tenant/run 隔离；日志、Outbox、Step/Event 禁止 Secret、Handle、完整敏感参数、压缩摘要与供应商原始响应。
- Agent Run 使用知识时只能把 Agent binding 当候选范围，并通过统一 `KnowledgeRetriever` 按当前 actor 重鉴权；结果必须以 user 级不可信参考注入，禁止进入 system/developer instructions。
- Run 第一次执行必须经统一 RuntimeContextProvider 固定 Time Snapshot；恢复/重试复用快照，可信上下文只临时注入 Prompt 尾部并与不可信记忆分层。
- 可选 Page Context 必须在 Run Start 经显式 Resolver 重解析并固定安全快照；恢复/重试禁止漂移，只临时注入触发 user message 附近，不写 canonical Message。
- 自动 Memory Recall 必须使用 frozen Agent Policy 与用户开关，受条数/Token/敏感等级限制；无结果不注入，Snapshot 只存 provenance refs。
