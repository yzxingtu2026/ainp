# 后台任务 Job

- Job ≠ Event ≠ Schedule：可靠任务经 Outbox 入队；禁止业务事务直连 Redis/Asynq。
- 业务只依赖 `framework/job`；禁止 import Asynq/Redis Client；一个 Job Type 一个 Go 文件（`modules/*/jobs/`）。
- Handler 必须幂等、响应 `ctx.Done()`；错误用 `Retryable`/`Permanent`/`Cancelled`；禁止自建 sleep/retry 循环。
- Worker 以 PostgreSQL JobRun 为可信源，不信任 Redis 中的 tenant/payload 篡改。
- 大结果用 `result_ref`；Progress 经 Reporter；取消为协作式。
- 变更同步 `docs/development/background-jobs.md` 并 `agent-rules sync`。

详规：`docs/development/background-jobs.md`
