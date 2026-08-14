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

下游模块若只需让默认“系统运营助手”发现本模块 Action，不得注册没有 builtin Agent 与 Slot 消费路径的独立 Contract。模块应实现 `host.AgentContractContributionProvider`，显式向 `system.operations@1.0.0` 贡献 owner、`AllowedToolCapabilities`，以及可选的 owner-scoped Prompt key/version。Host 在 API、embedded worker 和独立 Worker 共用的装配路径中按 owner 排序聚合；重复 owner、未知 target、非法 capability、跨 owner Prompt、空指令或版本漂移均启动失败。Contribution 不授予 Permission、Credential，也不覆盖内置安全指令、Required Tool selectors、确认、审计或租户边界。

## ownership 与版本

- `builtin` 由 Contract 驱动，不能物理删除；框架字段只由 reconcile 升级。
- `tenant` 对租户共享，仅智能体管理员可创建、编辑、发布和启停。
- `personal` 默认 private，普通用户只能管理本人资源；提升为 tenant visibility 必须具有租户管理权限。
- 模型只保存稳定 Model Alias。保存、发布和启用时通过 `ModelResolver` 校验。
- 运行配置的每次修改都创建新 `AgentVersion`，历史版本禁止原地更新。未来 Agent Run 必须保存 `agent_version_id`，不得运行时追随 current version。

## 指令与诊断

有效指令由后端共享解析器确定性合成：Contract Base 在前，非空 Tenant 补充指令通过固定分隔段追加在后。Builder Preview 与正式 Workspace Run 必须调用同一解析器，禁止分别拼接。Tenant 指令是 Agent Version 上唯一可配置的补充人设/业务约束，不能改写 Contract 安全边界；personal/tenant Agent 没有 Contract 时只使用自身 Tenant 指令。Contract key/version 无法精确解析或 Base 为空时返回 `agent.contract_unavailable` 并 fail-closed。不得在日志、审计、diagnostics 或模型调用账本记录 Base、Tenant 或最终有效指令正文，只允许记录 Contract key/version 和是否存在补充指令等来源元数据。不提供 `user_instructions`：会话态任务交给 Workspace 对话，跨会话偏好交给记忆系统，禁止写入不可变 Version 快照。

## Setup 与 reconcile

数据库 migration 只建 schema。Setup 通过租户初始化 hook 幂等创建 `system.operations`；存量租户由显式 `ai.agent_builtin_reconcile` Action 同步。reconcile 只创建缺失实例或升级托管字段，不覆盖租户可编辑字段，不删除历史版本。由于内置 Contract 不绑定租户 Model Alias，新创建的 builtin 保持 `draft + current_version=0`；管理员必须先在 Builder 选择模型并发布首个 release，列表才允许启用。后端启用入口同样 fail-closed，并返回稳定的 `ai.agent.publish_required`，不得用不存在的 version 0 继续诊断或运行。

Contract contribution 不创建第二个 builtin Agent，也不写 migration 业务种子。存量 `system.operations` Definition 继续引用同一 Contract key/version；聚合结果对新 Run 创建的 Tool Scope 生效，已固定的历史 Conversation、Version 和已持久化 Run Scope 不回写、不漂移。

## Agent Builder

智能体编排工作台（Agent Builder）是面向创建者与租户管理员的配置/调试入口，路由为 `/ai/agent-builder`（列表 `/ai/agents` 负责入口跳转）。Builder 复用本层 Action：创建、元数据更新、版本发布、工具绑定与 diagnostics；`agent_get` 额外返回只读 Contract 覆盖字段：`base_instructions`、`managed_fields`、`required_tool_selectors`。存在 Contract Base 时，桌面和移动端均默认折叠展示“内置基础指令”，只读显示正文与 Contract key/version，并标记系统托管、运行时自动加载；“补充人设与回复逻辑”默认展开并绑定 `draft_tenant_instructions`。builtin 的“恢复内置默认”只把当前补充指令草稿置空并标记未保存，不写库、不发布，也不修改模型、工具、知识库、记忆或生成参数；正式行为仍需保存并发布新 Version 后才变化。personal/tenant Agent 不显示不存在的 Contract Base 或恢复按钮。工具绑定在中间栏展示“Contract 必选 + 用户可选”的有效绑定；Contract 必选项不复制为数据库事实且不可移除。

「保存基本信息」经 `ai.agent_update` 同时写入名称/描述与 Builder 草稿运行配置（`has_draft` + `draft_tenant_instructions` / `draft_model_alias` / `draft_generation_config` / `draft_tool_policy`），**不**创建 release Version、**不**晋升 `current_version`。Workspace 与正式 Run 仍只消费 current Version；草稿仅供 Builder 回填编辑器与预览物化。点「发布新版本」创建 `kind=release` 并清空草稿后，新配置才对正式会话生效。

发布候选配置不得被旧 `current_version` 的 diagnostics 反向阻断：首版缺失或旧模型失效正是需要发布修复的状态，候选 Model Alias 与输入由 `ai.agent_version_publish` 重新校验；启用仍必须检查发布后的完整 diagnostics。

动态路由模式下，Builder 通过后端 `HideInMenu` 菜单 `ai.agent_builder` 注册；前端同时把静态隐藏子路由 `ai_agent-builder` 合并进用户路由树，避免仅有列表菜单时 `router.push({ name: 'ai_agent-builder' })` 找不到匹配。

右栏实时预览走正规 Conversation + Agent Runtime，**不得**旁路 Generate 或追随未落库的编辑器状态：

1. `ai.agent_preview_materialize` 将当前编辑器字段（tenant_instructions / model_alias / generation_config / tool_policy）物化为不可变 `AgentVersion`，`kind=preview`，按 `content_hash` 复用，**不晋升** `agents.current_version`。
2. 预览会话强制 `purpose=builder_preview`，并钉死上述 preview（或等价）`agent_version_id`；配置变化产生新 preview version 时开启新调试会话。
3. 正式 `ai.agent_version_publish` 仍创建 `kind=release` 并更新 `current_version`；版本历史列表只返回 release。
4. Workspace（#80）默认只列 `purpose=workspace`，禁止把 `kind=preview` 用于 workspace 会话。
5. Builder 预览可对 `draft|enabled` agent 启动 Run；`disabled` 仍禁止。工具绑定挂在 `agent_id` 上，绑/解绑即时生效，无需为纯工具变更强制新 preview version。

## 记忆策略

Builder 可选择继承租户默认记忆策略或提交 Agent 覆盖。发布和 preview 物化时按 `Agent 输入 -> 租户 ai.memory_policy 设置 -> 系统默认` 解析，并把完整有效 `memory_policy` 与 `memory_policy_source` 冻结到不可变 Agent Version；旧版本不随设置变化。启用记忆时 extraction/consolidation Alias 都必须经统一 Resolver 验证 Structured Output、可用性和最小上下文，整理 Alias 为空时回退 extraction。Builder 选择器只展示当前租户已启用且声明 Structured Output 的 Alias。

本层不执行候选提取、写入、召回或向量化；运行对象只能引用明确的 `agent_version_id`。
