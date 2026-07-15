# 幂等与并发控制基础设施

本文档描述 AINP 平台层统一的幂等（Idempotency）、乐观锁（Revision）和数据库不变量锁（dblock）机制。

## 1. Action 幂等

### 1.1 声明方式

在 Action `Metadata` 中设置 `Idempotency *IdempotencyMeta`：

```go
action.Metadata{
    Idempotency: &action.IdempotencyMeta{
        Mode:                "required",       // "none" | "optional" | "required"
        TTL:                 1 * time.Hour,    // 幂等记录保留时间（默认 24h）
        InProgressTimeout:   30 * time.Second, // processing 超时（默认 30s）
        CacheBusinessErrors: false,            // 是否缓存业务错误
    },
}
```

- **none**：不做幂等（默认）；禁止设置 TTL / InProgressTimeout。
- **optional**：客户端提供 `Idempotency-Key` 时启用幂等；未提供时直接执行。
- **required**：客户端必须提供 `Idempotency-Key`，缺失时返回 40001 错误。

### 1.2 HTTP Header

客户端通过 `Idempotency-Key` HTTP header 传递幂等键。中间件自动提取并写入 `auth.Context.IdempotencyKey`。

### 1.3 执行流程

1. 从 AuthContext 取 `tenant_id`、`user_id`、`IdempotencyKey`。
2. 计算 `request_hash = SHA-256(input)`（JSON 键稳定排序）。
3. 查 `idempotency_records`：
   - **不存在** → INSERT processing → 执行 Handler → Complete。
   - **completed + hash 匹配** → replay（返回缓存响应，审计标记 `replay: true`）。
   - **completed + hash 不匹配** → conflict (40002)。
   - **processing + 未超时** → in_progress (40003)。
   - **processing + 已超时** → 尝试抢占（ClaimProcessing）。
   - **failed + CacheBusinessErrors + hash 匹配** → replay 缓存错误。
4. 响应缓存大小限制 64KB，超限不缓存。

### 1.4 幂等键唯一维度

`tenant_id + user_id + action_id + idempotency_key`

### 1.5 错误码

| Code  | Key                       | 场景                    |
|-------|---------------------------|-------------------------|
| 40001 | `idempotency.key_required`| mode=required 但未传 key |
| 40002 | `idempotency.conflict`    | 同 key 不同请求          |
| 40003 | `idempotency.in_progress` | 操作正在处理中           |

### 1.6 审计

- replay 请求写审计记录，`replay: true`，`summary: "replay"`。
- replay 不重复触发业务逻辑。

## 2. 乐观锁（Revision）

### 2.1 公共协议

`platform/framework/revision` 包提供：

- `ErrRevisionConflict`：sentinel error，版本冲突。
- `Versioned` 接口：`GetRevision() int64`。
- `ConditionalUpdate(db, table, pk, expectedRevision, updates)`：原子 UPDATE ... WHERE revision = expected。
- `WithExpectedRevision(expected)`：GORM scope helper。
- `CheckUpdateResult(result)`：检查 GORM update 结果是否为版本冲突。

### 2.2 使用方式

```go
// 方式一：ConditionalUpdate
err := revision.ConditionalUpdate(db, "my_table", docID, expectedRev, map[string]any{
    "title": "new title",
})
if errors.Is(err, revision.ErrRevisionConflict) {
    // 提示用户刷新
}

// 方式二：GORM Scope
result := db.Model(&MyModel{}).
    Where("id = ?", id).
    Scopes(revision.WithExpectedRevision(expected)).
    Updates(map[string]any{"title": "new", "revision": gorm.Expr("revision + 1")})
if err := revision.CheckUpdateResult(result); err != nil { ... }
```

### 2.3 错误码

| Code  | Key                      | 场景         |
|-------|--------------------------|--------------|
| 40004 | `common.revision_conflict` | 版本冲突     |

## 3. 数据库不变量锁（dblock）

### 3.1 API

`platform/framework/dblock` 包提供：

- `WithinTx(ctx, db, fn)`：事务包装，错误自动回滚。
- `LockForTenant(tx, table, tenantID, pk)`：单行 SELECT FOR UPDATE，强制 tenant 条件。
- `LockRowsForUpdate(tx, table, tenantID, ids)`：多行锁定，自动排序减少死锁。
- `WithTimeout(ctx, timeout, fn)`：context 超时保护。
- `SortLockKeys(keys)`：稳定排序锁键。

### 3.2 使用规则

1. **多行加锁必须先排序**：使用 `SortLockKeys` 或依赖 `LockRowsForUpdate` 内置排序。
2. **锁内禁止外部调用**：HTTP、文件 I/O、AI 调用等均不允许在持锁期间执行。
3. **始终使用超时保护**：通过 `WithTimeout` 包装，默认 10s。
4. **强制 tenant 条件**：`LockForTenant` / `LockRowsForUpdate` 必须传 tenantID。

### 3.3 错误码

| Code  | Key                     | 场景       |
|-------|-------------------------|------------|
| 40005 | `common.resource_locked`| 资源被锁定 |

## 4. 前端集成

### 4.1 幂等 Key

- `generateIdempotencyKey(actionId, payload)`：基于 actionId + 业务关键字段的稳定哈希。
- `useIdempotentAction(actionId)` composable：管理 key 生命周期（双击/超时复用）。

### 4.2 请求拦截

- 对幂等 Action 自动注入 `Idempotency-Key` header。
- 识别 40002 / 40003 / 40004 错误码并给出用户友好提示。

### 4.3 Revision 表单

- `useRevisionForm<T>()` composable：保留 revision 值，提交时携带 expected_revision。
- 处理 revision_conflict：提示用户刷新数据。

## 5. 术语对照

| 场景             | 唯一键                                   |
|------------------|------------------------------------------|
| HTTP Action 幂等 | `tenant + actor + action + idempotency_key` |
| Outbox Handler   | `event_id + handler_name`（已有，不复实现） |
| Background Job   | `tenant + job_type + idempotency_key`（已有，不复实现） |
| 资源乐观锁       | `tenant + resource_id + revision`           |

> **禁止**使用 `request_id` 替代 `idempotency_key`。
