# Tool Runtime 硬约束

- 完整 ToolRegistry 只在服务端；模型常驻工具仅 `tool.search`、`tool.load`，禁止全量 Schema 或常驻 `tool.execute`。
- 模型调用通过 `enable_tools/tool_scope_id` 投影已加载真实 Schema，禁止客户端直接提交 `tools` 绕过 Registry。
- Tool ID 必须 namespaced；Provider/Executor 统一注册，未知来源、重复、非法 Schema 与缺失风险元数据 fail-closed。
- AI Action 的 Schema 必须由同一强类型 DTO 生成；Action Tool 只经 `ActionRegistry.Invoke`。
- search/load/execute 每次重新应用 tenant、realm、permission、Scope 与启用状态；Embedding/索引绝不是权限事实源。
- Handle 绑定 actor/session/auth version/scope/tool/runtime/schema hash/TTL；任何变化立即失效。
- 写/外部/付费/破坏性/high 必须单次确认，critical 默认禁止；批准后重新鉴权，不支持始终允许。
- Invocation 终态与 Outbox 同事务，日志/Outbox/审计禁止完整参数、结果、Secret 和原始第三方响应。
- 测试统一放在 `platform/tests`。详见 `docs/development/tool-runtime.md`。
