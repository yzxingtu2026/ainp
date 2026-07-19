# AI 长期记忆硬约束

- `MemoryItem` 是追加式事实源；FTS/vector/day/profile 都是带 revision 与 content hash 的可重建投影，禁止以投影充当授权或事实源。
- occurred/observed/recorded/valid 时间语义必须分离；禁止用 created_at 代替业务时间。
- 自动流程禁止原地覆盖历史事实；追加新 Item 并用 supersedes/validity 结束旧事实。
- 非用户直接陈述不得自动升级为 active 用户事实；来源正文留在 Conversation/Message，记忆仅存受控摘要、引用与哈希。
- 当前召回必须同步排除 superseded/revoked/expired/deleted、已过期或已失效事实；异步索引清理不得形成授权窗口。
- Vector 只经统一 Embedding Runtime，冻结 alias/deployment/dimensions；正文、向量和候选内容禁止进入普通日志、审计摘要或 Outbox。
- 所有关系使用 tenant/user 复合约束并经 TenantDB/Repository 访问；跨租户、跨用户 evidence、supersedes、projection 必须 fail-closed。
