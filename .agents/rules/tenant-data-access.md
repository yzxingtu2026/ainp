# 租户数据访问硬约束

- tenant 只来自 authenticated tenant-realm AuthContext；DTO、Query、Model 字段和消息 payload 不得构造数据库 scope。
- tenant-owned GORM Model 必须注册 Registry；确需认证启动、mixed realm 或跨租户恢复的 Model 必须进入逐项分类清单，禁止模糊豁免。
- 普通 CRUD 只经 `TenantDB` Repository；领域 GORM Query 只经 `GuardedDatabase`；平台跨租户只经 `SystemDB`。
- 禁止业务 Service 持有可直接执行 tenant CRUD 的裸 `*gorm.DB`；transaction 内禁止回退根 DB。
- Raw/Exec/Table/JOIN 只经 TenantRaw/TenantExec 或 SystemRaw/SystemExec；Callback 不保护未受控 SQL。
- `Unscoped()` 不能移除 tenant scope；生产调用必须精确登记并证明只处理软删除语义。
- Worker/Job/Outbox 必须从可信 PostgreSQL 事实恢复 AuthContext 并重新鉴权后再取 TenantDB；队列 tenant 不扩权。
- tenant-owned 表必须有 tenant 非空、租户内唯一、`(tenant_id,id)` 索引及关键复合外键；变更使用版本化 migration。
- 后台账号唯一事实源为 `TenantAccount/tenant_accounts`，账号角色为 `AccountRole/tenant_account_roles`；Identity/Session/AccountRole 外键统一使用 `account_id`。生产代码禁止访问或创建旧框架 `users/user_roles`，业务模块可独立定义自己的 `User/users`。
- 全仓 AST 门禁必须加入默认测试链；例外精确到文件、函数和调用方法，包含原因与风险，删除调用后 stale 例外必须失败。
- 新 Model 同步 Registry/分类、migration、跨租户矩阵、文档与测试；RLS 不在 Phase 1 范围。

详规：`docs/development/tenant-data-access.md`
