# 租户数据访问

本文定义 tenant-owned 数据在 GORM、Repository、复杂 SQL、事务和后台恢复路径中的强制边界。它与 `auth-context.md` 配套：AuthContext 负责可信身份，本文负责把可信 tenant 强制落实到数据库访问。

## 1. 安全目标与非目标

普通租户业务必须满足：

```text
AuthContext
  → tenant.TenantDB / tenant.GuardedDatabase
  → Repository / Domain Query
  → GORM tenant callbacks
  → PostgreSQL composite constraints
```

- tenant 只来自 authenticated tenant-realm AuthContext。
- tenant-owned 查询缺少可信上下文时 fail-closed。
- tenant A 即使持有 tenant B 的真实资源 ID，也不能读取、更新、删除、锁定或关联该资源。
- tenant scope 不替代 owner、visibility、permission、revision、lease 或状态机。
- 本阶段不实现 PostgreSQL RLS；RLS 可作为后续纵深防御独立评估。

## 2. Model 分类与 Registry

持久化 Model 分为：

1. `TenantOwned`：注册到 `tenant.DefaultRegistry`，GORM Callback 强制 tenant scope。
2. global Model：无 tenant 归属，不得误注册为 TenantOwned。
3. trusted infrastructure / bootstrap Model：具有 tenant 字段，但必须在 AuthContext 建立前跨租户恢复或兼容 tenant/platform mixed realm；只能通过本文登记的私有能力访问。

当前 44 个带 tenant 字段的持久化 Model 中：

- 36 个注册为 TenantOwned；
- 8 个是精确登记的基础设施或认证启动例外：`audit.Record`、`outbox.Event`、`job.Run`、`User`、`AuthIdentity`、`AuthSession`、`Role`、`UserRole`。

完整机器可检查清单位于 `platform/tests/tenant_data_architecture_test.go`。新增带 GORM `TenantID` 的 Model 未注册或未分类时，默认测试立即失败。例外必须包含原因和风险，不允许目录、包或模糊正则豁免。

Registry 注册必须在该 Model 第一次运行时访问前完成。重复类型、非法 Kind、缺少字符串 TenantID 会失败；新增 Model 还必须补 migration、数据库约束和跨租户测试。

## 3. 数据库入口

### TenantDB

`tenant.TenantDB(ctx, db)`：

- 只接受 authenticated tenant realm；
- tenant 只取自 AuthContext；
- 返回不暴露根 DB 的 `TenantDatabase`；
- Repository、TenantRaw/TenantExec 和 scoped transaction 均从该对象构造；
- 缺少 tenant、错误 realm 或 nil DB 返回稳定错误。

```go
tdb, err := tenant.TenantDB(ctx, rootDB)
if err != nil {
    return err
}
repo, err := tenant.NewTenantRepository[Order](tdb)
```

### GuardedDatabase

`tenant.GuardedDB(rootDB)` 返回不暴露根 DB 的运行时句柄。它仅供已经注册的 tenant-owned Model 执行领域专用 GORM Query；每次 `WithContext` 都恢复 tenant callback 模式。新通用 CRUD 优先使用 TenantDB + Repository。

GuardedDatabase 不开放 Raw、Exec 或根 DB 提取接口。复杂 SQL 必须转换为 TenantRaw/TenantExec。

### SystemDB

`tenant.SystemDB(ctx, db)` 只允许 authenticated platform realm 且 `IsSystemOperator=true`。普通 tenant realm、伪造 system 标志和缺失上下文均不能构造。

SystemDB 是平台跨租户数据库访问的唯一公共入口。旧 `SystemScope` 已删除，不得恢复第二套可选入口。

### MigrationDB

`tenant.MigrationDB` 只允许 schema installation 使用。请求、Worker、Setup 业务数据和普通 Service 禁止调用。架构门禁按文件、函数和调用方法精确登记现有 migration 例外。

## 4. GORM Callback 边界

数据库初始化安装 Query、Row、Create、Update、Delete Callback：

- Query/Update/Delete 自动增加当前 tenant 条件；
- Create 自动填充空 TenantID；
- Model TenantID 与 AuthContext 冲突时返回 `ErrTenantConflict`；
- batch、Save、soft delete、`Unscoped()` 和 transaction 保留 tenant mode；
- SystemDB 仅访问显式允许 system access 的注册 Model；
- global Model 不被误加 tenant 条件。

Callback 不能保护以下操作：

- `Raw` / `Exec`；
- 动态字符串 `Table`；
- 手写 JOIN / CTE 中未声明为 GORM Model 的表；
- schema migration。

因此禁止宣称“安装 Callback 后任意 SQL 都自动安全”。这些路径必须走受控 SQL API或精确基础设施例外。

## 5. Repository 与领域 Query

Repository 只能从 TenantDatabase 或 SystemDatabase 构造。高频操作使用：

- `GetByID` / `First` / `Find` / `List`；
- `Count` / `Exists`；
- `Create` / `CreateBatch`；
- `UpdateRevision` / `Updates`；
- `Delete` / `DeleteWhere`。

领域 Query 可以表达 owner、visibility、status、revision、lease、分页和排序，但不得重新接收 tenant ID 作为 scope。not found 必须避免泄露其他 tenant 资源是否存在。

## 6. 事务与锁

租户事务必须从 TenantDatabase 开始：

```go
return tdb.Transaction(func(scoped tenant.TenantDatabase) error {
    repo, err := tenant.NewTenantRepository[Order](scoped)
    if err != nil {
        return err
    }
    // 所有查询和写入继续使用 scoped / repo。
    return nil
})
```

禁止在 transaction closure 内回退 Service 根 DB。ResourceLinker、Outbox Writer 等要求 `*gorm.DB` 的基础设施接口只能接收该 scoped transaction。

`revision.ConditionalUpdate` 与 `dblock` 只接受调用方提供的 scoped transaction。tenant ID 必须来自同一 AuthContext 或已加载的受信业务事实，不能来自 DTO、Query 或队列 payload。

## 7. 典型操作

### Create 与 batch

- 不由客户端提交 TenantID；
- Callback 自动注入；
- 冲突 TenantID 拒绝；
- batch 中每个元素都执行同样校验。

### Revision update

使用 Repository `UpdateRevision(id, expectedRevision, updates)`；tenant callback 与 revision 条件同时生效。零行更新按 not found/revision conflict 的领域语义映射。

### Soft delete 与 Unscoped

普通读取保留 GORM soft-delete 条件。`Unscoped()` 只能用于明确的恢复或清理路径；它不能移除 tenant callback。生产调用点必须进入精确例外清单。

### Preload 与 Association

父子 Model 都必须注册，并使用 `(tenant_id, id)` / `(tenant_id, foreign_id)` 关系。Preload、Association 和子查询从相同 GuardedDatabase/TenantDatabase 上下文开始。

### Join / Raw

```go
rows, err := tenant.TenantRaw[Row](tdb, `
SELECT ...
FROM resources r
JOIN children c
  ON c.tenant_id = r.tenant_id AND c.resource_id = r.id
WHERE r.tenant_id = ? AND r.status = ?`, status)
```

TenantRaw/TenantExec 自动把可信 tenant 作为第一个参数。SQL 必须包含参数化 tenant 占位符；所有 tenant-owned JOIN 表必须绑定同一 tenant。禁止开放客户端字段、操作符或 SQL 片段。

## 8. Worker、Job 与 Outbox

普通 Handler 必须：

1. 从 PostgreSQL JobRun/Outbox 可信事实加载 tenant、actor/session/auth version；
2. 校验 payload 中的资源 revision 与稳定 ID；
3. 恢复 AuthContext；
4. 重新加载授权；
5. 获取 TenantDB 后读取业务数据。

队列 payload 的 tenant 不是权限事实。

以下能力在 AuthContext 恢复前运行，因此保留私有精确例外：

- Outbox dispatcher claim/publish/delivery；
- Job pending enqueue、trusted run load 与已 claim run 状态更新；
- ModelInvocation stale startup recovery；
- pre-auth identity/session/authorization bootstrap；
- mixed-realm Audit append。

这些能力不得暴露为业务通用 DB、不得接受请求 tenant、不得返回根 DB。

## 9. 数据库纵深约束

tenant-owned 表必须具备：

- `tenant_id NOT NULL`；
- 租户内唯一约束；
- 供复合外键引用的 `(tenant_id, id)` 唯一索引；
- 跨 tenant 关系使用 `(tenant_id, foreign_id)` 复合外键；
- 列表索引以 tenant_id 为前导列。

`0028_tenant_composite_constraints.sql` 收口 System 身份、Agent、Conversation、Tool 和 Agent Runtime 的关键复合关系。约束变更必须使用版本化 migration，不能只修改 GORM tag。

## 10. 强制架构门禁

`platform/tests/tenant_data_architecture_test.go` 加入默认 `go test ./...`，检查：

- 新 tenant GORM Model 未注册/未分类；
- Service 从裸 `*gorm.DB` 字段直接执行 tenant CRUD；
- 手写顶层 tenant ORM 条件；
- 未登记 Raw/Exec/Table/Joins/Unscoped；
- 已删除的 RuntimeDB/SystemScope 兼容入口；
- GORM 条件占位符与实参数量不一致；
- 过期、无原因或无风险说明的例外。

门禁例外键固定为 `文件:函数:调用方法`。删除调用后必须同步删除例外，否则 stale exception 会令测试失败。

## 11. 新 Model / Repository 交付清单

- [ ] 确认 Model 是 TenantOwned、global 还是可信基础设施事实。
- [ ] TenantOwned 注册 Registry；启动顺序在首次访问前。
- [ ] migration 包含 tenant 非空、租户内唯一、复合索引和关键复合外键。
- [ ] Service 只保存 GuardedDatabase、Repository 或领域接口，不保存可直接绕过的根 DB。
- [ ] 普通 CRUD 使用 Repository；复杂 SQL 使用 TenantRaw/TenantExec。
- [ ] transaction 内不回退根 DB。
- [ ] owner/visibility/permission/revision 等领域约束未被删除。
- [ ] 覆盖 tenant A 使用 tenant B 真实 ID 的 read/list/count/create/update/delete/batch/transaction/preload/join/raw 测试。
- [ ] Worker 先恢复 AuthContext，再获取 TenantDB。
- [ ] 更新 inventory、本文和 Agent 规则。
- [ ] `go test ./...`、`go vet ./...`、PostgreSQL 集成测试、规则检查和 `git diff --check` 通过。
