---
applyTo: "platform/**/*.go,platform/**/*.sql"
---

# 后端审查路由

审查后端变更时，除仓库级指令外，按变更内容读取并应用：

- 目录与依赖边界：`.agents/rules/backend-layout.md`
- API 信封与错误码：`.agents/rules/api-response.md`
- 鉴权与租户上下文：`.agents/rules/auth-context.md`
- 审计与日志：`.agents/rules/audit.md`、`.agents/rules/logging.md`
- 状态与字符串常量：`.agents/rules/code-sets-and-status.md`
- 幂等、并发、锁和数据库不变量：`.agents/rules/idempotency-concurrency.md`
- Outbox 与后台任务：`.agents/rules/transactional-outbox.md`、`.agents/rules/background-jobs.md`

重点检查业务逻辑是否下沉到服务/领域/公共模块，是否绕过统一 Registry、Provider、Repository 或事务边界，以及运行时变化是否在 `platform/tests` 中具有相应测试。
