# 智能体管理硬约束

- 内置能力必须注册 Agent Contract；数据库记录不得伪造或覆盖 Contract 安全字段。
- Agent 只引用 Model Alias；Tool Binding 只引用 namespaced Tool ID，绝不授予权限、Credential 或降低确认要求。
- Tool Definition 显式声明 `binding_mode` / `loading_mode`；Action 固定不可绑定且只经 Contract Scope 渐进发现，Meta/Runtime Slot 可按声明绑定。Contract 必选项作为不可移除的有效绑定但不复制数据库事实；候选、写入、列表与 Run Scope 共用 Registry 门禁。
- Agent Version 创建后不可变；未来 Run 必须引用 `agent_version_id`。
- builtin 不可物理删除；Setup/reconcile 创建租户业务数据，migration 禁止 seed。
- personal 默认 private 且只能由 owner 管理；tenant/builtin 变更必须检查租户管理权限。
- 资源不可用时保留配置并返回 diagnostics，所有 tenant/owner 查询 fail-closed。
- Agent Builder 是配置/调试入口，不得旁路 Runtime；实时预览必须物化 `kind=preview` Version（不晋升 current）并走 `purpose=builder_preview` Conversation/Run。
- Builder「保存」把提示词等运行配置写入 Agent 草稿列（与元数据同事务），不晋升 current；正式发布 release 后清空草稿，Workspace 才消费新 Version。
- Agent Version 不提供 `user_instructions`；可配置补充指令只有租户指令，用户偏好交给 Workspace/记忆。
- Contract Base 与租户补充指令必须经共享解析器按 Base 在前的顺序合成；Preview/正式 Run 共用，Contract 缺失或错配 fail-closed，正文不得进入日志、审计、diagnostics 或调用账本。
- Builder 只读展示 Contract Base；“恢复内置默认”只能清空当前补充指令草稿，保存并发布后才影响正式 Run。
- Memory Policy 在发布/preview 物化时按 Agent 输入→租户设置→系统默认解析并冻结到 Version；启用时模型 Alias 必须通过 Structured Output diagnostics，旧 Version 禁止随设置漂移。
- 详细扩展方式见 `docs/development/agent-management.md`。
