# Agent Slot

- AI 业务功能必须通过代码注册的 Agent Slot 解析默认 Agent，禁止页面硬编码 Agent ID、builtin code 或复制解析优先级。
- Slot 与 Agent Contract 分层：Contract 定义能力/安全约束，Slot 定义业务用途与覆盖策略。
- 解析顺序为允许时的用户偏好、租户绑定、系统默认；显式配置失效必须 fail-closed，不得静默回退。
- Slot 可声明 `PrincipalKinds` / `ClientSurfaces`，空值归一化为 `ainp.account` / `ainp.tenant-console`；`create-for-slot` 按当前访问面限制可解析 Slot，社区不得解析 `workspace.default`。
- Binding 保存 `agent_id`；新 Conversation 必须由服务端解析并固定实际 release `agent_version_id`，历史会话不得漂移。
- Slot Definition 不入库；migration 不写业务默认，reconcile 不覆盖租户或用户选择。
- capability 覆盖必须验证 Contract→builtin reconcile→registered Slot→Runtime Tool Scope 的真实可达性，Registry 并集不算可达。

详见 `docs/development/agent-slots.md`。
