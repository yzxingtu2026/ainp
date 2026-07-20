# Agent Slot 与业务功能默认智能体

Agent Contract 定义智能体能力和安全约束，Agent Definition 是租户内可见的智能体身份，Agent Version 是不可变运行配置。Agent Slot 只定义业务功能需要哪类智能体，不复制 Contract，也不授予任何权限。

## 注册与解析

- Slot Definition 由模块代码注册，稳定标识为 `key + version`；数据库不得动态创建 builtin Slot。
- 首个内置 Slot 是 `workspace.default@1.0.0`，默认 Contract 为 `system.operations@1.0.0`。
- 解析优先级为：允许时的用户偏好、租户绑定、系统默认。只有某层从未配置时才读取下一层。
- 显式配置指向不可见、停用、未发布、不兼容或 diagnostics 异常的 Agent 时必须 fail-closed，并返回结构化 diagnostics，禁止静默回退。
- Binding 和用户偏好保存稳定 `agent_id`；解析时读取当前 release Version。

## 数据与安全边界

- `ai_agent_slot_bindings` 按 `tenant_id + slot_key` 唯一，所有更新使用 revision。
- `ai_agent_slot_preferences` 按 `tenant_id + user_id + slot_key` 唯一，只允许当前用户读写本人偏好。
- Agent 必须属于当前租户，并满足当前用户可见性、Slot Contract、enabled、已发布和 diagnostics 健康要求。
- 清除覆盖会删除对应记录并恢复下一级来源，不重复保存 builtin Agent ID。
- migration 只创建结构；新租户先 reconcile builtin Agent，再解析代码注册的系统默认。存量 reconcile 不覆盖租户绑定或用户偏好。

## Conversation 固定版本

业务页面不得先在前端解析 Version 再提交。创建新会话应调用服务端 `conversations/create-for-slot`，由同一请求解析 Slot 并把实际 `agent_id` 与 `agent_version_id` 固定到 Conversation。历史 Conversation 和 Run 始终读取已固定的 Version，不随 Binding 或 Agent 发布变化。

临时选择其他 Agent 只影响本次新会话；只有明确的“设为个人默认”操作才能写用户偏好。

## 扩展约定

新 AI 功能应在模块初始化时注册 Slot，并声明默认 Contract、能力要求、租户/用户覆盖策略、fail-closed 策略与升级策略。页面和业务服务禁止硬编码 Agent ID、builtin code 或自行实现解析优先级。

后续可按相同方式注册 `i18n.translate`、`agent.prompt_authoring`、`content.copywriting`；这些示例不代表对应业务智能体已经实现。
