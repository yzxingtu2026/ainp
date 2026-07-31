# 租户数据访问迁移库存

本文档是 #112 的迁移基线。统计范围为 `platform/framework`、`platform/modules`、HTTP、Worker/Job 与 Outbox Handler；最终门禁与完整规范由 #113 收口。

## 基线

| 类别 | 初始数量 | 处理原则 |
|---|---:|---|
| 显式 tenant-owned GORM Model | 44 | 注册后只允许 TenantDB/SystemDB |
| 含手写顶层 tenant 条件的生产 Go 文件 | 29 | 改为 Repository/领域 Query，保留 owner/status/revision 条件 |
| Raw/Exec/Table/JOIN 调用点 | 37 | 业务查询迁移到受控出口；DDL/migration 精确登记 |

## 分批迁移

| 批次 | 范围 | 状态 | 验证 |
|---|---|---|---|
| #111 样板 | Agent、AgentVersion、AgentToolBinding 及直接 Runtime 消费方 | 已完成 | Agent/Builder Preview/PostgreSQL |
| Demo 模板 | RevisionDoc | 已完成 | 默认测试链与跨租户 Callback |
| System/Framework | Setting、Notification Request/Delivery/TemplateOverride/UserNotification（含 User 展示名与详情 JOIN）、Attachment/AttachmentLink（含 Service、管理查询、User 展示名与 Thumb Job）、Idempotency Record、Audit、Outbox、Job Run、AuthSession、Auth pre-auth/授权加载、Role 与 User 已迁移；Job Worker/Outbox dispatcher 保留为精确可信恢复例外 | 已完成 | 各模块 PostgreSQL 集成测试 |
| AI 治理与 Tool | Tool Catalog/Search/Scope/Confirmation/Invocation、Provider/Deployment/Alias/Model Invocation 与 FTS Raw 已迁移；Model Invocation 启动恢复保留私有固定 SQL 扫描并逐租户恢复 AuthContext | 已完成 | Model/Embedding/Tool Runtime 默认与 PostgreSQL 测试 |
| AI 会话与 Runtime | Conversation/Message/Stream/Snapshot、Run/Step/Event 已完成；Job claim 复用 Envelope 恢复的可信 tenant，Runtime ORM 已清理冗余 tenant 条件 | 已完成 | Conversation/Agent Runtime 默认与 PostgreSQL 测试 |
| Knowledge | Base/Document/Index/Chunk/Payload/Agent Binding 已启用 Registry 与 tenant-only ORM；Ingestion Worker 与 Service 冗余 tenant 条件已清理，Retrieval/Citation/Agent Binding JOIN 已迁移 TenantRaw | 已完成 | Knowledge 默认与 PostgreSQL 全链路测试 |
| Operations | Audit/User JOIN、Outbox/Event Delivery JOIN、Job 管理查询均已迁移 TenantRaw；Log 仅开放声明 TenantFilter 且回包复核 tenant 的 Provider | 已完成 | Operations Center/Audit/Outbox/Job/Log 测试 |

## 精确例外

| 位置 | 类型 | 原因 | 约束 |
|---|---|---|---|
| `framework/{job,idempotency,outbox}/migrate.go:ensureConstraints` | Exec/DDL | schema constraint 与索引维护 | 只在启动 migration 阶段 |
| `modules/ai/model.go:AutoMigrate`、`knowledge_model.go:AutoMigrateKnowledge/addKnowledgeConstraint` | Exec/DDL | AI schema、vector/FTS 与复合约束维护（含 tenant-owned `NavigationSearchDocument` 注册） | 只在启动 migration 阶段 |
| `modules/system/setup/seed.go:ensure*Constraint(s)` | Exec/Table | system schema 约束维护与固定表校验 | 只在启动 migration 阶段；固定内部表清单 |
| `RevisionDocService.AutoMigrate` | AutoMigrate | Demo 表启动建表 | 不执行 tenant-owned 业务 CRUD |
| `framework/revision`、`framework/dblock` | Table/Exec | 通用并发基础设施 | 调用方必须来自 scoped transaction；本 Issue 后续迁移 |
| `framework/audit.GormRecorder` | mixed-realm write | `audit_logs` 同时记录 tenant 与 platform realm | tenant 管理查询只走 TenantRaw；写入 tenant/realm 来自已认证 Action AuthContext |
| `framework/outbox.Dispatcher`、`gormDeliveryStore.RunHandler/ListByEvent`、`notify.go` | trusted dispatcher | Dispatcher 在请求 AuthContext 外跨租户 claim/publish/投递；notify 只控制固定 PostgreSQL LISTEN/NOTIFY 通道 | tenant 管理查询与手动重试走 TenantRaw；通知为空且不查询/承载 tenant 数据；能力不向业务暴露 |
| `framework/job.GetTrusted`、`RunStore.Create/UpdateFields/ListPendingEnqueue`、`CreateIdempotent` | trusted worker | Worker/Enqueuer 首次加载或写入 JobRun 时尚未恢复业务 AuthContext | tenant 列表/详情/取消走 TenantRaw；tenant 来自 ResolveTenantContext 或可信 JobRun |
| `InvocationService.RecoverStale` | trusted startup recovery | 启动时尚无 tenant AuthContext，需要发现跨租户遗留的 running 调用 | 仅允许私有固定 SQL 扫描未注册 DTO；逐行恢复 AuthContext 后通过 GuardedDatabase 更新与写 Outbox |
| `system.preAuthStore` | authentication bootstrap | 身份、会话和授权查询发生在完整 AuthContext 建立前 | 私有固定查询；tenant 来自已校验租户/身份；不暴露通用 DB |

逐调用方法的机器可检查清单位于 `platform/tests/tenant_data_architecture_test.go`，键固定为 `文件:函数:调用方法`，并记录原因与风险；删除调用后 stale exception 会令默认测试失败。

## 已识别契约风险

- Notification OverrideProvider 原接口缺少 `context.Context`，已升级为上下文透传并迁移到 TenantDB；禁止恢复仅凭 tenantID 的 context-free 查询。
- AuthSession refresh-token lookup 发生在 tenant 身份建立前，只能走 system 模块私有的 `preAuthStore` 固定查询；该能力不暴露通用 DB/SQL，也不能伪造 platform AuthContext。
- RolePermission 不含 tenant 字段，运行时读写必须通过 `roles.tenant_id` 归属约束的 TenantRaw/TenantExec；不得把它错误注册为可独立 tenant scope 的 Model。
- Idempotency cleanup 已收敛为仅 platform operator 可获取的 SystemDB；普通请求 CRUD 只接受 AuthContext tenant scope，方法参数 tenant 仅做冲突校验。
- Notification Delivery 已拍板：空 `status` 表示当前租户全部投递状态；异常队列使用显式 `failed`/`dead` 筛选，重试仍只允许这两种状态。旧测试期望已统一。

## 当前边界

- tenant 只来自 AuthContext；业务 DTO、Query 和消息 payload 不得构造数据库 tenant scope。
- 44 个持久化 tenant 字段 Model 已全部进入强制分类：36 个注册 TenantOwned，8 个为认证启动、mixed realm 或跨租户恢复例外；新增未分类 Model 由架构测试 fail-closed。
- owner、visibility、permission、revision、lease 和状态机仍是领域条件，不能因自动 tenant scope 删除。
- 业务复杂 SQL 只允许 TenantRaw/TenantExec 或领域专用 Query；SQL 中的 tenant 占位符由 AuthContext 注入。

## 最终扫描基线

- 普通业务裸 `Raw/Exec/Table/JOIN`：架构门禁零命中；剩余业务 SQL 均通过 TenantRaw/TenantExec。
- 普通业务手写顶层 tenant ORM 条件：架构门禁零命中；preAuth 与 Job enqueue 的精确启动例外已登记。
- DDL：精确限定到上表 migration/constraint 函数，不接受请求 tenant。
- 并发基础设施：`framework/revision` 与 `framework/dblock` 只接受调用方传入的 scoped transaction。
- 身份建立前查询：仅限 system 私有 `preAuthStore`，能力不向业务模块暴露。
- 跨租户后台能力：仅限已登记的 Outbox dispatcher、Job worker/pending recovery 与 ModelInvocation stale recovery。
- 受控 tenant SQL：System/Auth/Notification/Attachment、AI Raw/JOIN、Operations Audit/Outbox/Job 的 tenant 字样是 TenantRaw/TenantExec 固定占位符，不属于手写 ORM scope。
- 完整开发规范与新 Model 清单见 `docs/development/tenant-data-access.md`。
