# AI 长期记忆事实与投影协议

## 边界

`ai_memory_items` 是租户用户长期记忆的追加式事实源；全文、向量、按日摘要和当前画像均为可删除、可重建的投影。默认作用域为 `tenant_id + user_id`，可用 `agent_id` 进一步隔离。对话与消息继续保存来源正文，记忆只保存受控摘要、引用和内容哈希，禁止复制完整对话、工具结果或第三方原文。

时间语义必须分开：`occurred_at/occurred_end_at` 表示发生时间，`occurred_timezone/local_date/date_precision` 表示用户语义日期，`observed_at` 表示系统获知时间，`recorded_at` 表示正式记录时间，`valid_from/valid_to` 表示事实有效区间。不得用 `created_at` 替代任一业务时间。

## 生命周期与来源

- 状态为 `candidate / pending_review / active / superseded / revoked / expired / deleted`。自动流程追加新 Item 并用 `supersedes_item_id` 串联历史，禁止原地覆盖证据。
- 只有用户直接陈述可直接成为 `active` 用户事实；助手推断、工具、知识库和网页来源必须停留在候选或待审核状态。
- Evidence 可引用 Conversation、Message、Run、Agent、Agent Version、actor 与提取 Invocation。来源撤权后同步改变事实可召回性，异步 Job 只负责物理索引清理。
- 当前查询只返回 `active` 且未过期、处于有效期内的事实；因此撤销、删除、过期或 superseded 不依赖投影清理完成。

## 投影

`exact / fts / vector` 投影必须保存 Item revision 与 content hash。Vector 只能通过统一 Embedding Runtime 生成，并冻结 model alias、deployment、dimensions；元数据不完整时 fail-closed。`ai_memory_day_summaries` 和 `ai_memory_current_profiles` 只定义重建边界，本阶段不实现模型整理流程。

所有表必须注册 tenant data access Registry。Item、Evidence、Version、Projection 的数据库关联使用包含 tenant/user 的复合外键，应用服务也必须在创建 supersedes 等关系前复核用户作用域。日志、审计摘要和 Outbox 事件仅允许 ID、状态、类型、revision 与安全计数，不得携带正文、Embedding 或候选内容。

## Memory Policy

Memory Policy 包含开关、extraction/consolidation Alias、低敏自动保存、写入审批、Agent 专属作用域、各阶段预算及 policy/prompt/schema version。解析优先级固定为 Agent Version 输入、租户 `ai.memory_policy` 默认、系统默认；发布时冻结有效策略和来源。模型只引用 Alias，必须支持 Structured Output；consolidation 为空回退 extraction，不得静默切换到其他未声明模型。

## 每轮候选提取

成功完成的 Agent Run 在终态事务写 `ai.agent_run.terminal` Outbox 后，由独立 `ai.memory.evaluate_run` Job 异步评估。资格检查只验证 tenant/user/conversation/run、committed user/assistant 文本和冻结策略，不得使用业务关键词或句式字典判断长期价值。纯工具结果、derived/附件/网页文本、未完成 Run 和关闭策略只记录安全的跳过分类。

每个 Run 对应唯一 `ai_memory_evaluations` 水位事实；并发、重试和 compaction flush 都复用 `MemoryExtractionService.EvaluateRun`。输入只包含本轮用户文本、助手最终文本、允许引用的 Message ID、可信时间以及冻结版本号和预算。Structured Output 禁止 tenant/user/run/actor 字段，未知字段、越界来源、非法时间和超预算候选 fail-closed。合法空数组表示已评估且没有长期价值。

模型输出只落 `candidate`，并保存 Evaluation、Agent Version、冻结 Policy、Invocation、prompt/schema version 与 arguments hash。候选不会自动变为 active。日志只记录 Run ID、安全计数、类型/敏感度分布、耗时和 Usage，禁止原文与候选正文。
