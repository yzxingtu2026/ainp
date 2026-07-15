# 幂等与并发控制 — Agent 要点

## 必守规则

1. **幂等声明**：写 Mutation Action 时，必须评估是否需要 `Idempotency`；涉及资金、创建、状态变更的 Action 应设为 `required` 或 `optional`。
2. **禁止 request_id 替代 idempotency_key**：幂等键由客户端生成并传入 `Idempotency-Key` header。
3. **乐观锁**：带并发更新风险的资源必须加 `revision` 字段，使用 `revision.ConditionalUpdate` 或 `revision.CheckUpdateResult`；禁止手写 `WHERE revision = ?` 散落各处。
4. **数据库锁**：保护业务不变量时使用 `dblock.LockForTenant` / `dblock.LockRowsForUpdate`；多行加锁必须先排序；锁内禁止 HTTP/文件/AI 调用。
5. **错误码**：幂等/并发相关错误使用 40001–40005 段，禁止自定义重复错误码。
6. **审计 replay**：replay 请求的审计记录 `Replay: true`，`Summary: "replay"`；禁止在 replay 路径重复执行业务逻辑。

## 前端

- 提交 mutation 请求前调用 `generateIdempotencyKey(actionId, payload)` 生成 key。
- 使用 `useIdempotentAction` composable 管理 key 生命周期。
- 编辑表单使用 `useRevisionForm` 管理 revision 值和冲突检测。

## 详细文档

→ `docs/development/idempotency-concurrency.md`
