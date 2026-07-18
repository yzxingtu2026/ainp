# 统一 Tool Runtime

新增工具的代码示例、风险选择、测试模板和交付检查清单见 [AI 工具开发指南](./ai-tool-development.md)。

## 渐进式披露

`ToolRegistry` 是完整工具定义的服务端事实源，不等于模型 Prompt。启用工具的模型调用固定注入 `tool.search`、`tool.load`，并可按运行时资源条件投影少量核心工具；`tool.search` 返回最多 10 个不含完整 Schema 的安全候选，`tool.load` 受单次数量、Scope 总量和 32 KiB Schema 预算限制。后续模型请求只投影当前 ToolScope 已加载或满足动态安全条件的真实 Tool Definition。

调用方使用 `enable_tools=true` 创建新 Scope，后续携带返回的 `tool_scope_id`。禁止直接提交 `tools` 绕过 Registry。`tool.execute` 是服务端内部协议，不是常驻模型工具。

## Registry 与 Provider

工具 ID 必须为 `{source}:{namespaced-id}`。首期实现 `meta` 与 `action`，`builtin/plugin/http/mcp` 只能通过同一 Provider/Executor 接口扩展。未知来源、重复 Provider、非法 Schema、缺少风险元数据均在注册或刷新时 fail-closed。

AI Action 必须声明 `AIMeta.ExposeTool`，并使用 `action.SchemaFor[Request]()` 从同一个强类型 DTO 生成输入 Schema。Action Executor 只能调用 `ActionRegistry.Invoke`。

核心 `builtin:knowledge.search` 仍通过 Registry/Provider/Executor 注册，但只在当前 Agent 至少存在一个绑定、当前 actor 可读、enabled 且索引 ready 的知识库时投影。动态描述只能列出这些候选的名称、用途和稳定 ID，不得暴露 tenant、对象存储字段或无权资源。

## Search、Scope 与 Handle

搜索顺序为安全过滤、精确 ID/名称、关键词/FTS、可选 Embedding 融合。Embedding 不可用时必须退化；向量和搜索文档不是权限事实源。搜索、加载和执行均重新应用 tenant、realm、user permission、Scope 白名单和启用状态。

Handle 使用服务端签名并绑定 tenant、user、realm、session/auth version、scope、tool、runtime ref、schema hash 和 TTL。任何绑定变化、过期或 Schema 变化均拒绝。

## 确认与事实记录

写入、外部、付费、破坏性、高风险工具默认单次确认，critical 默认禁止。确认绑定不可变 arguments hash，仅展示递归脱敏摘要；批准后重新校验全部安全条件并单次消费，不提供“始终允许”。

工具调用事实与 `ai.tool_invocation.finished.v1` 同事务完成；目录变化写 `ai.tool_catalog.changed.v1`。Outbox、审计摘要和普通日志不得包含完整参数、结果、Secret、Credential 或第三方原始响应。

知识工具的 Invocation、Step、事件和 Outbox 只记录次数、候选/返回/Citation 数、耗时、降级、缓存命中与稳定失败分类；Chunk 正文只存在于发给模型的 Tool Result，不进入这些事实摘要。

## 扩展示例

新 Provider 实现 `Source/List/Get`，新 Executor 实现 `Execute`。Plugin、MCP 或 HTTP Executor 必须经 Runtime 的 Scope、Handle、权限、确认、幂等、审计、限流和结果脱敏，禁止自行向模型注册或直连执行。
