# API 响应与错误

- 统一信封：`code`、`message`、`message_key`、`data`、`request_id`；禁止业务自行拼响应 JSON。
- `message` 由服务端按 `message_key` + `Accept-Language` 本地化填充；`message_key` 为稳定机器键（`domain.reason` 小写点分）。
- `code == 0` 成功；业务可预期失败用 **HTTP 200 + `code != 0`**；401/403/429/5xx 等协议/系统问题用对应 HTTP。
- 资源不存在用业务码（如 `10004`）+ HTTP 200，不用裸 404 表示领域资源缺失。
- Handler 经框架 `AppError` / `response.OK|Fail` 出口；错误码按分段常量，禁止魔法数。
- 新业务错误使用 `errs.NewKey(code, key, fallback)`；旧 `errs.New` 自动映射默认 key。

详规：`docs/development/api-response.md`
