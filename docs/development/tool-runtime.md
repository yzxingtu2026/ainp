# 统一 Tool Runtime

新增工具的代码示例、风险选择、测试模板和交付检查清单见 [AI 工具开发指南](./ai-tool-development.md)。

## 渐进式披露

`ToolRegistry` 是完整工具定义的服务端事实源，不等于模型 Prompt。启用工具的模型调用固定注入 `tool.search`、`tool.load`，并可按运行时资源条件投影少量核心工具；`tool.search` 返回最多 10 个不含完整 Schema 的安全候选，`tool.load` 受单次数量、Scope 总量和 32 KiB Schema 预算限制。后续模型请求只投影当前 ToolScope 已加载或满足动态安全条件的真实 Tool Definition。

调用方使用 `enable_tools=true` 创建新 Scope，后续携带返回的 `tool_scope_id`。禁止直接提交 `tools` 绕过 Registry。结构化输出的 response-only function 是 Model Adapter 私有输出约束，不进入 Registry、Scope、Handle、Confirmation、Step 或执行事实，也不得由业务调用方通过 `Tools/ToolChoice` 构造。`tool.execute` 是服务端内部协议，不是常驻模型工具。

## Registry 与 Provider

工具 ID 必须为 `{source}:{namespaced-id}`。首期实现 `meta` 与 `action`，`builtin/plugin/http/mcp` 只能通过同一 Provider/Executor 接口扩展。未知来源、重复 Provider、非法 Schema、缺少风险元数据均在注册或刷新时 fail-closed。

AI Action 必须声明 `AIMeta.ExposeTool`，并使用 `action.SchemaFor[Request]()` 从同一个强类型 DTO 生成输入 Schema。Action Executor 只能调用 `ActionRegistry.Invoke`。

框架核心工具通过 `RuntimeSlot` 注册。Slot 同时声明稳定 Definition、`direct/progressive` 发现模式、动态投影、Executor、Step/Invocation 分类、预算类别与安全观测字段。`platform/modules/ai/tools/register.go` 是唯一显式聚合入口；每个核心工具位于独立子目录，例如 `tools/knowledge/slot.go`。生产启动不扫描源码、不使用 Go Plugin，也不依赖隐藏 `init()`。

Registry 按 Tool ID 稳定排序，并在启动时拒绝重复 Tool ID、重复 RuntimeRef、非法发现模式或非法元数据。新增 Slot 只允许新增工具目录并在聚合入口增加注册项，不得修改 `ToolService`、`AgentRuntimeService` 或已有工具实现。

核心 `builtin:knowledge.search` 是首个直接投影 Slot，但只在当前 Agent 至少存在一个绑定、当前 actor 可读、enabled 且索引 ready 的知识库时可用。动态描述只能列出这些候选的名称、用途和稳定 ID，不得暴露 tenant、对象存储字段或无权资源。

## Search、Scope 与 Handle

搜索顺序为安全过滤、精确 ID/名称、关键词/FTS、可选 Embedding 融合。Embedding 不可用时必须退化；向量和搜索文档不是权限事实源。`ProjectModelTools`、`tool.search`、`tool.load`、Handle 执行和确认后执行统一调用 Registry 的动态投影协议，并重新应用 tenant、realm、user permission、Scope 白名单、启用状态和实时资源条件。动态投影只能收紧静态结果，不能把 Scope 外工具扩权加入候选。

Handle 使用服务端签名并绑定 tenant、user、realm、session/auth version、scope、tool、runtime ref、schema hash 和 TTL。任何绑定变化、过期或 Schema 变化均拒绝。

## 确认与事实记录

写入、外部、付费、破坏性、高风险工具默认单次确认，critical 默认禁止。确认绑定不可变 arguments hash，仅展示递归脱敏摘要；批准后重新校验全部安全条件并单次消费，不提供“始终允许”。

工具调用事实与 `ai.tool_invocation.finished.v1` 同事务完成；目录变化写 `ai.tool_catalog.changed.v1`。Outbox、审计摘要和普通日志不得包含完整参数、结果、Secret、Credential 或第三方原始响应。

Slot 的专用预算由通用 Tool Runtime 按 `BudgetCategory/MaxCallsPerRun` 计数；Agent Runtime 不识别具体 Tool ID。Step 类型、Invocation 分类和事实摘要从 Slot 元数据生成，只有 `Observations` 声明的字段及受控来源映射可以进入 Step/事件；未列入 `FailureCategories` 的失败子分类统一收敛为 `execution_failed`。知识工具只允许记录次数、候选/返回/Citation 数、耗时、降级、缓存命中与稳定失败分类；Chunk 正文只存在于发给模型的 Tool Result，不进入这些事实摘要。

## 扩展示例

新核心工具实现 `RuntimeSlot` 并通过显式聚合入口注册；动态第三方目录仍实现 `Source/List/Get` Provider 和 `Execute` Executor。Plugin、MCP 或 HTTP Executor 必须经 Runtime 的 Scope、Handle、权限、确认、幂等、审计、限流和结果脱敏，禁止自行向模型注册或直连执行。
