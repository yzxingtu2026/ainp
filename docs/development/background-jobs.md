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

表 `job_runs` 是租户可见事实源；Asynq 消息只带 `job_run_id`，Worker 从 PG 加载可信租户与 Payload。

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

## 运维 Action

`system.job_list` / `job_get` / `job_cancel` / `job_retry`（取消与重试审计；不可改 Payload）。

## Demo

`POST /api/v1/demo/jobs/ping` → Outbox `demo.job_ping.v1` → 入队 `demo.ping` → Worker 进度 → `succeeded`。

## 非目标

管理前端、完整 Scheduler UI、Workflow/DAG、Redis Cluster、具体转码/AI Job 实现、exactly-once。
