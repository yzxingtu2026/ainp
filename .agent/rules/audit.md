# 审计日志

- 运行日志 ≠ 审计；`meta.audit=true` 的 Action 由执行链统一写 Audit，禁止业务散落打点或用 Info 冒充。
- 最小集必填：`id`、`occurred_at`、`tenant_id`、`user_id`（可空）、`actor_type`、`action_id`、`result`、`request_id`。
- 禁止写入密钥、Token、大段 Body/Prompt；细节排障走 Logger。
- Service 可在请求 AuthContext 设置受控 resource_type/resource_id/summary，由 Action 执行链统一落库；禁止 Handler 自写审计表。
- 租户审计查询必须从 AuthContext 强制 tenant scope，跨租户详情返回不存在；默认 24 小时、最大 31 天、分页最多 100。
- 审计查询本身 `audit=true` 且数据只读；普通高频 list/get/轮询默认 `audit=false`。用户不存在时仍保留原始 `user_id`。

详规：`docs/development/audit.md`
