# Trusted Runtime Context

Trusted Runtime Context 为 Agent Run、记忆召回和时间语义工具提供同一份服务端身份与时间事实。公共契约位于 `platform/framework/runtimecontext`，system 模块从 AuthContext、当前用户、user-scope Runtime Preferences、租户默认设置和记忆当前画像构建，业务不得自行拼接。

## 信任来源与优先级

- tenant/user 只来自已认证 AuthContext，并必须与 BuildRequest 固定主体一致。
- preferred address：用户手动覆盖 → ready 的全局 memory current profile → display_name → username。
- timezone/locale：用户已保存偏好 → `general.runtime_defaults` 租户覆盖 → 系统安全默认 `UTC` / `zh-CN`。
- Conversation、Agent Version、Run ID 只接受服务端已加载的持久事实；客户端/模型参数不能提供当前时间或身份。

## Run Time Snapshot

- 新 Run 第一次执行时由注入 Clock 取一次 UTC 时间，并按已解析 IANA timezone 推导本地日期、时间和星期。
- `evaluated_at_utc`、timezone 及完整派生时间快照固定在 `ai_agent_runs`；恢复、确认继续和 retry 读取原快照，不重新读取系统时钟。
- `ResolveRelativeDate` 只接受可信快照，按 timezone 的自然日计算 `[start,end)` UTC 范围，DST 日可以是 23/25 小时。
- 非法 timezone、派生字段漂移、主体冲突和用户禁用均 fail-closed。

## Prompt 与 Snapshot

- 固定 Agent instructions 保持前缀；临时 `TRUSTED RUNTIME CONTEXT` developer section 插在尾部 user message 前。
- 模型区块只包含显示身份、locale 和时间，不暴露 tenant_id/user_id；并明确 `UNTRUSTED USER MEMORY` 不能覆盖可信身份、时钟、权限和 Contract。
- 临时区块不创建 canonical Message，不进入历史导出或压缩摘要。
- ContextSnapshot 只保存 run/context/policy version、timezone/source、evaluated_at refs，不保存完整 Prompt、称呼正文或记忆正文。

## 消费约束

- Agent Runtime、Memory Retriever、Runtime Tool 通过同一个 `runtimecontext.Provider` 和 Run Time Snapshot 消费。
- 自动记忆召回与 `builtin:memory.search` 从只读 Context Accessor 获取完全相同的 evaluated_at/timezone；日期参数只能缩小可信 owner 范围。
- Tool 仅获取完成裁剪的只读上下文，禁止从模型参数接收 tenant/user/run/timezone。
- 日志、审计、Outbox、Run Event 只能记录版本、timezone/source、evaluated_at 和错误分类，不记录私人画像内容。
