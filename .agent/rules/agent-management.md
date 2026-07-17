# 智能体管理硬约束

- 内置能力必须注册 Agent Contract；数据库记录不得伪造或覆盖 Contract 安全字段。
- Agent 只引用 Model Alias；Tool Binding 只引用 namespaced Tool ID，绝不授予权限、Credential 或降低确认要求。
- Agent Version 创建后不可变；未来 Run 必须引用 `agent_version_id`。
- builtin 不可物理删除；Setup/reconcile 创建租户业务数据，migration 禁止 seed。
- personal 默认 private 且只能由 owner 管理；tenant/builtin 变更必须检查租户管理权限。
- 资源不可用时保留配置并返回 diagnostics，所有 tenant/owner 查询 fail-closed。
- Agent Builder 是配置/调试入口，不得旁路 Runtime；实时预览必须物化 `kind=preview` Version（不晋升 current）并走 `purpose=builder_preview` Conversation/Run。
- Builder「保存」把提示词等运行配置写入 Agent 草稿列（与元数据同事务），不晋升 current；正式发布 release 后清空草稿，Workspace 才消费新 Version。
- Agent Version 不提供 `user_instructions`；可配置补充指令只有租户指令，用户偏好交给 Workspace/记忆。
- 详细扩展方式见 `docs/development/agent-management.md`。
