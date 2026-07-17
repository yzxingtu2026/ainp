# 智能体管理与扩展

AINP 的智能体管理分为三个不可混用的层次：

```text
Agent Contract（代码注册的能力与安全契约）
  -> Agent Definition（租户内身份、ownership、状态）
  -> Agent Version（不可变运行配置快照）
```

## 新增业务智能体

业务模块通过 `aiagent.Registry.Register` 注册稳定的 Contract。Contract 必须声明稳定 key、语义化版本、框架基础指令、模型能力、namespaced Tool ID、配置 JSON Schema、托管字段和升级策略。禁止用数据库记录冒充 builtin Contract，也禁止让租户内容覆盖鉴权、租户隔离、确认和数据安全指令。

新增工具仍按 `docs/development/tool-runtime.md` 注册 Provider/Definition/Executor。Agent Tool Binding 只保存 Tool ID 和选择策略，不复制 Schema，不授予 permission、Credential 或更低风险等级。运行时必须重新计算当前用户可见工具并执行确认策略。

## ownership 与版本

- `builtin` 由 Contract 驱动，不能物理删除；框架字段只由 reconcile 升级。
- `tenant` 对租户共享，仅智能体管理员可创建、编辑、发布和启停。
- `personal` 默认 private，普通用户只能管理本人资源；提升为 tenant visibility 必须具有租户管理权限。
- 模型只保存稳定 Model Alias。保存、发布和启用时通过 `ModelResolver` 校验。
- 运行配置的每次修改都创建新 `AgentVersion`，历史版本禁止原地更新。未来 Agent Run 必须保存 `agent_version_id`，不得运行时追随 current version。

## 指令与诊断

指令固定按 Contract Base、Tenant、未来 Run 的顺序合成。Tenant 指令是 Agent Version 上唯一可配置的补充人设/业务约束，不能改写 Contract 安全边界。不提供 `user_instructions`：会话态任务交给 Workspace 对话，跨会话偏好交给记忆系统，禁止写入不可变 Version 快照。当前版本、Model Alias、Contract 必需模型能力和必需工具任何一项不可用时，保留配置并返回结构化 diagnostics；不得静默降级为健康。

## Setup 与 reconcile

数据库 migration 只建 schema。Setup 通过租户初始化 hook 幂等创建 `system.operations`；存量租户由显式 `ai.agent_builtin_reconcile` Action 同步。reconcile 只创建缺失实例或升级托管字段，不覆盖租户可编辑字段，不删除历史版本。

## Agent Builder

智能体编排工作台（Agent Builder）是面向创建者与租户管理员的配置/调试入口，路由为 `/ai/agent-builder`（列表 `/ai/agents` 负责入口跳转）。Builder 复用本层 Action：创建、元数据更新、版本发布、工具绑定与 diagnostics；`agent_get` 额外返回只读 Contract 覆盖字段：`base_instructions`、`managed_fields`、`required_tool_selectors`。编排页左栏整栏为租户指令（人设提示词）编辑区，使用 Markdown 语法高亮编辑器（CodeMirror）；**不展示** Contract `base_instructions`（由代码契约托管，运行时自动合成），也不提供用户指令或 Run 指令字段。工具绑定在中间栏仅展示已绑定摘要；搜索/浏览已注册工具并选择绑定通过独立弹窗（`AgentToolPickerModal`）完成，管理端 Browse 上限 100，与模型侧 `tool.search` 硬顶 10 分离。

「保存基本信息」经 `ai.agent_update` 同时写入名称/描述与 Builder 草稿运行配置（`has_draft` + `draft_tenant_instructions` / `draft_model_alias` / `draft_generation_config` / `draft_tool_policy`），**不**创建 release Version、**不**晋升 `current_version`。Workspace 与正式 Run 仍只消费 current Version；草稿仅供 Builder 回填编辑器与预览物化。点「发布新版本」创建 `kind=release` 并清空草稿后，新配置才对正式会话生效。

动态路由模式下，Builder 通过后端 `HideInMenu` 菜单 `ai.agent_builder` 注册；前端同时把静态隐藏子路由 `ai_agent-builder` 合并进用户路由树，避免仅有列表菜单时 `router.push({ name: 'ai_agent-builder' })` 找不到匹配。

右栏实时预览走正规 Conversation + Agent Runtime，**不得**旁路 Generate 或追随未落库的编辑器状态：

1. `ai.agent_preview_materialize` 将当前编辑器字段（tenant_instructions / model_alias / generation_config / tool_policy）物化为不可变 `AgentVersion`，`kind=preview`，按 `content_hash` 复用，**不晋升** `agents.current_version`。
2. 预览会话强制 `purpose=builder_preview`，并钉死上述 preview（或等价）`agent_version_id`；配置变化产生新 preview version 时开启新调试会话。
3. 正式 `ai.agent_version_publish` 仍创建 `kind=release` 并更新 `current_version`；版本历史列表只返回 release。
4. Workspace（#80）默认只列 `purpose=workspace`，禁止把 `kind=preview` 用于 workspace 会话。
5. Builder 预览可对 `draft|enabled` agent 启动 Run；`disabled` 仍禁止。工具绑定挂在 `agent_id` 上，绑/解绑即时生效，无需为纯工具变更强制新 preview version。

本层不实现 Workspace 产品页、知识库或长期记忆；运行对象只能引用明确的 `agent_version_id`。
