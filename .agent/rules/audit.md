# 审计日志

- 运行日志 ≠ 审计；`meta.audit=true` 的 Action 由执行链统一写 Audit，禁止业务散落打点或用 Info 冒充。
- 最小集必填：`id`、`occurred_at`、`tenant_id`、`user_id`（可空）、`actor_type`、`action_id`、`result`、`request_id`。
- Agent 代表用户执行 Action 时保留责任 `user_id`，并记录 `actor_type/client=agent`、Run 固定 request/trace 与可信 `agent_run_id/tool_invocation_id`；关联字段禁止来自模型或工具参数。
- MCP 代表用户执行 Action 时记录 `actor_type/client=mcp` 与可信 `mcp_client_id/mcp_grant_id`；适配器预检拒绝和 Action 执行结果使用同一审计协议，参数、Token 与原始结果不得进入摘要。
- 禁止写入密钥、Token、大段 Body/Prompt；细节排障走 Logger。
- Service 可在请求 AuthContext 设置受控 resource_type/resource_id/summary，由 Action 执行链统一落库；禁止 Handler 自写审计表。
- 租户审计查询必须从 AuthContext 强制 tenant scope，跨租户详情返回不存在；默认 24 小时、最大 31 天、分页最多 100。
- 操作人筛选按当前租户用户名或用户昵称模糊查询，不使用用户 ID 作为查询条件。
- 审计查询本身 `audit=true` 且数据只读；普通高频 list/get/轮询默认 `audit=false`。用户不存在时仍保留原始 `user_id`。
- 日志来源、运行日志查询和详情属于安全敏感读取，必须 `audit=true`；运行日志正文不得复制进审计表。

详规：`docs/development/audit.md`
