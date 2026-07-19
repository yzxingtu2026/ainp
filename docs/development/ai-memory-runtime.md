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

## 治理、整理与投影

Candidate 进入确定性治理：只有用户明确表达、normal、confidence ≥ 0.85、类型为 preference/fact 且冻结策略允许时才能自动 active；敏感、推断、低置信和事实冲突默认 pending_review，confidence < 0.2 才自动 rejected。模型 sensitivity/confidence 只能收紧，不能绕过这些门槛。治理决策保存 item revision、原因和幂等键；用户确认/reject/revoke 也必须用 revision 与幂等键。

等价 active 事实不会复制：新 candidate 标为 rejected，创建 duplicate 关系并把新 Evidence 追加到现有事实。带稳定 attribute_key 的明确 preference 纠正通过新 Item active、旧 Item superseded + valid_to、显式 supersedes 关系表达；普通事实冲突进入 review。自动流程禁止原地改写正文或删除事实。

active 变化只在事实事务写安全的 `ai.memory.item.changed` Outbox。独立 Job 从当前 active items 全量重建：

- day summary 使用 local_date/timezone/date_precision 分组；unknown/range 使用明确 date_group，绝不退回 observed_at/recorded_at；只存 item ID/revision provenance 和安全计数摘要。
- current profile 是小容量强类型 JSON，包含 preferred_address、稳定 preferences/facts，并保存 source item revisions、policy version 和 content hash。

## 时间感知混合召回

- `MemoryRetrievalService` 只接受已注入的 Trusted Runtime Context；tenant/user/run/timezone 不从查询参数读取。
- 查询先硬过滤 tenant、user、active、agent scope、expires/validity、日期、类型与敏感等级，再融合 exact、PostgreSQL FTS、ready vector、recency、confidence 和 importance。
- exact/FTS/vector 投影保存 item revision 与 content hash；vector 额外固定 Embedding Alias、Deployment、dimensions。Embedding 不可用或 provenance 漂移时只降级 exact + FTS，不得跳过硬过滤。
- relative date 由 Run Time Snapshot 解析为确定性 UTC/local 范围；`local_date` 与 occurred range 语义分开。
- 自动 Recall Hook 只注入 normal 的 fact/context/event，受 Agent Version `max_recall_items/max_recall_tokens` 和用户 `auto_memory_enabled` 限制；当前称呼继续由强类型 profile 确定性解析。
- 注入使用 `UNTRUSTED USER MEMORY` user-level reference section；Snapshot 仅保存 item/evidence revision refs，不保存正文或向量。
- 唯一主动检索入口为 direct Slot `builtin:memory.search`；只读，禁止 add/update/delete Tool。

第二阶段 consolidation 按 tenant+user 唯一批次和 lease 串行运行，使用冻结 consolidation Alias。模型输入仅含受控摘要、attribute、revision、recall/importance/validity；输出只允许 noop/duplicate/conflict/summary plan。应用层复核 item scope 与 revision 后只追加关系、保存计划和水位，禁止模型 activate/supersede/revoke/delete。

## 用户治理工作台

用户长期记忆治理统一复用 `MemoryWorkbench`，独立隐藏路由为 `/profile/memory`。该路由不进入菜单，当前由个人主页进入，后续 AI Workspace 只组合相同组件、Store 和 Action，不复制治理逻辑。

工作台只允许 `tenant + self`，即使调用者拥有租户管理权限也不能扩大到成员正文。列表、详情与当前画像是 authenticated 自助 Action；确认、拒绝、纠正、删除和清空必须使用 Action 框架持久化幂等。单项 mutation 使用 item revision，清空使用服务端按当前可清理 item ID/revision 生成的不透明 `revision_token`，过期令牌必须冲突失败。

纠正追加新 Item 并保留 superseded 历史；删除和清空在事实事务内先改变状态，使召回同步失效，再经安全 Outbox 重建投影。详情仅展示受控 Evidence 元数据，撤权来源不得回传正文；Prompt、向量、模型原始响应及其他用户信息永不进入治理 API。
