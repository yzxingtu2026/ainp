# Page Context 协议

Page Context 描述用户发送消息时所在页面的受限语义。它是服务端重新解析的上下文与工具推荐信号，不是权限事实，不进入 canonical Conversation Message，也不替代 Action、ToolScope、AccessPolicy 或执行重鉴权。

## 数据流

```text
页面 Provider -> Envelope -> Run Start -> Resolver Registry
  -> tenant/user/permission/revision 校验 -> 不可变安全 Snapshot
  -> Prompt 临时注入 + Tool Search 候选排序
```

前端应用壳只调用 `capturePageContext()`；业务页面通过 `usePageContextProvider()` 注册类型化 Provider。禁止全局 AI 组件读取 DOM、组件实例或路由字符串推断业务语义。Provider 在 mounted/activated 注册，在 deactivated/unmount 注销；捕获时还必须校验 Provider 的 `route_name` 与当前激活路由一致，避免 keep-alive 或转场时序残留旧页面上下文。未注册页面返回空上下文并保持通用对话。

## Envelope 边界

- 当前 Schema 为 `1.0`，总大小最多 8 KiB。
- `page_type` 与 `route_name` 必须是稳定代码；mode 只允许 list/detail/create/edit。
- resource refs 最多 8 个，只包含 type/id/revision。
- UI state 最多 16 个字段，只允许短字符串、数字、布尔、null 或最多 8 项的一维标量数组。
- capability/tags/query hints 各最多 8 项；客户端 title、summary、refs 与 hints 全部不可信。
- 未知字段由 Action JSON decoder 拒绝；未知 Schema/page type、非法预算和嵌套对象 fail-closed。
- 禁止 DOM、隐藏字段、Secret、Credential、完整表单、未提交草稿、Prompt、模型响应和工具结果。

## Resolver

每个 page type 必须显式注册 Resolver。Resolver 从 AuthContext 取得 tenant/user/realm/permission，通过 TenantDB/Repository 重查资源并校验 revision，输出 `trusted_refs`、`safe_summary`、server-owned `tool_hints` 与稳定 `warnings`。

`pagecontext.Registry.PageTypes()` 提供稳定排序的后端 page type manifest，供下游 CI 与前端 Provider Registry 声明做集合校验；必须满足“前端声明 ⊆ 后端 Resolver”。该接口只枚举稳定类型代码，不暴露 Resolver 实例或运行时数据。Page Context 仍是可选能力，不要求每个页面注册 Provider。

资源不存在、跨租户、撤权或 revision 冲突统一返回 page context unavailable，不泄露资源是否存在。当前 Resolver 包括 `system.accounts`、`system.roles` 和 `operations.audit-logs`，覆盖 list/detail/create/edit。

## Run 与 Prompt

Run Start 只捕获一次 Envelope 并同步解析。安全 Snapshot 以 JSONB 固定在 Agent Run；恢复、重试、确认继续和同一 Run 的所有模型调用复用原快照。Prompt 使用独立 developer section，插在触发 user message 附近；Conversation Message 不被修改。

Run Event 只记录 status/page type/bytes。ContextSnapshot 只附加 schema、page type、mode 与 trusted refs，不复制 safe summary。日志、审计和 Telemetry 禁止记录页面正文。

## 工具推荐

Tool Runtime 先执行原有 Registry 投影、Agent Scope、当前 permission、realm、enabled 与 AccessPolicy 过滤，再用 Snapshot hints 对最多 10 个可见候选加权，最后按调用方 limit 截断。Hints 不修改 allowed sources/capabilities/tools，不自动 load，不改变 Handle/确认/execute 重鉴权。

## 新页面接入

1. 定义稳定 page type 和资源类型。
2. 在所属后端模块注册 Resolver，并声明权限、Tenant 查询、安全摘要和 server-owned hints。
3. 页面用 `usePageContextProvider()` 返回受限 Envelope；编辑/创建不得包含表单值。
4. 增加预算、权限、跨租户、revision、生命周期和无敏感字段测试。
5. 同步本文档与 `.agents/rules/page-context.md`。
