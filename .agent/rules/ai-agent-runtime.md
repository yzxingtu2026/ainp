# AI Agent Runtime 硬约束

- Run 必须固定 actor/session/auth version、Conversation/trigger Message、不可变 Agent Version、预算与 ContextSnapshot。
- HTTP 只写 Run/控制事实；执行必须经版本化 Outbox → Job，禁止请求内无界循环。
- Run/Step 状态迁移 fail-closed；重复 Job/事件/Step/工具副作用/最终消息必须幂等。
- Worker 必须使用行锁 + lease；有效 lease 和终态重复投递 no-op，只有过期 lease 可接管。
- 每次执行/恢复重新加载当前用户授权；禁止平台管理员身份、长期全权限 token 或跳过 confirmation。
- `purpose=builder_preview` 才允许 draft agent 执行；workspace 禁止 preview Version；非 builder 路径不得放宽 enabled。
- 模型只经统一 Model Runtime，工具只经 Tool Runtime，压缩只经 ContextCompactor。
- ToolConfirmation 暂停时释放 lease；恢复必须校验 auth、permission、Agent/Tool/Schema、参数 hash、会话 revision 与 Snapshot。
- assistant/tool 消息只经 ConversationService 追加；模型 tool_call 必须先落 assistant 再落 tool_result，二者成对；失败/取消禁止伪造完成消息。
- 事件 cursor 单调且 tenant/run 隔离；日志、Outbox、Step/Event 禁止 Secret、Handle、完整敏感参数、压缩摘要与供应商原始响应。
