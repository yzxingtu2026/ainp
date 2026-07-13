# 后端日志

- 运行日志只用框架统一 Logger（基于 `log/slog`）；禁止 `print`/`fmt.Println` 与业务内自建日志库。
- **运行日志 ≠ 审计**：排障走 Logger；「谁对何 Action 做了什么」走 Audit API（见详规）。
- 经 `context` 传递并自动带上：`request_id`/`trace_id`、`tenant_id`、`user_id`、`action_id`/`capability`（在 Action 内）等约定字段。
- 禁止输出密钥、Token、证件号等敏感信息；AI 载荷按详规脱敏/截断。

详规：`docs/development/logging.md`
