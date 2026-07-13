# 审计日志

- 运行日志 ≠ 审计；`meta.audit=true` 的 Action 由执行链统一写 Audit，禁止业务散落打点或用 Info 冒充。
- 最小集必填：`id`、`occurred_at`、`tenant_id`、`user_id`（可空）、`actor_type`、`action_id`、`result`、`request_id`。
- 禁止写入密钥、Token、大段 Body/Prompt；细节排障走 Logger。

详规：`docs/development/audit.md`
