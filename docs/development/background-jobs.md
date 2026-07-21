# 后台任务（Redis + Asynq）

Agent 只读要点：`.agent/rules/background-jobs.md`。本文档为 Job 基础设施详规（Issue #21）。

## 概念边界

| 概念 | 含义 | 不做什么 |
|------|------|----------|
| **Event（Outbox）** | 已经发生的业务事实 | 不执行长任务 |
| **Job** | 需要执行的后台命令 | 不替代事务一致性 |
| **Schedule** | 何时触发 | 不直接跑长任务（留给后续 Scheduler） |

可靠业务任务路径：**业务事务写 Outbox → Outbox Handler 调 Enqueuer → Asynq → Worker**。禁止业务 GORM 事务内直接调 Redis/Asynq。

## 包结构

- `framework/job`：Contract（Definition/Registry/Enqueuer/Handler/Reporter/RunStore/Transport）
- `framework/asynqadapter`：Asynq Adapter（业务与 framework/job **不得** import Asynq 类型）
- `cmd/ainp-worker`：独立 Worker；与 HTTP 共用 `internal/boot`
- 业务：`modules/<m>/jobs/<type>.go`（一个 Job Type 一个文件）+ `Register`

## JobRun（PostgreSQL）

表 `job_runs` 是租户可见事实源；Asynq 消息只带 `job_run_id`。Worker 通过私有 trusted load 获取 JobRun 后，必须恢复 actor/session/auth version 的 AuthContext、重新加载授权，再获取 TenantDB 访问业务数据；不得把 Redis payload 或 Envelope tenant 直接当数据库 scope，也不得在 Handler 内回退根 DB。

pending enqueue、trusted load 和已 claim Run 状态更新属于 AuthContext 恢复前的精确基础设施例外；业务列表、详情、取消与领域资源访问仍必须走 TenantRaw/TenantDB。详见 [`tenant-data-access.md`](tenant-data-access.md)。

状态：`pending_enqueue` → `queued` → `running` → `succeeded` | `retrying` → `failed`/`dead` | `cancelled`。

`idempotency_key`：租户 + job_type 范围内受控唯一（Outbox 用 `event_id:job_type`）。

大型结果用 `result_ref`，不写大 JSON。

## 配置

```yaml
redis: { addr, db, password via env REDIS_PASSWORD }
jobs:
  enabled: false   # 默认关；无 Redis 可启 HTTP
  concurrency / queues / timeouts ...
```

- `jobs.enabled=false`：核心 HTTP 可启动；Job API 返回 `job.unavailable`
- `enabled=true` 且 Redis 不可用：Worker/组装失败（明确错误）
- `embedded_worker=true`（仅开发/单机）：在 `cmd/ainp` 内嵌启动 Asynq Worker，关停时一并优雅退出；生产保持 `false` 并独立跑 `cmd/ainp-worker`

## 运维 Action

`operations.job_list` / `job_get` / `job_cancel` / `job_retry`（取消与重试审计；不可改 Payload）。

## Demo

`POST /api/v1/demo/jobs/ping` → Outbox `demo.job_ping.v1` → 入队 `demo.ping` → Worker 进度 → `succeeded`。

附件缩略图（#36）：上传 `ready` → Outbox `attachment.ready.v1` → 入队 `attachment.generate_thumb`（`file` 队列）→ 写入派生对象；详见 `docs/development/attachments.md`。

工具语义索引：`ai.tool_catalog.changed.v1` → `ai.tool_catalog.embed`（`ai` 队列）→ 统一 Embedding Runtime → revision/provenance 条件写入 pgvector。重复事件使用 `tool_id + catalog_revision` 幂等键；旧 Job 不得覆盖新版 Search Document。

## 非目标

管理前端、完整 Scheduler UI、Workflow/DAG、Redis Cluster、具体转码/AI Job 实现、exactly-once。
