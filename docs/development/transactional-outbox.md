# Transactional Outbox

Agent 只读要点：`.agent/rules/transactional-outbox.md`。本文档为 PostgreSQL Transactional Outbox 详规（Issue #20）。

## 目标

业务数据变更与领域事件在**同一数据库事务**内提交；Dispatcher 可靠领取后经可替换 `EventTransport` 投递给 Handler；`event_deliveries` 提供 Handler 级幂等，形成 **at-least-once** 语义。

Phase 1 **不依赖 Redis**；架构预留 Redis Streams Transport，替换传输层时不改业务 EventWriter / Handler。

## 三层边界

| 组件 | 职责 | 业务可否依赖 |
|------|------|--------------|
| `OutboxWriter` | 在业务 GORM 事务中持久化 Envelope | ✅ 唯一生产入口 |
| `EventTransport` | 投递已提交事件（Phase 1：InProcess/postgres） | ❌ |
| `EventHandler` | 消费稳定 Envelope | ✅ 按 event type 注册 |
| `DeliveryStore` | `(event_id, handler_name)` 幂等收据 | Handler 间接使用 |
| `Dispatcher` | `SKIP LOCKED` 领取、退避、dead、优雅关闭 | ❌ |

业务 Service **只**依赖 `EventWriter` + 稳定 Envelope；禁止直接写 `outbox_events`、调用 Dispatcher、依赖具体 Transport 或同步双写 Redis。

## Envelope 契约

必填语义：`id`、`event_type`、`schema_version`、`tenant_id`、`realm`、`aggregate_*`、`occurred_at`、`payload`；可选 `correlation_id` / `causation_id` / `actor_id` / `metadata`。

- 事件类型稳定命名：`attachment.ready.v1`、`user.disabled.v1`。
- `schema_version` 是契约版本，不等于 DB 迁移版本；不兼容变更必须新版本。
- payload 必须是明确 DTO；禁止序列化 GORM Model、密码、Token、文件内容或完整 Request/Response。

## 写入与租户

- `outbox.WithinTx` / `Writer.Append`：业务数据与事件同事务；任一步失败同时回滚。
- 租户事件 fail-closed：`tenant_id` 只来自 `AuthContext` 或显式 `EventContext`（Job/系统）；禁止 Body/Query 指定。
- 无 HTTP 上下文时用 `outbox.NewTenantEventContext` / `WithEventContext`。

## 状态

- Outbox：`pending` → `processing` → `published` | `failed` →（超限）`dead`。
- Delivery：`pending` / `processing` / `succeeded` / `failed`；成功 Handler 不因其他 Handler 失败而重跑。

CodeSet：`system.outbox_status`、`system.delivery_status`。

## Dispatcher（postgres）

- `FOR UPDATE SKIP LOCKED` 批量领取；多实例安全。
- `lock_timeout` 后恢复卡死的 `processing`。
- 失败指数退避 + jitter；`max_attempts` 后 `dead`。
- 仅当 Transport `Publish` 成功后标记 `published`。
- 配置见 `configs/config.example.yaml` 的 `outbox:`；`enabled` 省略默认开启。

## 运维 Action（租户域）

| Action | 说明 |
|--------|------|
| `system.outbox_summary` | 状态计数与最老未完成时间 |
| `system.outbox_list_deliveries` | 查看某事件 deliveries |
| `system.outbox_retry` | 将 failed/dead 重置为 pending（**不可改 payload**）；审计 |

## Demo

`demo.note_create`：同事务写 `demo_notes` + `demo.note_created.v1`；两个 Handler 演示多 Handler 幂等。

## Redis Streams 迁移路径（本 Issue 不实现）

后续 Adapter 映射（文档契约，非代码）：

1. PostgreSQL Outbox 仍是事务事实源；Dispatcher 对 committed event 执行 `XADD`，成功后再推进 Outbox `published`。
2. Redis 不可用时事件保持 `pending`/`failed` 并重试；**不回滚**已提交业务事务，**不同步双写**。
3. Consumer group + `XREADGROUP` / `XACK`；PEL 认领恢复；dead-letter 与 Outbox `dead` 对齐策略由 Adapter 定义。
4. 消费侧继续用 PostgreSQL `DeliveryStore`（`event_id` + `handler_name`）做 Handler 幂等。
5. Transport 不得把 stream key / Redis message ID 写入业务 payload。
6. Fake/contract test（`RecordingTransport`）证明替换 Transport 无需改 Writer / Service / Handler。

## 非目标

- Redis/Kafka 等实际 Adapter、通用 Job Queue、Workflow、exactly-once、全局严格顺序、Outbox 管理前端、Event Sourcing、长时间 AI/转码执行器（Outbox 只负责可靠触发，长任务交给后续 #21 Asynq）。

## 与审计 / 任务队列边界

- Outbox ≠ Audit：事件驱动后续处理；审计追责走 Audit API。
- Outbox ≠ Job Queue：长任务由任务队列执行；Outbox 发布「需要做某事」的事实。

## 包位置

`platform/framework/outbox`；迁移 `migrations/0006_outbox.sql`；测试 `platform/tests/outbox_test.go`。
