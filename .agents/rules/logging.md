# 后端日志

- 运行日志只用框架统一 Logger（基于 `log/slog`）；禁止 `print`/`fmt.Println` 与业务内自建日志库。
- **运行日志 ≠ 审计**：排障走 Logger；「谁对何 Action 做了什么」走 Audit API（见详规）。
- 经 `context` 传递并自动带上：`request_id`/`trace_id`、`tenant_id`、`user_id`、`action_id`/`capability`（在 Action 内）等约定字段。
- 生产保留 stdout，并可组合并发安全轮转 JSONL 文件 Sink；API/Worker 分文件，必须带 `service`/`instance_id`，文件故障只降级不得阻断业务。
- 内部错误使用 `framework/diagnostic` 的稳定 category/stage/safe attributes 并保留 error chain；公共最终边界记录 Error，业务层只包装诊断，禁止逐层重复输出。
- 禁止输出密钥、Token、证件号等敏感信息；AI 载荷按详规脱敏/截断。
- 日志中心只经 `framework/logcenter` 只读 Provider 查询外部日志；业务库不存运行日志，业务模块禁止 Provider 分支或透传原生 DSL。
- 当前不注册日志中心 Action、HTTP API、权限或菜单；仅保留底层协议与页面骨架，恢复必须另开 Issue。
- tenant 查询默认 15 分钟、最大 24 小时、分页 100、超时 5 秒、结果 1 MiB；统一白名单筛选与脱敏/截断。
- 只有声明可信 tenant filter 的 Provider 可用于 tenant realm；否则 fail-closed。平台/数据库原始日志不得降级开放给租户。

详规：`docs/development/logging.md`
