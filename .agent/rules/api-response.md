# API 响应与错误

- 统一信封：`code`、`message`、`data`、`request_id`；禁止业务自行拼响应 JSON。
- `code == 0` 成功；业务可预期失败用 **HTTP 200 + `code != 0`**；401/403/429/5xx 等协议/系统问题用对应 HTTP。
- 资源不存在用业务码（如 `10004`）+ HTTP 200，不用裸 404 表示领域资源缺失。
- Handler 经框架 `AppError` / `response.OK|Fail` 出口；错误码按分段常量，禁止魔法数。

详规：`docs/development/api-response.md`
