# AI 长期记忆硬约束

- `MemoryItem` 是追加式事实源；FTS/vector/day/profile 都是带 revision 与 content hash 的可重建投影，禁止以投影充当授权或事实源。
- occurred/observed/recorded/valid 时间语义必须分离；禁止用 created_at 代替业务时间。
- 自动流程禁止原地覆盖历史事实；追加新 Item 并用 supersedes/validity 结束旧事实。
- 非用户直接陈述不得自动升级为 active 用户事实；来源正文留在 Conversation/Message，记忆仅存受控摘要、引用与哈希。
- 当前召回必须同步排除 superseded/revoked/expired/deleted、已过期或已失效事实；异步索引清理不得形成授权窗口。
- Vector 只经统一 Embedding Runtime，冻结 alias/deployment/dimensions；正文、向量和候选内容禁止进入普通日志、审计摘要或 Outbox。
- 所有关系使用 tenant/user 复合约束并经 TenantDB/Repository 访问；跨租户、跨用户 evidence、supersedes、projection 必须 fail-closed。
- Memory Policy 发布时按 Agent 输入→租户默认→系统默认解析并冻结；只引用满足 Structured Output 的 Alias，禁止 Provider/Deployment/Credential 进入策略。
- Run 终态 Outbox→Job 异步提取候选；资格检查只能做 committed user/assistant 增量等结构判断，禁止关键词/句式判断长期价值。
- 每 Run 唯一 Evaluation 水位；正常异步与 compaction flush 复用同一 EvaluateRun。模型只能引用允许 Message ID，未知字段/越界来源/非法时间 fail-closed，输出只能保存 candidate。
- 提取日志只含安全计数、类型/敏感度分布、延迟、Usage 与失败分类，禁止消息原文和候选正文。
- Candidate 自动 active 仅限明确用户表达 + normal + 高置信 + preference/fact + Policy 允许；敏感、推断、冲突默认 review，极低置信才自动 reject。
- 重复事实追加 Evidence + duplicate 关系；纠正追加新 Item 并以 supersedes/valid_to 结束旧事实，禁止原地覆盖正文或自动删除。
- active 生命周期变化必须写安全 Outbox，由 Job 重建 day/profile；日归档只按 local_date/timezone/date_precision，unknown/range 显式分组，禁止 observed/recorded 日期回填。
- Consolidation 按 tenant+user lease 串行，模型只能提 noop/duplicate/conflict/summary plan；执行前复核 item scope/revision，禁止模型 activate/supersede/revoke/delete。
