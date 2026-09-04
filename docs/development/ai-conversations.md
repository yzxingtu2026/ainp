# AI Conversation 与上下文装配

Canonical Message 永久保留附件 ID、revision、sequence 与顺序。Run 的 Context projection 不等于 canonical 改写：图片累计超过单次媒体预算时，只选择当前触发消息和最近历史组成有界原生工作集；每次 Run 另从全部 canonical `input_image` / `input_file` 确定性投影 Attachment Reference Manifest，固定包含 `source_sequence + attachment_id + attachment_revision + content_kind`。禁止删除历史附件或把会话生命周期累计附件数当成单次 Provider 上限。

## 边界

Conversation 是租户内、供应商无关的会话事实边界。它固定一个明确的 `agent_version_id`；Agent 发布新版本不会改变已有会话语义。Message 是不可变追加事实，模型运行、压缩和 Workspace 都只能引用或追加，不能覆盖已提交消息。

会话必须声明 `purpose`：`workspace`（默认）或 `builder_preview`。列表缺省只返回 `workspace`，Builder 调试会话不得混入 Workspace；`purpose=workspace` 禁止钉 `kind=preview` 的 Agent Version。

本层不调用模型、不执行工具、不压缩上下文。`ContextAssembler` 只按授权和顺序读取 canonical Message，计算字节与 Token 估算并返回 `within_budget` 或 `compaction_required`。后续 `ContextCompactor` 和 Agent Runtime 负责压缩及执行。

## 权限与可见性

- Conversation、Message 与 Agent Run 是所有 tenant 已登录用户的自助能力，不要求额外角色 permission；管理 Agent、Slot、模型或知识库不属于本层。
- `tenant_id`、owner 和当前 actor 只来自 `AuthContext`。Conversation / Run owner 使用 `AuthContext.PrincipalID`；社区等非 Account 主体不写独立 `user_id` 也能建立 private 会话。
- 已注册 Client Surface 上的已登录主体通过 Host 投影路径使用同一套运行面 Action，例如 `{PathPrefix}ai/conversations/create-for-slot`、`messages/create`、`agent-runs/start|get|events|cancel` 与 `tool-confirmations/list|approve|reject`。第一期官方出口是 `events` cursor 轮询，不要求 SSE/WebSocket。
- `create-for-slot` 只解析当前 Surface/Principal 允许的 Slot；社区主体不得解析 `workspace.default`。
- 默认 `private`，只有 owner 可见；`tenant` 会话可供同租户授权用户读取。
- 跨租户、无权 private 会话、消息和 Snapshot 统一按不存在处理。
- 客户端只能提交 `user` 消息，不能伪造 `assistant`、`system` 或 `tool`。
- 图片只保存 `image_attachment_id + attachment_revision`，文档只保存 `file_attachment_id + attachment_revision`。客户端不得提交 `derived_text`；每次提交和 Run 装配都必须重新经过附件 AccessPolicy、ready、MIME、大小与 revision 校验。

## 并发与幂等

- Conversation 使用 `revision` 乐观锁；重命名、归档和恢复必须提交当前 revision。
- `message_create` 强制 `Idempotency-Key`。唯一键为 `(tenant_id, conversation_id, idempotency_key)`，相同请求不会产生第二条消息。
- 消息 sequence 在会话事务内分配并具有数据库唯一约束；游标分页只使用 sequence，不以不稳定时间戳翻页。
- Conversation revision 在每次消息提交后递增，因此旧 ContextSnapshot 可审计，但不能被当作最新上下文。

## Content Part 与敏感信息

Message content 复用 `aimodelruntime.ContentPart`。数据库保存最终 canonical Content Parts；流式 delta 属于内部可重放传输事实，不能替代完成快照，也不能创建多个 assistant Message。

Runtime 为 deferred retry Tool Call 持久化 `retry_of_tool_call_id`，该字段只表达审计与恢复 lineage，不进入执行参数、授权或 Provider 私有字段。Canonical 历史完整保留原 Call/deferred Result 与 retry Call/最终 Result；ContextAssembler 和 Compactor 向模型提供业务视图时，仅在原 Result 确为 `deferred_for_confirmation`、原/retry Call 与 Result 唯一且 Tool/参数一致时，移除 retry Call/Result，并以原 Call ID 归一最终 Result。投影不得修改 canonical Message，字节预算可继续按 canonical 保守计算；孤立、重复、歧义或非法 Result 不折叠并 fail-closed，确保所有 Provider 始终获得合法的一一配对 Call/Result。

消息正文、附件内容和 Snapshot payload 不进入普通日志、Outbox 或审计摘要。审计只记录会话管理 mutation 的资源与结果。

当前触发消息中的即时文档正文只在 Run 内存中由 `DocumentResolver + SafeParser` 展开为 `derived_text`。历史文档不得在每轮无条件读取或解析，模型需要内容时通过 `tool.search -> tool.load -> system.attachment_content_inspect` 按需查看。canonical Message 和幂等重放始终保留 `input_file` 引用；刷新/恢复时前端按当前附件授权读取显示元数据，失败只显示安全不可用状态。

## ContextSnapshot

Snapshot 固定 conversation revision、Agent Version、模型 Deployment、来源 sequence 范围、parent、kind、策略版本与 Token 估算。Runtime Context 不是 Message；Snapshot 只附加 run/context/policy version、timezone/source 与 evaluated_at refs，不保存完整动态 Prompt、称呼或记忆正文。Snapshot 创建使用租户内会话级幂等键，创建后不可覆盖 payload。

Snapshot 的 retained messages 必须把文档 `derived_text` 还原为 canonical `input_file` 引用，禁止持久化解析正文。Attachment Reference Manifest 不进入 Snapshot；Snapshot 消息装配完成后，Runtime 必须从 canonical Message 重新投影并注入 Manifest，不能依赖摘要、retained messages 或历史 Tool Result 偶然保留附件 ID。

## 前端边界

`frontend/src/service/api/ai.ts` 和 `store/modules/ai-conversation` 提供最小 Client/Store，包括会话分页、消息游标和当前会话状态。完整会话页面、模型流、工具确认与恢复属于后续 AI Workspace。
