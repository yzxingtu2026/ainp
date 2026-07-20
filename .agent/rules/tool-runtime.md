# Tool Runtime 硬约束

- 完整 ToolRegistry 只在服务端；模型固定工具为 `tool.search`、`tool.load`，核心工具仅在运行时资源条件满足时动态投影；禁止全量 Schema 或常驻 `tool.execute`。
- 模型调用通过 `enable_tools/tool_scope_id` 投影已加载真实 Schema，禁止客户端直接提交 `tools` 绕过 Registry。
- Tool ID 必须 namespaced；Provider/Executor 统一注册，未知来源、重复、非法 Schema 与缺失风险元数据 fail-closed。
- 核心工具必须实现独立 `RuntimeSlot` 并经 `platform/modules/ai/tools/register.go` 显式聚合；禁止扫描源码、Go Plugin、隐藏 `init()`，禁止在 ToolService/Agent Runtime 增加业务 Tool ID 分支。
- Action 是否暴露以注册处的 `Metadata.AI` 为唯一事实源；仅 `AIMeta.ExposeTool=true` 自动投影，`AI=nil` 默认不暴露。输入/输出 Schema 必须由同一强类型 DTO 生成，Action Tool 只经 `ActionRegistry.Invoke`，禁止维护平行暴露清单。
- Scope 精确 Tool 与来源/能力白名单按“精确命中或来源+能力命中”授权；来源/能力只开放渐进 search/load，禁止未 load Schema 进入模型初始投影。
- Catalog 在模块注册完成、新租户初始化或显式 rebuild 时幂等投影；请求路径禁止写 Catalog。变更以单调 revision 同事务写 Outbox，移除/停用立即 stale，重复/乱序事件不得覆盖当前 Search Document。
- search/load/execute 每次重新应用 tenant、realm、permission、Scope 与启用状态；Embedding/索引绝不是权限事实源。
- project/search/load/execute 共用 Slot 动态可用性协议且只能收紧权限；Step/Invocation 分类、专用预算和事实摘要由 Slot 元数据生成，观测只持久化白名单字段。
- Handle 绑定 actor/session/auth version/scope/tool/runtime/schema hash/TTL；任何变化立即失效。
- 写/外部/付费/破坏性/high 必须单次确认，critical 默认禁止；批准后重新鉴权，不支持始终允许。
- Invocation 终态与 Outbox 同事务，日志/Outbox/审计禁止完整参数、结果、Secret 和原始第三方响应。
- `knowledge.search` 必须经 Registry/Scope/Executor，且仅在存在当前可读、enabled、ready 的 Agent 绑定库时投影；知识正文不得进入 Invocation/Step/事件事实摘要。
- 测试统一放在 `platform/tests`。详见 `docs/development/tool-runtime.md`。
- 新增 Action Tool 或 Provider/Executor 时同时遵循 `docs/development/ai-tool-development.md`。
