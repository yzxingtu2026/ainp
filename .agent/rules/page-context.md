# Page Context

- Page Context 仅是服务端重解析的上下文与工具推荐信号，不是权限事实，不写 canonical Message，不替代 ToolScope/Action/AccessPolicy/执行重鉴权。
- 前端业务页只经统一 Provider Registry 注册；全局 AI 只捕获一次快照，禁止 DOM/组件实例/路由字符串猜测，禁止上传完整表单、未提交草稿、Secret 或 Credential。
- Envelope 必须版本化并限制总字节、refs、UI state、hints 与嵌套类型；未知 Schema/page type/字段、非法预算 fail-closed。
- 每个 page type 必须显式注册 Resolver，从 AuthContext + TenantDB/Repository 重查资源、权限与 revision；客户端 title/summary/capability/tags 均不可信。
- Run 固定安全 Snapshot，恢复/重试复用；Prompt 只临时注入触发 user message 附近，事件/日志/审计只保留安全状态与 refs。
- Tool hints 只对完成 Registry/Scope/permission/realm/enabled 过滤的候选排序，禁止扩大 allowed scope、自动 load 或绕过确认。

详规：`docs/development/page-context.md`
