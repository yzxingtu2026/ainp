# 后台任务 Job

- Job ≠ Event ≠ Schedule：可靠任务经 Outbox 入队；禁止业务事务直连 Redis/Asynq。
- 业务只依赖 `framework/job`；禁止 import Asynq/Redis Client；一个 Job Type 一个 Go 文件（`modules/*/jobs/`）。
- Handler 必须幂等、响应 `ctx.Done()`；错误用 `Retryable`/`Permanent`/`Cancelled`；禁止自建 sleep/retry 循环。
- 可分类错误统一携带安全 category/stage/summary，并在 JobRun 与结构化日志中保持一致；重跑和成功时清空旧分类，禁止持久化底层敏感错误。
- Job 补偿扫描必须基于持久化的明确可恢复状态；禁止按错误字符串分类或无差别重放历史/permanent 失败。
- Worker 以 PostgreSQL JobRun 为可信源，不信任 Redis 中的 tenant/payload 篡改；加载可信事实后恢复 AuthContext、重新鉴权并获取 TenantDB，禁止回退根 DB。
- 开发可用 `jobs.embedded_worker=true` 内嵌 Worker；生产独立部署 `ainp-worker`。
- 稳定队列仅 `critical/default/file/notification/ai`；Agent Run 固定进 `ai`。独立 Worker 可用进程级 queues/concurrency 覆盖，必须 fail-fast 校验且不得修改共享配置；未覆盖时本地/embedded 继续消费全部配置队列。
- 安装向导可选启用 Job；启用前及初始化时必须探测 Redis。非密钥意图写 `state.yaml`，Redis 凭据只走环境变量，且环境变量优先。
- 大结果用 `result_ref`；Progress 经 Reporter；取消为协作式。
- 变更同步 `docs/development/background-jobs.md` 并 `agent-rules sync`。

详规：`docs/development/background-jobs.md`
