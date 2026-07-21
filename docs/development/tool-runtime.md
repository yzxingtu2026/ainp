# 统一 Tool Runtime

新增工具的代码示例、风险选择、测试模板和交付检查清单见 [AI 工具开发指南](./ai-tool-development.md)。

## 渐进式披露

`ToolRegistry` 是完整工具定义的服务端事实源，不等于模型 Prompt。模型只获得 Agent 的 Contract 必选项与用户可选绑定；没有有效绑定的翻译、摘要等 Agent 不注入任何工具。绑定 `tool.search`、`tool.load` 后才启用渐进发现；`tool.search` 返回最多 10 个不含完整 Schema 的安全候选，`tool.load` 受单次数量、Scope 总量和 32 KiB Schema 预算限制。`knowledge.search` / `memory.search` 等条件工具只有已绑定且实时资源条件满足时才投影。

调用方使用 `enable_tools=true` 创建新 Scope，后续携带返回的 `tool_scope_id`。禁止直接提交 `tools` 绕过 Registry。结构化输出的 response-only function 是 Model Adapter 私有输出约束，不进入 Registry、Scope、Handle、Confirmation、Step 或执行事实，也不得由业务调用方通过 `Tools/ToolChoice` 构造。`tool.execute` 是服务端内部协议，不是常驻模型工具。

## Registry 与 Provider

工具 ID 必须为 `{source}:{namespaced-id}`。Definition 必须分别声明稳定 `Name` / `DescriptionForLLM` 与插件自带的 `TitleForHuman` / `DescriptionForHuman`；框架不要求插件预注册平台 i18n key，管理界面只展示 Human 契约，Human 文案不参与 Schema Hash、Handle 或 Embedding 文档。Definition 还必须显式声明 `binding_mode=forbidden|optional` 与 `loading_mode=direct|conditional|progressive`；需要原子绑定的工具用稳定 `binding_group` 聚合。首期实现 `meta` 与 `action`，`builtin/plugin/http/mcp` 只能通过同一 Provider/Executor 接口扩展。未知来源、重复 Provider、非法 Schema、缺少 Human/风险/绑定/加载元数据均在注册或刷新时 fail-closed。

Action 是否暴露以注册处的 `Metadata.AI` 为唯一事实源：仅 `AIMeta.ExposeTool=true` 自动投影为 Action Tool，`AI=nil` 默认不暴露。暴露项使用 `action.SchemaFor[Request/Response]()` 从强类型 DTO 生成输入/输出 Schema，并声明 tags、风险、副作用、确认策略与安全结果摘要。禁止维护 `action_ai_inventory.go` 等平行暴露清单；Action Executor 只能调用 `ActionRegistry.Invoke`。

框架核心工具通过 `RuntimeSlot` 注册。Slot 同时声明稳定 Definition、`direct/progressive` 发现模式、动态投影、Executor、Step/Invocation 分类、预算类别与安全观测字段。`platform/modules/ai/tools/register.go` 是唯一显式聚合入口；每个核心工具位于独立子目录，例如 `tools/knowledge/slot.go`。生产启动不扫描源码、不使用 Go Plugin，也不依赖隐藏 `init()`。

Registry 按 Tool ID 稳定排序，并在启动时拒绝重复 Tool ID、重复 RuntimeRef、非法发现模式或非法元数据。新增 Slot 只允许新增工具目录并在聚合入口增加注册项，不得修改 `ToolService`、`AgentRuntimeService` 或已有工具实现。

Tool Catalog 是完整 Registry 的租户级派生事实，不是权限事实源。应用完成全部模块注册后为已有租户执行幂等 Reconcile，新租户初始化时同步投影；运维可调用 `ai.tool_catalog_rebuild` 修复当前租户。`tool.search` 只读 Registry、Catalog/Search Document 与 Scope，禁止在请求路径刷新或写入 Catalog。

Reconcile 按 Tool ID 比较完整安全定义：新增和变更写入 Catalog，Registry 中已移除的 Tool 保留事实行但标记 `disabled`。每次有效变化推进单调 `catalog.revision`，并在同一事务写 `ai.tool_catalog.changed.v1`；事件 `aggregate_version` 等于 Catalog revision。Search Document 绑定 `catalog_revision`：启用定义变更后进入 `pending`，词法投影完成后进入 `ready`，停用或移除立即进入 `stale`。Handler 只处理仍匹配当前 revision 的事件，重复或乱序事件不得覆盖新状态。`failed` 保留给后续索引任务记录终态，修复 Action 可再次生成新 revision。

租户设置 `ai.tool_search.embedding_model_alias` 控制语义检索，空值表示只使用 exact + 中文词法归一化 + PostgreSQL FTS。配置 Alias 后，Catalog 事件经 Outbox 入队 `ai.tool_catalog.embed`，Job 只通过统一 `EmbeddingRuntime` 以 `purpose=tool_index`、`cosine_normalize.v1` 生成 pgvector，并持久化 alias、deployment、remote model、dimensions、preprocessing、schema version 与 catalog revision。模型、维度、预处理或 revision 不匹配的向量不得参与查询；失败标记 `failed`，可重试或通过 rebuild 生成新 revision。

核心 `builtin:knowledge.search` 是首个直接投影 Slot，但只在当前 Agent 至少存在一个绑定、当前 actor 可读、enabled 且索引 ready 的知识库时可用。动态描述只能列出这些候选的名称、用途和稳定 ID，不得暴露 tenant、对象存储字段或无权资源。

## Search、Scope 与 Handle

搜索顺序为安全过滤、精确 ID/名称、关键词/FTS、可选 Embedding 融合。Embedding 不可用时必须退化；向量和搜索文档不是权限事实源。`ProjectModelTools`、`tool.search`、`tool.load`、Handle 执行和确认后执行统一调用 Registry 的动态投影协议，并重新应用 tenant、realm、user permission、Scope 白名单、启用状态和实时资源条件。动态投影只能收紧静态结果，不能把 Scope 外工具扩权加入候选。

模型侧 `tool.search` 的 `source` / `capability` 是可选的稳定代码精确过滤器，不是自然语言检索词；不确定时必须省略并只填写 `query`。Meta Tool Schema 必须暴露 source 枚举和字段说明，`ToolService.Search` 必须在所有入口自行校验，非法过滤不得静默返回空结果。

Page Context 的 server-owned tool hints 只能在 Registry 投影、ToolScope、当前用户 permission/realm/enabled 过滤完成后提高可见候选排序。Hints 不得并入 allowed sources/capabilities/tools，不得自动 load Schema 或绕过 Handle、确认与 execute 重鉴权。详见 `page-context.md`。

中文降级会为安全发现文本生成有上限的 Unicode 双字词，避免仅依赖 PostgreSQL `simple` 分词。exact/名称命中始终优先；词法/FTS 与 provenance 匹配的 vector 候选使用确定性 RRF 融合，最后再应用权限、Scope 和动态投影。Embedding 设置、调用或索引不可用时响应标记 degraded，但 exact + lexical/FTS 仍返回结果。

ToolScope 的授权由两类白名单组成：精确 `allowed_tools` 用于 meta、RuntimeSlot、合法显式 Binding 和已知特殊工具；`allowed_sources + allowed_capabilities` 用于渐进发现。非 Action 工具满足精确 Tool 白名单，或同时满足来源与能力白名单时才进入 Scope；Action 禁止通过精确 Binding 授权，只能命中 Contract 的来源与能力白名单。来源/能力授权只允许 search/load，未 load 的 Action Schema 不得进入模型初始工具列表。

Handle 使用服务端签名并绑定 tenant、user、realm、session/auth version、scope、tool、runtime ref、schema hash 和 TTL。任何绑定变化、过期或 Schema 变化均拒绝。

## 确认与事实记录

写入、外部、付费、破坏性、高风险工具默认单次确认，critical 默认禁止。确认绑定不可变 arguments hash，仅展示递归脱敏摘要；批准后重新校验全部安全条件并单次消费，不提供“始终允许”。

工具调用事实与 `ai.tool_invocation.finished.v1` 同事务完成；目录变化写 `ai.tool_catalog.changed.v1`。Outbox、审计摘要和普通日志不得包含完整参数、结果、Secret、Credential 或第三方原始响应。

Slot 的专用预算由通用 Tool Runtime 按 `BudgetCategory/MaxCallsPerRun` 计数；Agent Runtime 不识别具体 Tool ID。Step 类型、Invocation 分类和事实摘要从 Slot 元数据生成，只有 `Observations` 声明的字段及受控来源映射可以进入 Step/事件；未列入 `FailureCategories` 的失败子分类统一收敛为 `execution_failed`。知识工具只允许记录次数、候选/返回/Citation 数、耗时、降级、缓存命中与稳定失败分类；Chunk 正文只存在于发给模型的 Tool Result，不进入这些事实摘要。

## 扩展示例

新核心工具实现 `RuntimeSlot` 并通过显式聚合入口注册；动态第三方目录仍实现 `Source/List/Get` Provider 和 `Execute` Executor。Plugin、MCP 或 HTTP Executor 必须经 Runtime 的 Scope、Handle、权限、确认、幂等、审计、限流和结果脱敏，禁止自行向模型注册或直连执行。
