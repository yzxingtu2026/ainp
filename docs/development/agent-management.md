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

指令固定按 Contract Base、Tenant、User、未来 Run 的顺序合成。Tenant/User 指令是补充内容，不能改写 Contract 安全边界。当前版本、Model Alias、Contract 必需模型能力和必需工具任何一项不可用时，保留配置并返回结构化 diagnostics；不得静默降级为健康。

## Setup 与 reconcile

数据库 migration 只建 schema。Setup 通过租户初始化 hook 幂等创建 `system.operations`；存量租户由显式 `ai.agent_builtin_reconcile` Action 同步。reconcile 只创建缺失实例或升级托管字段，不覆盖租户可编辑字段，不删除历史版本。

本层不实现 Conversation、Agent Run/Step、Workspace、知识库或长期记忆。这些运行对象后续只能引用明确的 `agent_version_id`。
