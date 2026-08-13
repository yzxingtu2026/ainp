# 统一 Tool Runtime

新增工具的代码示例、风险选择、测试模板和交付检查清单见 [AI 工具开发指南](./ai-tool-development.md)。

## 渐进式披露

`ToolRegistry` 是完整工具定义的服务端事实源，不等于模型 Prompt。模型只获得 Agent 的 Contract 必选项与用户可选绑定；没有有效绑定的翻译、摘要等 Agent 不注入任何工具。绑定 `tool.search`、`tool.load` 后才启用渐进发现；`tool.search` 返回最多 10 个不含完整 Schema 的安全候选，`tool.load` 受单次数量、Scope 总量和 32 KiB Schema 预算限制。`knowledge.search` / `memory.search` 等条件工具只有已绑定且实时资源条件满足时才投影。

同一 Agent Run 的 `loaded_tools` 是单调递增集合，每轮模型调用都投影 `allowed_tools + loaded_tools`；不同 Run 不继承 Scope。Progressive 工具未进入当前投影时不得通过普通执行路径隐式 load，必须先调用 `tool.load`。重复 load 已在 Scope 内的工具只返回紧凑 `already_loaded_tool_ids`，不得再次把完整 Definition 写入模型消息；首次 load 才返回完整 Schema。Search/Load 使用稳定 Registry ID（如 `action:system.account_update`），执行必须使用投影后的模型工具名（如 `action__system_account_update`）；模型混用两者时返回可恢复错误并允许同批后续 Search/Load 继续，禁止直接终止 Run。

调用方使用 `enable_tools=true` 创建新 Scope，后续携带返回的 `tool_scope_id`。禁止直接提交 `tools` 绕过 Registry。结构化输出的 response-only function 是 Model Adapter 私有输出约束，不进入 Registry、Scope、Handle、Confirmation、Step 或执行事实，也不得由业务调用方通过 `Tools/ToolChoice` 构造。`tool.execute` 是服务端内部协议，不是常驻模型工具。

## Registry 与 Provider

工具 ID 必须为 `{source}:{namespaced-id}`。Definition 必须分别声明稳定 `Name` / `DescriptionForLLM` 与插件自带的 `TitleForHuman` / `DescriptionForHuman`；框架不要求插件预注册平台 i18n key，管理界面只展示 Human 契约，Human 文案不参与 Schema Hash、Handle 或 Embedding 文档。Definition 还必须显式声明 `binding_mode=forbidden|optional` 与 `loading_mode=direct|conditional|progressive`；需要原子绑定的工具用稳定 `binding_group` 聚合。首期实现 `meta` 与 `action`，`builtin/plugin/http/mcp` 只能通过同一 Provider/Executor 接口扩展。未知来源、重复 Provider、非法 Schema、缺少 Human/风险/绑定/加载元数据均在注册或刷新时 fail-closed。

Action 是否暴露以注册处的 `Metadata.AI` 为唯一事实源：仅 `AIMeta.ExposeTool=true` 自动投影为 Action Tool，`AI=nil` 默认不暴露。暴露项使用 `action.InputSchemaFor[Request]()` 生成显式输入契约，响应使用 `action.SchemaFor[Response]()`；HTTP 与 Tool 必须复用同一强类型 DTO 和 Handler。字段 required 只由显式验证元数据决定，局部更新的缺失字段不得被解码零值覆盖；Tool Runtime 禁止补字段、隐式强转或维护 Agent 专用 DTO。暴露项还必须声明 tags、风险、副作用、确认策略与安全结果摘要。禁止维护 `action_ai_inventory.go` 等平行暴露清单；Action Executor 只能调用 `ActionRegistry.Invoke`。

远程 MCP 的 OAuth/transport 不属于 Tool Provider。MCP Action Adapter 只复用现有 ActionProvider 投影，并在 list/call 时求 Action 状态、tenant realm、实时 permission 与 OAuth scope 的交集；执行只进入 `ActionRegistry.Invoke`。普通写由 `actions:write` 持续授权；敏感 Action 只有在 Grant 包含 `actions:sensitive` 时投影，首次调用返回逐次确认请求。批准后通过固定 `ainp_confirmation_execute` 治理工具和 opaque confirmation ID 单次恢复原 Action；该工具禁止接收 Action ID 或业务参数，不构成通用 `tool.execute`。外部 `source=mcp` Provider 仍是独立后续能力，禁止借本功能接入 Agent Runtime。

框架核心工具通过 `RuntimeSlot` 注册。Slot 同时声明稳定 Definition、`direct/progressive` 发现模式、动态投影、Executor、Step/Invocation 分类、预算类别与安全观测字段。Slot 跟随能力 owner，例如 `modules/ai/knowledge/slot.go`；根装配只调用 `tooling.RegisterBuiltins` 显式聚合。生产启动不扫描源码、不使用 Go Plugin，也不依赖隐藏 `init()`。

Registry 按 Tool ID 稳定排序，并在启动时拒绝重复 Tool ID、重复 RuntimeRef、非法发现模式或非法元数据。新增 Slot 归属能力 owner，并在根装配的聚合调用增加注册项，不得修改 `ToolService`、`AgentRuntimeService` 或已有工具实现。

Tool Catalog 是完整 Registry 的租户级派生事实，不是权限事实源。应用完成全部模块注册后为已有租户执行幂等 Reconcile，新租户初始化时同步投影；运维可调用 `ai.tool_catalog_rebuild` 修复当前租户。`tool.search` 只读 Registry、Catalog/Search Document 与 Scope，禁止在请求路径刷新或写入 Catalog。

Reconcile 按 Tool ID 比较完整安全定义：新增和变更写入 Catalog，Registry 中已移除的 Tool 保留事实行但标记 `disabled`。每次有效变化推进单调 `catalog.revision`，并在同一事务写 `ai.tool_catalog.changed.v1`；事件 `aggregate_version` 等于 Catalog revision。Search Document 绑定 `catalog_revision`：启用定义变更后进入 `pending`，词法投影完成后进入 `ready`，停用或移除立即进入 `stale`。Handler 只处理仍匹配当前 revision 的事件，重复或乱序事件不得覆盖新状态。`failed` 保留给后续索引任务记录终态，修复 Action 可再次生成新 revision。

租户设置 `ai.tool_search.embedding_model_alias` 控制语义检索，空值表示只使用 exact + 中文词法归一化 + PostgreSQL FTS。配置 Alias 后，Catalog 事件经 Outbox 入队 `ai.tool_catalog.embed`，Job 只通过统一 `EmbeddingRuntime` 以 `purpose=tool_index`、`cosine_normalize.v1` 生成 pgvector，并持久化 alias、deployment、remote model、dimensions、preprocessing、schema version 与 catalog revision。模型、维度、预处理或 revision 不匹配的向量不得参与查询；失败标记 `failed`，可重试或通过 rebuild 生成新 revision。

核心 `builtin:knowledge.search` 是首个直接投影 Slot，但只在当前 Agent 至少存在一个绑定、当前 actor 可读、enabled 且索引 ready 的知识库时可用。动态描述只能列出这些候选的名称、用途和稳定 ID，不得暴露 tenant、对象存储字段或无权资源。

## Search、Scope 与 Handle

Dynamic Reference 随 `tool.search` 候选 prerequisite 和 `tool.load` Definition 投影，但候选集合永不进入稳定 Input Schema。Discovery Action 成功后，Scope 仅记录 Resolver ID、Discovery Tool ID、候选版本摘要、读取时间和 Invocation ID；证据天然绑定 Scope 的 tenant、actor、session、auth version 与 Agent Run，禁止跨 Scope 复用或复制完整目录。

`require_evidence=true` 的写调用在 confirmation 前执行通用批量预检：缺证据返回 `reference_required`，目录外值返回 `invalid_reference`，均不创建 confirmation。批准后、Executor 前重新解析当前 Resolver；版本变化返回 `reference_stale`，值失效返回 `invalid_reference`，并按统一 confirmation=`consumed`、invocation=`failed`、配对失败 Result 收口。纠正只能重新读取目录并创建新 Tool Call/confirmation。

搜索顺序为安全过滤、精确 ID/名称、关键词/FTS、可选 Embedding 融合。Embedding 不可用时必须退化；向量和搜索文档不是权限事实源。`ProjectModelTools`、`tool.search`、`tool.load`、Handle 执行和确认后执行统一调用 Registry 的动态投影协议，并重新应用 tenant、realm、user permission、Scope 白名单、启用状态和实时资源条件。动态投影只能收紧静态结果，不能把 Scope 外工具扩权加入候选。

模型侧 `tool.search` 的 `source` / `capability` 是可选的稳定代码精确过滤器，不是自然语言检索词；不确定时必须省略并只填写 `query`。Meta Tool Schema 必须暴露 source 枚举和字段说明，`ToolService.Search` 必须在所有入口自行校验，非法过滤不得静默返回空结果。

Page Context 的 server-owned tool hints 只能在 Registry 投影、ToolScope、当前用户 permission/realm/enabled 过滤完成后提高可见候选排序。Hints 不得并入 allowed sources/capabilities/tools，不得自动 load Schema 或绕过 Handle、确认与 execute 重鉴权。详见 `page-context.md`。

中文降级会为安全发现文本生成有上限的 Unicode 双字词，避免仅依赖 PostgreSQL `simple` 分词。exact/名称命中始终优先；词法/FTS 与 provenance 匹配的 vector 候选使用确定性 RRF 融合，最后再应用权限、Scope 和动态投影。Embedding 设置、调用或索引不可用时响应标记 degraded，但 exact + lexical/FTS 仍返回结果。

ToolScope 的授权由两类白名单组成：精确 `allowed_tools` 用于 meta、RuntimeSlot、合法显式 Binding 和已知特殊工具；`allowed_sources + allowed_capabilities` 用于渐进发现。非 Action 工具满足精确 Tool 白名单，或同时满足来源与能力白名单时才进入 Scope；Action 禁止通过精确 Binding 授权，只能命中 Contract 的来源与能力白名单。来源/能力授权只允许 search/load，未 load 的 Action Schema 不得进入模型初始工具列表。

同一 Conversation 已合法激活或完成执行的工具维护为最多 32 项的有界工作集。新 ToolScope 按最近使用、频率、最近激活与 Tool ID 的确定性顺序恢复最多 4 项；工作集只表示投影偏好，不保存 Handle、Schema、参数或结果，也不构成授权凭证。显式 Load、跨 Run 恢复与运行中 rehydrate 必须复用同一激活原语并实时重验 Registry、Scope、actor 权限、动态投影和 Schema 预算。未投影调用只有在当前 Conversation 工作集命中后才可恢复；Schema hash 变化时必须返回配对的中性 `rehydrated` 结果，让现有主循环在正式 Schema 投影后重新生成调用，禁止执行旧参数。

因此新增 Action Tool 必须同时完成三层验证：Action Registry 中 `ExposeTool=true` 且契约完整；目标 Agent Contract 明确允许其 source/capability；当前 actor 拥有 Action permission。缺少任一层都应从 `tool.search` 候选中消失。模块新增 Capability 或扩大 AI 暴露范围时必须同步更新目标 Contract，并增加基于真实 ToolScope 的可达性与无权限不可见测试；只验证全局 Registry 投影会漏掉 Contract Scope 回归。

Handle 使用服务端签名并绑定 tenant、user、realm、session/auth version、scope、tool、runtime ref、schema hash 和 TTL。任何绑定变化、过期或 Schema 变化均拒绝。

## 确认与事实记录

写入、外部、付费、破坏性、高风险工具默认单次确认，critical 默认禁止。确认绑定不可变 arguments hash；批准后重新校验全部安全条件并单次消费，不提供“始终允许”。

Assistant Tool Call 创建时从当前投影 Definition 固化 `ToolPresentation{title,description}` 快照。模型协议仍保留 `tool_name` / `tool_call_id`，但前端主视图只消费展示快照；历史消息没有快照时才降级显示原始工具名。Definition 后续改名不得改变历史消息，前端禁止解析 `action__...`、`meta__...` 或维护具体 Tool ID 名称映射。

Action Tool 的 `TitleForHuman` 必须来自注册处显式声明的 `AIMeta.ToolTitle`，并采用准确的“动作 + 对象”短语；`Metadata.Name` 不承担工具过程标题职责。`SideEffect` 只用于风险治理，不能据此推导“查询/更新/删除”等标题，因为 read/write 还包含搜索、预览、解析、创建、归档、重置、绑定等不同业务动作。缺少 `ToolTitle` 的 exposed Action 必须在注册校验阶段 fail-closed。

确认创建时在同一事务边界固化版本化 `ConfirmationPresentation`，包含 Tool 标题/描述、操作摘要和结构化 Details。原始 `ArgumentsJSON`、`ArgumentsHash`、`SchemaHash` 始终是批准后执行事实，展示快照禁止反向作为执行输入。SSE 只通知 confirmation ID 和状态，不动态查询业务资源或拼接授权文案。

没有领域 Presenter 时，默认生成器只读取 Definition Human 文案及 Input Schema 明确声明的人类标签，并只展示安全短标量；敏感字段、UUID/内部 ID、数组和嵌套对象不进入普通用户主视图。复杂 Tool 通过 `ToolRegistry.RegisterConfirmationPresenter` 在所属模块注册可信 Presenter。Presenter 可在当前 actor 权限和租户范围内只读解析资源名称，不得产生副作用；查询失败、目标删除或无权读取时 Runtime 自动使用安全默认展示。Tool Runtime 和 SSE 禁止按具体 Tool ID 增加业务分支。

前端确认卡片默认显示标题、描述、摘要、结构化 Details、风险和副作用；原始 Tool ID、Tool Call ID 与脱敏参数摘要进入默认折叠的技术详情。`PresentationVersion=0` 或快照为空表示历史数据，必须兼容读取且不得阻断批准流程。

同一模型响应含多个 Tool Call 时按顺序执行。任一调用进入确认后必须立即停止执行同批后续调用，并为每个未执行调用追加配对的 `deferred_for_confirmation` Tool Result；模型在确认恢复后按需重新发起。批准、拒绝、过期及其他确认终态都必须为原 Tool Call 产生配对结果，禁止留下永久等待卡片。

Runtime 必须先完成同批全部 canonical Tool Result，再固定最终 Conversation revision，最后将 Run 转为 `waiting_confirmation` 并发布等待事件。批准前必须校验 Agent Run 仍在等待该确认、Conversation 仍 active 且 revision 等于批次收口后的快照；用户新增消息、归档或其他可信外部变化必须在 Action 副作用前 fail-closed。

批准使用短事务同时将 confirmation 从 `pending` 认领为 `approved`、将 invocation 从 `waiting_confirmation` 认领为 `running`；任一条件更新失败必须整体回滚。确认恢复后的 Executor 上下文必须使用由 Tool invocation ID 派生的稳定幂等键，同一 invocation 稳定、不同 invocation 隔离，且不得继承批准请求自身的幂等键；未经确认的调用继续使用 Agent Runtime 提供的原始幂等键。Action Executor 在事务外最多执行一次，不允许通过重复批准重放。执行实际开始后，无论成功还是失败，invocation 终态、confirmation=`consumed`、`consumed_at`、安全 Result 与 `ai.tool_invocation.finished.v1` 必须在同一事务提交。失败 Result 只持久化 `call_id`、`status=failed`、安全摘要和稳定 `error_category`，禁止包含原始参数、Secret、数据库错误或堆栈。

对 `consumed/rejected/expired/cancelled` confirmation 的重复批准或拒绝统一返回稳定业务冲突，不得暴露 invocation 内部状态或把查询不到 `pending` 映射为 500。前端在决策请求期间必须同时禁用批准和拒绝；请求完成后按服务端状态刷新列表，失败时保留第一次安全业务错误，不得让后续重复决策冲突覆盖它。

工具调用事实与 `ai.tool_invocation.finished.v1` 同事务完成；目录变化写 `ai.tool_catalog.changed.v1`。Outbox、审计摘要和普通日志不得包含完整参数、结果、Secret、Credential 或第三方原始响应。

所有 Executor Result 在返回模型前统一规范化为 JSON value。typed/untyped struct、map、slice、array、pointer 与 interface 使用同一递归脱敏规则；password、secret、token、authorization、credential 字段一律脱敏。Runtime 使用单一共享编码字节预算约束 `data + truncation`，按稳定 key/数组前缀裁剪且保持 object/array JSON 类型；受控 `truncation.entries` 只报告 path、kind、returned/omitted count 与原因，不生成业务 cursor。Invocation、Outbox 和审计仍只保存安全摘要，不因裁剪诊断扩大 Result 留存。

Slot 的专用预算由通用 Tool Runtime 按 `BudgetCategory/MaxCallsPerRun` 计数；Agent Runtime 不识别具体 Tool ID。Step 类型、Invocation 分类和事实摘要从 Slot 元数据生成，只有 `Observations` 声明的字段及受控来源映射可以进入 Step/事件；未列入 `FailureCategories` 的失败子分类统一收敛为 `execution_failed`。知识工具只允许记录次数、候选/返回/Citation 数、耗时、降级、缓存命中与稳定失败分类；Chunk 正文只存在于发给模型的 Tool Result，不进入这些事实摘要。

## 扩展示例

新核心工具实现 `RuntimeSlot` 并通过显式聚合入口注册；动态第三方目录仍实现 `Source/List/Get` Provider 和 `Execute` Executor。Plugin、MCP 或 HTTP Executor 必须经 Runtime 的 Scope、Handle、权限、确认、幂等、审计、限流和结果脱敏，禁止自行向模型注册或直连执行。
