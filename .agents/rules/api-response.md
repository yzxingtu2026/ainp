# API 响应与多语言

- 统一信封：`code`、`message`、`message_key`、`data`、`request_id`；禁止业务自行拼响应 JSON。
- `message` 由服务端按 `message_key` + `Accept-Language` 本地化填充；`message_key` 为稳定机器键（`domain.reason` 小写点分）。
- 翻译 YAML 由 `embed.FS` 随版本发布；正式源不可 reload，仅显式 `-dev-runtime-assets` 受信开发模式可调用 i18n reload。
- 所有用户可见字符串（Capability/Action/Menu/业务响应）均走 i18n key，禁止硬编码中文。
- `code == 0` 成功；业务可预期失败用 **HTTP 200 + `code != 0`**；401/403/429/5xx 等协议/系统问题用对应 HTTP。
- 资源不存在用业务码（如 `10004`）+ HTTP 200，不用裸 404 表示领域资源缺失。
- Handler 经框架 `AppError` / `response.OK|Fail` 出口；错误码按分段常量，禁止魔法数。
- 请求验证错误必须是 `CodeValidation` + `common.validation_failed` + `AppError.Fields`；字段名使用 JSON 路径，字段文案使用稳定 `validation.*` key。
- 分页请求固定 `page/page_size`，成功载荷由 `pagination.Result[T]` 输出 `items/total/page/page_size`；禁止 `records/current/size` 和手写分页 map。
- 新业务错误使用 `errs.NewKey(code, key, fallback)`；业务 handler 用 `i18n.FromCtx(ctx)` + `i18n.T(lang, key)`。

详规：`docs/development/api-response.md`
