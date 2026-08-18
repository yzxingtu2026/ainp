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
- `cmd/ainp-worker`：独立 Worker；与 HTTP 共用公开 `host` Composition
- 业务：`modules/<m>/jobs/<type>.go`（一个 Job Type 一个文件）+ `Register`

## JobRun（PostgreSQL）

表 `job_runs` 是租户可见事实源；Asynq 消息只带 `job_run_id`。Worker 通过私有 trusted load 获取 JobRun 后，必须恢复 actor/session/auth version 的 AuthContext、重新加载授权，再获取 TenantDB 访问业务数据；不得把 Redis payload 或 Envelope tenant 直接当数据库 scope，也不得在 Handler 内回退根 DB。

可分类失败必须通过统一 Job classified error 携带受控 `error_category/error_stage/safe_summary`。Worker 在 `retrying/dead/cancelled` 时将三者一致写入 JobRun 和结构化日志，重新运行与成功时清空旧分类。分类错误的公开字符串只能包含安全摘要，底层 cause 仅供进程内判定，禁止持久化 provider body、Secret、完整输入或堆栈。

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
- `enabled=true` 且 Redis 不可用或不兼容当前 Asynq：安装 fail-closed，Worker/组装不得带病启用。`PING` 只证明连接可达，不证明队列兼容。
- `embedded_worker=true`（仅开发/单机）：在 `cmd/ainp` 内嵌启动 Asynq Worker，关停时一并优雅退出；生产保持 `false` 并独立跑 `cmd/ainp-worker`
- 稳定资源队列集合固定为 `critical/default/file/notification/ai`；`ai.agent_run.execute` 进入 `ai`。禁止按 tenant、Agent 或 Job Type 动态创建队列。
- 独立 `ainp-worker` 可用 `WORKER_QUEUES=queue=weight,...` 和 `WORKER_CONCURRENCY` 覆盖当前进程；覆盖优先于共享 `jobs.queues/concurrency`，不写回 config/state，也不影响 embedded Worker 或其他进程。空集合、未知/重复队列、非正权重和非正并发启动即失败。
- 未设置覆盖时，独立与 embedded Worker 均保持共享配置的一进程全队列模式。本地无需第二个 Worker；生产/Demo 可用同一二进制部署互不重叠的通用 Worker 与 AI Worker。
- Worker 启动日志必须输出最终 queues、weights、concurrency、service、instance_id；Job 开始日志输出 `job_queue_latency_ms=started-(queued/retrying updated_at)`，高基数关联字段不得作为指标 label。

## 首次安装

安装向导的“后台任务”步骤可选择是否启用 Job：

1. 保持关闭时无需 Redis，HTTP、登录等核心能力可正常使用；附件缩略图等异步能力暂不可用。
2. 启用时填写 Redis `addr` / `db` 并先执行统一队列兼容性探测；初始化会再次执行同一探测，不可达、认证失败、Lua/EVAL 不可用或核心命令不兼容时均 fail-closed，不会静默写入启用状态。
3. Redis 密码不由浏览器提交，也不写入 `state.yaml`；使用 `REDIS_PASSWORD`（可选 `REDIS_USERNAME`）环境变量注入。
4. 完成安装后重启 HTTP 服务加载新配置。生产环境另行启动 `ainp-worker`；开发/单机可在向导中选择 `embedded_worker`，由 HTTP 进程内嵌 Worker。

向导生成的 DSN、Redis 地址/DB、Job 开关统一写入权限为 `0600` 的 `configs/state.yaml`，不回写只读的 `config.yaml`。启动时先加载主配置与环境变量，再合并安装状态；`DATABASE_DSN` / `REDIS_ADDR` / Redis 凭据等部署环境变量保持更高优先级，避免形成双份配置真相。

### Redis/Asynq 兼容性探测

`framework/asynqadapter.ProbeCompatibility` 是 Setup 的唯一 Redis 队列探测入口，必须与当前 Asynq Adapter 真实依赖同步。当前 Asynq v0.25.1 的入队、出队、完成、重试与 Inspector 广泛使用 Redis Script，因此探测使用与 Asynq 相同的 `redis.Script.Run` 路径验证 `EVALSHA → NOSCRIPT → EVAL`，并在隔离前缀临时 key 上验证 String、Hash、List、Set、Sorted Set 后清理；取消通知另以唯一临时 channel 验证 Pub/Sub 订阅握手与发布。无 key Lua 探测、隔离 key 和临时 channel 均不得读取或修改现有 `asynq:*` 业务状态。

稳定结果区分 `redis_connection_failed`、`redis_authentication_failed`、`redis_lua_unavailable` 与 `redis_queue_incompatible`，同时返回逐项 capability；API、日志和状态文件不得暴露 Redis 原始错误、密码、用户名或完整敏感配置。升级 Asynq 或改变 Adapter 命令路径时，必须同步审查并更新探测与正负测试，避免自定义探测通过而实际运行失败。

## 运维 Action

`operations.job_list` / `job_get` / `job_cancel` / `job_retry`（取消与重试审计；不可改 Payload）。

## Demo

`POST /api/v1/demo/jobs/ping` → Outbox `demo.job_ping.v1` → 入队 `demo.ping` → Worker 进度 → `succeeded`。

附件缩略图（#36/#200）：上传 `ready` → Outbox `attachment.ready.v1` → 入队 `attachment.generate_thumb`（`file` 队列）→ 写入派生对象。确定性输入拒绝必须 `Permanent`，基础设施故障 `Retryable`，取消/超时 `Cancelled`；补偿只重放明确可恢复状态。详见 `docs/development/attachments.md`。

工具语义索引：`ai.tool_catalog.changed.v1` → `ai.tool_catalog.embed`（`ai` 队列）→ 统一 Embedding Runtime → revision/provenance 条件写入 pgvector。重复事件使用 `tool_id + catalog_revision` 幂等键；旧 Job 不得覆盖新版 Search Document。

## 非目标

管理前端、完整 Scheduler UI、Workflow/DAG、Redis Cluster、具体转码/AI Job 实现、exactly-once。
