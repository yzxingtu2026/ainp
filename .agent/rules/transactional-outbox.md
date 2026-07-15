# Transactional Outbox

- 仅在需要可靠异步、跨模块传播、外部副作用、失败重试或最终一致性时使用 Outbox；禁止为普通 CRUD 发 `record.updated` 等泛化事件。
- 核心一致性写入必须在当前事务同步完成；禁止用 Outbox 延迟订单/明细/库存等。
- 业务只依赖 `EventWriter` + 稳定 Envelope；禁止直写 outbox 表、调用 Dispatcher、依赖具体 Transport、同步双写 PG/Redis。
- Handler 必须幂等；不假设全局 FIFO；payload 禁止 GORM Model / 密钥 / 文件内容 / 完整请求响应。
- 租户上下文只来自 AuthContext / 受校验 EventContext；缺失 fail-closed。
- Outbox 不替代 Audit；长任务（转码/AI）只触发，执行交给任务队列。
- 新增事件须同步 Definition、schema version、Handler 幂等、DTO 与 `platform/tests/`。

详规：`docs/development/transactional-outbox.md`
