# AI Conversation 与上下文装配

## 边界

Conversation 是租户内、供应商无关的会话事实边界。它固定一个明确的 `agent_version_id`；Agent 发布新版本不会改变已有会话语义。Message 是不可变追加事实，模型运行、压缩和 Workspace 都只能引用或追加，不能覆盖已提交消息。

会话必须声明 `purpose`：`workspace`（默认）或 `builder_preview`。列表缺省只返回 `workspace`，Builder 调试会话不得混入 Workspace；`purpose=workspace` 禁止钉 `kind=preview` 的 Agent Version。

本层不调用模型、不执行工具、不压缩上下文。`ContextAssembler` 只按授权和顺序读取 canonical Message，计算字节与 Token 估算并返回 `within_budget` 或 `compaction_required`。后续 `ContextCompactor` 和 Agent Runtime 负责压缩及执行。

## 权限与可见性

- `tenant_id`、owner 和当前 actor 只来自 `AuthContext`。
- 默认 `private`，只有 owner 可见；`tenant` 会话可供同租户授权用户读取。
- 跨租户、无权 private 会话、消息和 Snapshot 统一按不存在处理。
- 客户端只能提交 `user` 消息，不能伪造 `assistant`、`system` 或 `tool`。
- 附件只保存 `attachment_id + attachment_revision`。每次提交和后续读取都必须重新经过附件 AccessPolicy；当前实现接受附件 SHA-256 或受控更新时间 revision。

## 并发与幂等

- Conversation 使用 `revision` 乐观锁；重命名、归档和恢复必须提交当前 revision。
- `message_create` 强制 `Idempotency-Key`。唯一键为 `(tenant_id, conversation_id, idempotency_key)`，相同请求不会产生第二条消息。
- 消息 sequence 在会话事务内分配并具有数据库唯一约束；游标分页只使用 sequence，不以不稳定时间戳翻页。
- Conversation revision 在每次消息提交后递增，因此旧 ContextSnapshot 可审计，但不能被当作最新上下文。

## Content Part 与敏感信息

Message content 复用 `aimodelruntime.ContentPart`。数据库保存最终 canonical Content Parts；流式 delta 属于内部可重放传输事实，不能替代完成快照，也不能创建多个 assistant Message。

消息正文、附件内容和 Snapshot payload 不进入普通日志、Outbox 或审计摘要。审计只记录会话管理 mutation 的资源与结果。

## ContextSnapshot

Snapshot 固定 conversation revision、Agent Version、模型 Deployment、来源 sequence 范围、parent、kind、策略版本与 Token 估算。Runtime Context 不是 Message；Snapshot 只附加 run/context/policy version、timezone/source 与 evaluated_at refs，不保存完整动态 Prompt、称呼或记忆正文。Snapshot 创建使用租户内会话级幂等键，创建后不可覆盖 payload。

## 前端边界

`frontend/src/service/api/ai.ts` 和 `store/modules/ai-conversation` 提供最小 Client/Store，包括会话分页、消息游标和当前会话状态。完整会话页面、模型流、工具确认与恢复属于后续 AI Workspace。
