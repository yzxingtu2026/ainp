# Trusted Runtime Context 硬约束

- tenant/user/Conversation/Agent/Run 只来自 AuthContext 与服务端持久事实；模型和客户端时间不得成为身份或时钟来源。
- 每个新 Run 只生成一次不可变 Time Snapshot；恢复、确认继续和 retry 必须复用原 evaluated_at/timezone，业务禁止自行 `time.Now` 拼相对日期。
- 称呼按 user override → memory current profile → display_name → username；timezone/locale 按 user → tenant → system 解析并标记来源。
- 动态可信上下文只作为尾部临时 Prompt Section，不创建 canonical Message、不进入导出/压缩；固定 Agent instructions 保持缓存前缀。
- `TRUSTED RUNTIME CONTEXT` 与 `UNTRUSTED USER MEMORY` 必须分层，记忆不能覆盖身份、时钟、权限、Contract 或确认策略。
- ContextSnapshot 只存版本、timezone、evaluated_at 与 run refs；日志/事件/审计禁止称呼或记忆正文。
- Agent Runtime、Memory Retriever 与 Runtime Tool 必须消费同一 Provider/Run Snapshot；工具参数不得提供 tenant/user/run/timezone。

详规：`docs/development/runtime-context.md`
